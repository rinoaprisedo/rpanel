//go:build integration

package supervisor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"rpanel/backend/system"
)

func TestIntegrationReal(t *testing.T) {
	s := NewService(system.Default, DefaultPaths())
	ctx := context.Background()
	st := s.Status(ctx)
	t.Logf("status: installed=%v running=%v version=%q procs=%d err=%q", st.Installed, st.Running, st.Version, len(st.Processes), st.Error)
	if !st.Running {
		t.Fatal("supervisord is not running")
	}

	c := Config{Name: "ticker", Command: `sh -c "while true; do echo tick 100%; sleep 1; done"`, User: "root", NumProcs: 2, AutoStart: true, AutoRestart: "true", Environment: []string{"MODE=prod", `MSG=say "hi"`}}
	if _, err := s.Save(ctx, c, true); err != nil {
		t.Fatalf("save: %v", err)
	}
	time.Sleep(3 * time.Second)
	st = s.Status(ctx)
	running := 0
	for _, p := range st.Processes {
		t.Logf("  %s %s %s managed=%v", p.Name, p.State, p.Detail, p.Managed)
		if p.Program == "ticker" && p.State == "RUNNING" && p.Managed {
			running++
		}
	}
	if running != 2 {
		t.Fatalf("expected 2 running managed processes, got %d", running)
	}
	out, err := s.Log(ctx, "ticker:ticker_00", "stdout", 0)
	t.Logf("log: %q err=%v", strings.TrimSpace(out), err)
	if !strings.Contains(out, "tick 100%") {
		t.Errorf("log should contain the program output")
	}
	if _, err := s.Action(ctx, "stop", "ticker:ticker_00"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := s.Action(ctx, "start", "ticker:ticker_00"); err != nil {
		t.Fatalf("start: %v", err)
	}
	// update: single process, different command
	c.NumProcs, c.Command = 1, `sh -c "while true; do echo tock; sleep 1; done"`
	if _, err := s.Save(ctx, c, false); err != nil {
		t.Fatalf("update: %v", err)
	}
	time.Sleep(2 * time.Second)
	for _, p := range s.Status(ctx).Processes {
		t.Logf("  after update: %s %s", p.Name, p.State)
	}
	// a program supervisord refuses must roll back cleanly
	bad := Config{Name: "broken", Command: "/definitely/not/here --x", Directory: "/nonexistent-dir", NumProcs: 1, AutoStart: true, AutoRestart: "false"}
	_, err = s.Save(ctx, bad, true)
	t.Logf("broken program: err=%v", err)
	if _, statErr := os.Stat(DefaultPaths().ConfDir + "/rpanel-broken.conf"); err != nil && statErr == nil {
		t.Error("a rejected program must not stay on disk")
	}
	if err := s.Delete(ctx, "ticker"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete(ctx, "broken"); err != nil {
		t.Logf("delete broken: %v", err)
	}
	for _, p := range s.Status(ctx).Processes {
		if p.Program == "ticker" {
			t.Errorf("ticker still listed: %+v", p)
		}
	}
}
