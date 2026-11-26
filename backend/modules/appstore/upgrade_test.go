package appstore

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"rpanel/backend/system"
)

// nginxRunner scripts the commands an nginx upgrade runs.
type nginxRunner struct {
	*fakeRunner
	candidate     string
	testFails     []bool // successive `nginx -t` outcomes (true = fails), then success
	restartActive bool   // is nginx active after the restart
	tests         int
}

func (n *nginxRunner) Run(ctx context.Context, d time.Duration, name string, args ...string) (system.Result, error) {
	switch {
	case name == "apt-cache" && len(args) == 2 && args[0] == "policy":
		return system.Result{Stdout: args[1] + ":\n  Installed: 1.24.0-2ubuntu7\n  Candidate: " + n.candidate + "\n"}, nil
	case name == "nginx" && len(args) == 1 && args[0] == "-t":
		n.fakeRunner.Run(ctx, d, name, args...) // record
		n.mu.Lock()
		i := n.tests
		n.tests++
		n.mu.Unlock()
		if i < len(n.testFails) && n.testFails[i] {
			return system.Result{Stderr: "nginx: [emerg] unknown directive \"x\"\n"}, errors.New("exit 1")
		}
		return system.Result{}, nil
	case name == "tar" && len(args) > 1 && args[0] == "-czf":
		n.fakeRunner.Run(ctx, d, name, args...)
		return system.Result{}, os.WriteFile(args[1], []byte("x"), 0o600)
	case name == "systemctl" && len(args) == 2 && args[0] == "is-active":
		if n.restartActive {
			return system.Result{Stdout: "active\n"}, nil
		}
		return system.Result{Stdout: "failed\n"}, errors.New("exit 3")
	}
	return n.fakeRunner.Run(ctx, d, name, args...)
}

func newNginxSvc(t *testing.T, n *nginxRunner) *Service {
	t.Helper()
	backupDir, settle = t.TempDir(), 0
	n.fakeRunner = &fakeRunner{installed: map[string]string{"nginx": "1.24.0-2ubuntu7"}, active: map[string]bool{"nginx": true}}
	return NewService(n)
}

func jobState(s *Service) (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs["nginx"]; ok {
		return j.state, j.err
	}
	return StateIdle, ""
}

func TestUpgradeNginxSuccess(t *testing.T) {
	n := &nginxRunner{candidate: "1.24.0-2ubuntu7.9", restartActive: true}
	s := newNginxSvc(t, n)
	if _, err := s.Upgrade("nginx"); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	if st, e := jobState(s); st != StateIdle {
		t.Fatalf("state %s: %s", st, e)
	}
	if !n.saw("apt-get", "install", "--only-upgrade") || !n.saw("systemctl", "restart", "nginx") {
		t.Fatalf("calls: %+v", n.calls)
	}
	if n.saw("tar", "-xzf", "") {
		t.Fatal("must not restore on success")
	}
}

func TestUpgradeNginxRefusesBrokenConfig(t *testing.T) {
	n := &nginxRunner{candidate: "1.24.0-2ubuntu7.9", testFails: []bool{true}, restartActive: true}
	s := newNginxSvc(t, n)
	s.Upgrade("nginx")
	s.wg.Wait()
	st, e := jobState(s)
	if st != StateFailed || !strings.Contains(e, "invalid") {
		t.Fatalf("state %s: %s", st, e)
	}
	if n.saw("apt-get", "install", "") || n.saw("systemctl", "restart", "") {
		t.Fatal("nothing may change when the configuration is already broken")
	}
}

func TestUpgradeNginxRollsBackWhenNewBinaryRejectsConfig(t *testing.T) {
	n := &nginxRunner{candidate: "1.24.0-2ubuntu7.9", testFails: []bool{false, true}, restartActive: true}
	s := newNginxSvc(t, n)
	s.Upgrade("nginx")
	s.wg.Wait()
	st, e := jobState(s)
	if st != StateFailed || !strings.Contains(e, "restored") {
		t.Fatalf("state %s: %s", st, e)
	}
	if !n.saw("tar", "-xzf", "nginx-") {
		t.Fatalf("backup not restored: %+v", n.calls)
	}
}

func TestUpgradeNginxRollsBackWhenItDoesNotStart(t *testing.T) {
	n := &nginxRunner{candidate: "1.24.0-2ubuntu7.9", restartActive: false}
	s := newNginxSvc(t, n)
	s.Upgrade("nginx")
	s.wg.Wait()
	st, e := jobState(s)
	if st != StateFailed || !strings.Contains(e, "did not start") {
		t.Fatalf("state %s: %s", st, e)
	}
	if !n.saw("tar", "-xzf", "nginx-") {
		t.Fatal("configuration must be restored")
	}
}

func TestUpgradeNginxAlreadyCurrent(t *testing.T) {
	n := &nginxRunner{candidate: "1.24.0-2ubuntu7", restartActive: true}
	s := newNginxSvc(t, n)
	s.Upgrade("nginx")
	s.wg.Wait()
	if st, e := jobState(s); st != StateIdle {
		t.Fatalf("state %s: %s", st, e)
	}
	if n.saw("apt-get", "install", "") || n.saw("systemctl", "restart", "") {
		t.Fatal("nothing to do when apt offers no newer build")
	}
}

func TestUpgradeRejectsOtherApps(t *testing.T) {
	s := NewService(&fakeRunner{installed: map[string]string{"redis-server": "7"}})
	if _, err := s.Upgrade("redis"); !errors.Is(err, ErrBadInput) {
		t.Fatalf("redis: %v", err)
	}
	if _, err := s.Upgrade("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := NewService(&fakeRunner{}).Upgrade("nginx"); !errors.Is(err, ErrBadState) {
		t.Fatalf("not installed: %v", err)
	}
}
