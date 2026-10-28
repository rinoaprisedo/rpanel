package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"rpanel/backend/system"
)

const releasesJSON = `[
 {"tag_name":"v1.4.0","name":"RPanel 1.4.0","body":"## 1.4.0\n- Fail2ban page","draft":false,"prerelease":false,"published_at":"2026-10-30T10:00:00Z","html_url":"https://github.com/x/y/releases/tag/v1.4.0"},
 {"tag_name":"v1.3.3","name":"RPanel 1.3.3","body":"## 1.3.3\n- Panel SSL","draft":false,"prerelease":false,"published_at":"2026-10-29T10:00:00Z","html_url":"https://github.com/x/y/releases/tag/v1.3.3"},
 {"tag_name":"v1.5.0-rc1","name":"rc","body":"x","draft":false,"prerelease":true,"published_at":"2026-10-31T10:00:00Z","html_url":""},
 {"tag_name":"v1.9.0","name":"draft","body":"x","draft":true,"prerelease":false,"published_at":"","html_url":""},
 {"tag_name":"v1.3.2","name":"RPanel 1.3.2","body":"old","draft":false,"prerelease":false,"published_at":"2026-10-28T10:00:00Z","html_url":""},
 {"tag_name":"weird","name":"x","body":"x","draft":false,"prerelease":false}
]`

type call struct {
	name string
	args []string
}

type fake struct {
	calls  []call
	active string
	failSR bool
}

func (f *fake) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.calls = append(f.calls, call{name, args})
	if name == "systemctl" {
		return system.Result{Stdout: f.active}, nil
	}
	if name == "systemd-run" && f.failSR {
		return system.Result{Stderr: "Failed to connect to bus", ExitCode: 1}, errors.New("exit 1")
	}
	return system.Result{}, nil
}

func setup(t *testing.T, current string) (*Service, *fake, *int32, Settings) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/repos/x/y/releases" || r.Header.Get("User-Agent") == "" {
			http.Error(w, "bad", 400)
			return
		}
		w.Write([]byte(releasesJSON))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	bin := filepath.Join(dir, "rpanel-update")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	cfg := Settings{Repo: "x/y", APIBase: srv.URL, Current: current, LogPath: filepath.Join(dir, "update.log"), UpdateBin: bin}
	f := &fake{}
	return NewService(f, cfg), f, &hits, cfg
}

