package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rpanel/backend/system"
)

const sim = `Reading package lists...
Inst libc6 [2.39-0ubuntu8.3] (2.39-0ubuntu8.4 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst openssl [3.0.13-0ubuntu3.4] (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-security [amd64])
Inst vim [2:9.1.0016-1ubuntu7.3] (2:9.1.0016-1ubuntu7.5 Ubuntu:24.04/noble-updates [amd64])
Conf libc6 (2.39-0ubuntu8.4 Ubuntu:24.04/noble-updates [amd64])
`

type call struct {
	name string
	args []string
}

type fake struct {
	mu        sync.Mutex
	calls     []call
	afterRun  string // simulation output after an install
	installed bool
	failName  string
	dpkgOK    bool
	events    []string
}

func (f *fake) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{name, args})
	j := strings.Join(args, " ")
	if name == f.failName && strings.Contains(j, "upgrade") && !strings.Contains(j, "-s") || (name == f.failName && strings.Contains(j, "--only-upgrade")) {
		return system.Result{Stderr: "E: Sub-process /usr/bin/dpkg returned an error code (1)", ExitCode: 100}, errors.New("exit 100")
	}
	if name == "dpkg" {
		if f.dpkgOK {
			return system.Result{Stdout: "Package: unattended-upgrades\nStatus: install ok installed\n"}, nil
		}
		return system.Result{ExitCode: 1}, errors.New("not installed")
	}
	if name == "apt-get" && len(args) > 0 && args[0] == "-s" {
		if f.installed {
			return system.Result{Stdout: f.afterRun}, nil
		}
		return system.Result{Stdout: sim}, nil
	}
	if name == "apt-get" && (strings.Contains(j, "upgrade") || strings.Contains(j, "--only-upgrade")) {
		f.installed = true
		return system.Result{Stdout: "Get:1 http://archive.ubuntu.com libc6\nUnpacking libc6 (2.39)\nSetting up libc6 (2.39)\nUnpacking openssl\nSetting up openssl\nUnpacking vim\nSetting up vim\n"}, nil
	}
	return system.Result{}, nil
}

func (f *fake) find(name, sub string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.name == name && strings.Contains(strings.Join(c.args, " "), sub) {
			return c.args
		}
	}
	return nil
}

func setup(t *testing.T) (*Service, *fake, Paths) {
	t.Helper()
	dir := t.TempDir()
	p := Paths{RebootFile: filepath.Join(dir, "reboot-required"), AutoConf: filepath.Join(dir, "apt.conf.d", "52rpanel")}
	f := &fake{afterRun: "Reading package lists...\nInst vim [1] (2 Ubuntu:24.04/noble-updates [amd64])\n"}
	return NewService(f, p), f, p
}

