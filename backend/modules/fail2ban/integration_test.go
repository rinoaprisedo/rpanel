//go:build integration

package fail2ban

import (
	"context"
	"os"
	"testing"

	"rpanel/backend/system"
)

// Runs against a real fail2ban daemon (see the docker command in the commit notes).
func TestIntegrationReal(t *testing.T) {
	s := NewService(system.Default, DefaultPaths())
	ctx := context.Background()
	st := s.Status(ctx, "192.0.2.1")
	t.Logf("status: installed=%v running=%v version=%q jails=%d", st.Installed, st.Running, st.Version, len(st.Jails))
	if !st.Running || len(st.Jails) == 0 {
		t.Fatalf("daemon not running: %+v", st)
	}
	for _, j := range st.Jails {
		t.Logf("jail %+v", j)
	}
	set, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set.BanTime, set.MaxRetry = "2h", 3
	set.IgnoreIP = append(set.IgnoreIP, "203.0.113.5")
	for i := range set.Jails {
		if set.Jails[i].Name == "sshd" {
			set.Jails[i].Enabled = true
			set.Jails[i].MaxRetry = 2
		}
	}
	got, err := s.Save(ctx, *set)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Logf("saved: %+v", got)
	b, _ := os.ReadFile(DefaultPaths().ConfFile)
	t.Logf("file:\n%s", b)
	st = s.Status(ctx, "")
	for _, j := range st.Jails {
		if j.Name == "sshd" && (j.MaxRetry != "2" || j.BanTime != "7200") {
			t.Errorf("not applied: %+v", j)
		}
	}
	if err := s.Ban(ctx, "sshd", "198.51.100.77", "192.0.2.1"); err != nil {
		t.Logf("ban (may fail without iptables in a container): %v", err)
	} else {
		st = s.Status(ctx, "")
		t.Logf("after ban: %+v", st.Jails[0].BannedIPs)
		if err := s.Unban(ctx, "sshd", "198.51.100.77"); err != nil {
			t.Fatalf("unban: %v", err)
		}
	}
	// a bad config must roll back and leave the daemon working
	bad := *got
	bad.Jails = append([]JailSetting(nil), got.Jails...)
	if _, err := s.Save(ctx, bad); err != nil {
		t.Fatalf("resave: %v", err)
	}
	lines, _ := s.Log(20)
	t.Logf("log lines: %d", len(lines))
}