func TestInfoListsNewerStableReleases(t *testing.T) {
	s, _, _, _ := setup(t, "1.3.2")
	info, err := s.Info(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Latest != "1.4.0" || info.Name != "RPanel 1.4.0" {
		t.Fatalf("info %+v", info)
	}
	if len(info.Releases) != 2 || info.Releases[0].Tag != "1.4.0" || info.Releases[1].Tag != "1.3.3" {
		t.Fatalf("releases %+v", info.Releases)
	}
	if !strings.Contains(info.Releases[0].Body, "Fail2ban page") {
		t.Error("release notes missing")
	}
	if len(info.History) != 1 || info.History[0].Tag != "1.3.2" || info.History[0].Body != "old" {
		t.Errorf("history should hold the running release: %+v", info.History)
	}
}

func TestHistoryWhenUpToDate(t *testing.T) {
	s, _, _, _ := setup(t, "1.4.0")
	info, _ := s.Info(context.Background(), false)
	if len(info.History) != 3 || info.History[0].Tag != "1.4.0" || info.History[2].Tag != "1.3.2" {
		t.Fatalf("history newest first, current included: %+v", info.History)
	}
}

func TestInfoUpToDateAndCache(t *testing.T) {
	s, _, hits, _ := setup(t, "1.4.0")
	info, _ := s.Info(context.Background(), false)
	if info.Available || info.Latest != "1.4.0" || len(info.Releases) != 0 {
		t.Fatalf("expected up to date: %+v", info)
	}
	s.Info(context.Background(), false)
	if atomic.LoadInt32(hits) != 1 {
		t.Errorf("cache not used: %d requests", *hits)
	}
	s.Info(context.Background(), true)
	if atomic.LoadInt32(hits) != 2 {
		t.Error("force must refetch")
	}
}

func TestInfoKeepsStaleAnswerWhenGitHubIsDown(t *testing.T) {
	s, _, _, cfg := setup(t, "1.3.2")
	s.Info(context.Background(), false)
	s.cfg.APIBase = "http://127.0.0.1:1" // unreachable
	s.ttl = 0
	info, err := s.Info(context.Background(), false)
	if err != nil || info.Latest != "1.4.0" {
		t.Fatalf("expected the cached answer, got %+v %v", info, err)
	}
	s2 := NewService(&fake{}, Settings{Repo: "x/y", APIBase: "http://127.0.0.1:1", Current: "1.3.2", LogPath: cfg.LogPath})
	if _, err := s2.Info(context.Background(), false); err == nil {
		t.Error("with nothing cached an unreachable GitHub is an error")
	}
}

func TestStartLaunchesDetachedUnit(t *testing.T) {
	s, f, _, cfg := setup(t, "1.3.2")
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var args []string
	for _, c := range f.calls {
		if c.name == "systemd-run" {
			args = c.args
		}
	}
	j := strings.Join(args, " ")
	for _, want := range []string{"--unit=rpanel-self-update", "--collect", "StandardOutput=file:" + cfg.LogPath, cfg.UpdateBin + " 1.4.0"} {
		if !strings.Contains(j, want) {
			t.Errorf("systemd-run args missing %q: %s", want, j)
		}
	}
}

func TestStartRefusals(t *testing.T) {
	s, f, _, _ := setup(t, "1.4.0")
	if err := s.Start(context.Background()); !errors.Is(err, ErrBadInput) {
		t.Errorf("up to date: %v", err)
	}
	s, f, _, _ = setup(t, "1.3.2")
	f.active = "active\n"
	if err := s.Start(context.Background()); !errors.Is(err, ErrBusy) {
		t.Errorf("running: %v", err)
	}
	f.active = "inactive\n"
	s.cfg.UpdateBin = "/nonexistent/rpanel-update"
	if err := s.Start(context.Background()); !errors.Is(err, ErrNoScript) {
		t.Errorf("no script: %v", err)
	}
	s.cfg.UpdateBin = os.Args[0]
	f.failSR = true
	if err := s.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "Failed to connect") {
		t.Errorf("systemd-run failure should surface: %v", err)
	}
	s.cfg.LogPath = "/tmp/a b;rm"
	f.failSR = false
	if err := s.Start(context.Background()); !errors.Is(err, ErrBadInput) {
		t.Errorf("unsafe log path: %v", err)
	}
}

func TestJobStateFromUnitAndLog(t *testing.T) {
	s, f, _, cfg := setup(t, "1.3.2")
	ctx := context.Background()
	if j := s.JobOnly(ctx); j.State != "idle" || len(j.Log) != 0 {
		t.Fatalf("idle: %+v", j)
	}
	f.active = "active\n"
	os.WriteFile(cfg.LogPath, []byte("\x1b[1;32m==>\x1b[0m Downloading rpanel-1.4.0\n"), 0o644)
	j := s.JobOnly(ctx)
	if j.State != "running" || len(j.Log) != 1 || strings.Contains(j.Log[0], "\x1b") || !strings.Contains(j.Log[0], "Downloading") {
		t.Fatalf("running: %+v", j)
	}
	f.active = "inactive\n"
	os.WriteFile(cfg.LogPath, []byte("==> Upgrading RPanel 1.3.2 -> 1.4.0\n==> Upgrade finished\n"), 0o644)
	if j := s.JobOnly(ctx); j.State != "done" {
		t.Errorf("done: %+v", j)
	}
	os.WriteFile(cfg.LogPath, []byte("warning: upgrade failed: restoring the previous version\nerror: upgrade failed and was rolled back\n"), 0o644)
	if j := s.JobOnly(ctx); j.State != "failed" {
		t.Errorf("failed: %+v", j)
	}
}

func TestStatusReportsCanUpdate(t *testing.T) {
	s, _, _, _ := setup(t, "1.3.2")
	st, err := s.Status(context.Background(), false)
	if err != nil || !st.CanUpdate || st.Job == nil {
		t.Fatalf("%+v %v", st, err)
	}
	s.cfg.UpdateBin = "/nonexistent"
	if st, _ := s.Status(context.Background(), false); st.CanUpdate {
		t.Error("can_update must be false without the script")
	}
}