func waitJob(t *testing.T, s *Service) *Progress {
	t.Helper()
	for i := 0; i < 300; i++ {
		if p := s.Progress(); p != nil && p.State != "running" {
			return p
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return nil
}

func TestParseSimulation(t *testing.T) {
	pk := parseSimulation(sim)
	if len(pk) != 3 {
		t.Fatalf("got %d packages: %+v", len(pk), pk)
	}
	if pk[0].Name != "libc6" || !pk[0].Security || pk[0].Current != "2.39-0ubuntu8.3" || pk[0].New != "2.39-0ubuntu8.4" {
		t.Errorf("first %+v", pk[0])
	}
	if pk[2].Name != "vim" || pk[2].Security {
		t.Errorf("security packages must sort first: %+v", pk)
	}
}

func TestStatusAndRebootFlag(t *testing.T) {
	s, _, p := setup(t)
	st := s.Status(context.Background())
	if !st.Known || st.Total != 3 || st.Security != 2 || st.RebootRequired || st.AutoSecurity {
		t.Fatalf("status %+v", st)
	}
	os.WriteFile(p.RebootFile, []byte("*** System restart required ***"), 0o644)
	os.WriteFile(p.RebootFile+".pkgs", []byte("linux-image-6.8.0-1\nlibc6\n"), 0o644)
	st = s.Status(context.Background())
	if !st.RebootRequired || len(st.RebootPackages) != 2 {
		t.Errorf("reboot flag %+v", st)
	}
}

func TestCheckJobRefreshesLists(t *testing.T) {
	s, f, _ := setup(t)
	if _, err := s.Start(KindCheck); err != nil {
		t.Fatal(err)
	}
	p := waitJob(t, s)
	if p.State != "done" || p.Percent != 100 || len(p.Steps) != 2 {
		t.Fatalf("job %+v", p)
	}
	if f.find("apt-get", "update") == nil {
		t.Error("apt-get update not run")
	}
	if f.find("apt-get", "upgrade") != nil && f.find("apt-get", "--only-upgrade") != nil {
		t.Error("a check must not install anything")
	}
}

func TestSecurityJobInstallsOnlySecurityPackages(t *testing.T) {
	s, f, _ := setup(t)
	if _, err := s.Start(KindSecurity); err != nil {
		t.Fatal(err)
	}
	p := waitJob(t, s)
	if p.State != "done" || p.Total != 2 {
		t.Fatalf("job %+v", p)
	}
	args := f.find("apt-get", "--only-upgrade")
	j := strings.Join(args, " ")
	if !strings.Contains(j, "libc6") || !strings.Contains(j, "openssl") || strings.Contains(j, "vim") {
		t.Errorf("wrong package set: %v", args)
	}
	if !strings.Contains(j, "--force-confold") || !strings.Contains(j, "DPkg::Lock::Timeout=300") {
		t.Errorf("missing safety options: %v", args)
	}
	if st := s.Status(context.Background()); st.Total != 1 || st.Security != 0 {
		t.Errorf("counts not refreshed: %+v", st)
	}
	if p.Done != 2 {
		t.Errorf("installed counter %d", p.Done)
	}
}

func TestAllJobAndFailureReporting(t *testing.T) {
	s, f, _ := setup(t)
	if _, err := s.Start(KindAll); err != nil {
		t.Fatal(err)
	}
	if p := waitJob(t, s); p.State != "done" || p.Total != 3 {
		t.Fatalf("all job %+v", p)
	}
	s2, f2, _ := setup(t)
	f2.failName = "apt-get"
	if _, err := s2.Start(KindAll); err != nil {
		t.Fatal(err)
	}
	p := waitJob(t, s2)
	if p.State != "failed" || !strings.Contains(p.Error, "dpkg returned an error") {
		t.Fatalf("expected a failure, got %+v", p)
	}
	failed := false
	for _, st := range p.Steps {
		if st.Key == "install" && st.Status == "failed" {
			failed = true
		}
	}
	if !failed {
		t.Errorf("install step should be failed: %+v", p.Steps)
	}
	_ = f
}

func TestOnlyOneJobAtATime(t *testing.T) {
	s, _, _ := setup(t)
	s.job = &job{state: "running", started: time.Now()}
	if _, err := s.Start(KindAll); !errors.Is(err, ErrBusy) {
		t.Errorf("expected busy, got %v", err)
	}
	if err := s.Reboot(true); !errors.Is(err, ErrBusy) {
		t.Errorf("reboot during a job must be refused, got %v", err)
	}
	if _, err := s.Start("rm -rf"); !errors.Is(err, ErrBadInput) {
		t.Errorf("bad kind: %v", err)
	}
}

func TestAutoSecurity(t *testing.T) {
	s, f, p := setup(t)
	if err := s.SetAutoSecurity(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if f.find("apt-get", "install") == nil {
		t.Error("unattended-upgrades should be installed when missing")
	}
	b, _ := os.ReadFile(p.AutoConf)
	if !strings.Contains(string(b), `Unattended-Upgrade "1"`) {
		t.Errorf("conf: %s", b)
	}
	if !s.autoEnabled() {
		t.Error("autoEnabled")
	}
	if err := s.SetAutoSecurity(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if s.autoEnabled() {
		t.Error("should be off")
	}
}

func TestReboot(t *testing.T) {
	s, _, _ := setup(t)
	done := make(chan struct{}, 1)
	s.rebootFn = func() error { done <- struct{}{}; return nil }
	if err := s.Reboot(false); !errors.Is(err, ErrBadInput) {
		t.Errorf("confirmation is required, got %v", err)
	}
	select {
	case <-done:
		t.Fatal("rebooted without confirmation")
	case <-time.After(100 * time.Millisecond):
	}
	if err := s.Reboot(true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("no reboot")
	}
}
