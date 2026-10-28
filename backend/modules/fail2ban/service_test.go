package fail2ban

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rpanel/backend/system"
)

type call struct {
	name string
	args []string
}

type fake struct {
	calls   []call
	outputs map[string]string // key: joined args
	failOn  string            // joined args that fail
}

func (f *fake) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.calls = append(f.calls, call{name, args})
	key := strings.Join(args, " ")
	if f.failOn != "" && key == f.failOn {
		return system.Result{Stderr: "ERROR  Failed during configuration: bad", ExitCode: 1}, errors.New("exit 1")
	}
	return system.Result{Stdout: f.outputs[key]}, nil
}

func (f *fake) did(key string) bool {
	for _, c := range f.calls {
		if strings.Join(c.args, " ") == key {
			return true
		}
	}
	return false
}

const sshdStatus = "Status for the jail: sshd\n|- Filter\n|  |- Currently failed:\t2\n|  |- Total failed:\t17\n|  `- Journal matches:\t_SYSTEMD_UNIT=ssh.service\n`- Actions\n   |- Currently banned:\t2\n   |- Total banned:\t5\n   `- Banned IP list:\t203.0.113.7 198.51.100.4\n"

func setup(t *testing.T) (*Service, *fake, Paths) {
	t.Helper()
	dir := t.TempDir()
	p := Paths{ConfFile: filepath.Join(dir, "jail.d", "rpanel.local"), FilterDir: filepath.Join(dir, "filter.d"), LogFile: filepath.Join(dir, "fail2ban.log")}
	os.MkdirAll(p.FilterDir, 0o755)
	os.WriteFile(p.LogFile, []byte("log\n"), 0o644)
	for _, f := range []string{"sshd.conf", "nginx-http-auth.conf", "recidive.conf"} {
		os.WriteFile(filepath.Join(p.FilterDir, f), []byte("x"), 0o644)
	}
	f := &fake{outputs: map[string]string{
		"version":           "1.0.2",
		"ping":              "Server replied: pong",
		"status":            "Status\n|- Number of jail:\t1\n`- Jail list:\tsshd",
		"status sshd":       sshdStatus,
		"get sshd bantime":  "600",
		"get sshd findtime": "600",
		"get sshd maxretry": "5",
	}}
	s := NewService(f, p)
	s.installed = func() bool { return true }
	return s, f, p
}

func TestStatusParsesJails(t *testing.T) {
	s, _, _ := setup(t)
	st := s.Status(context.Background(), "192.0.2.1")
	if !st.Installed || !st.Running || st.Version != "1.0.2" || len(st.Jails) != 1 {
		t.Fatalf("status %+v", st)
	}
	j := st.Jails[0]
	if j.Name != "sshd" || j.CurrentlyFailed != 2 || j.TotalFailed != 17 || j.CurrentlyBanned != 2 || j.TotalBanned != 5 {
		t.Errorf("counters %+v", j)
	}
	if len(j.BannedIPs) != 2 || j.BannedIPs[0] != "203.0.113.7" {
		t.Errorf("banned %v", j.BannedIPs)
	}
	if j.BanTime != "600" || j.MaxRetry != "5" || !j.Managed {
		t.Errorf("config %+v", j)
	}
}

func TestStatusNotInstalledOrStopped(t *testing.T) {
	s, f, _ := setup(t)
	s.installed = func() bool { return false }
	if st := s.Status(context.Background(), ""); st.Installed || len(f.calls) != 0 {
		t.Error("must not call fail2ban-client when not installed")
	}
	s.installed = func() bool { return true }
	f.outputs["ping"] = ""
	if st := s.Status(context.Background(), ""); st.Running {
		t.Error("should report stopped")
	}
}

func TestBanRules(t *testing.T) {
	s, f, _ := setup(t)
	ctx := context.Background()
	for _, ip := range []string{"not-an-ip", "127.0.0.1", "0.0.0.0", "::1"} {
		if err := s.Ban(ctx, "sshd", ip, "192.0.2.1"); !errors.Is(err, ErrBadInput) {
			t.Errorf("%s should be refused, got %v", ip, err)
		}
	}
	if err := s.Ban(ctx, "sshd", "192.0.2.1", "192.0.2.1"); !errors.Is(err, ErrBadInput) || !strings.Contains(err.Error(), "own IP") {
		t.Errorf("own IP must be refused: %v", err)
	}
	if err := s.Ban(ctx, "nope", "203.0.113.9", "192.0.2.1"); !errors.Is(err, ErrBadInput) {
		t.Errorf("unknown jail: %v", err)
	}
	if err := s.Ban(ctx, "sshd;rm", "203.0.113.9", ""); !errors.Is(err, ErrBadInput) {
		t.Errorf("bad jail name: %v", err)
	}
	if err := s.Ban(ctx, "sshd", "203.0.113.9", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if !f.did("set sshd banip 203.0.113.9") {
		t.Error("banip not called")
	}
	if err := s.Unban(ctx, "sshd", "203.0.113.7"); err != nil || !f.did("set sshd unbanip 203.0.113.7") {
		t.Errorf("unban: %v", err)
	}
}

func goodSettings() Settings {
	return Settings{BanTime: "1h", FindTime: "10m", MaxRetry: 4, IgnoreIP: []string{"127.0.0.1/8", "::1", "203.0.113.5"},
		Jails: []JailSetting{{Name: "sshd", Enabled: true, MaxRetry: 3}, {Name: "recidive", Enabled: true, BanTime: "-1"}}}
}

func TestSaveWritesFileAndReloads(t *testing.T) {
	s, f, p := setup(t)
	got, err := s.Save(context.Background(), goodSettings())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p.ConfFile)
	for _, want := range []string{"[DEFAULT]", "bantime = 1h", "maxretry = 4", "ignoreip = 127.0.0.1/8 ::1 203.0.113.5", "[sshd]\nenabled = true\nmaxretry = 3", "[recidive]\nenabled = true\nbantime = -1"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("file missing %q:\n%s", want, b)
		}
	}
	if !f.did("-t") || !f.did("reload") {
		t.Errorf("expected -t and reload, got %v", f.calls)
	}
	if got.BanTime != "1h" || got.MaxRetry != 4 || len(got.IgnoreIP) != 3 {
		t.Errorf("roundtrip %+v", got)
	}
	var sshd, recidive JailSetting
	for _, j := range got.Jails {
		switch j.Name {
		case "sshd":
			sshd = j
		case "recidive":
			recidive = j
		}
	}
	if !sshd.Enabled || sshd.MaxRetry != 3 || !recidive.Enabled || recidive.BanTime != "-1" {
		t.Errorf("jails %+v %+v", sshd, recidive)
	}
}

func TestSaveRollsBackWhenRejected(t *testing.T) {
	s, f, p := setup(t)
	os.MkdirAll(filepath.Dir(p.ConfFile), 0o755)
	os.WriteFile(p.ConfFile, []byte("# previous\n[DEFAULT]\nbantime = 5m\n"), 0o644)
	f.failOn = "-t"
	if _, err := s.Save(context.Background(), goodSettings()); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected rejection, got %v", err)
	}
	if b, _ := os.ReadFile(p.ConfFile); !strings.Contains(string(b), "# previous") {
		t.Errorf("previous file not restored: %s", b)
	}
	if f.did("reload") {
		t.Error("must not reload a rejected config")
	}
	// no previous file: the new one is removed
	os.Remove(p.ConfFile)
	f.failOn = "-t"
	if _, err := s.Save(context.Background(), goodSettings()); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(p.ConfFile); err == nil {
		t.Error("file should be removed after a failed first save")
	}
}

func TestValidateRejectsInjection(t *testing.T) {
	s, _, _ := setup(t)
	bad := []func(*Settings){
		func(x *Settings) { x.BanTime = "1h\n[evil]" },
		func(x *Settings) { x.BanTime = "abc" },
		func(x *Settings) { x.FindTime = "-1" },
		func(x *Settings) { x.MaxRetry = 0 },
		func(x *Settings) { x.IgnoreIP = []string{"1.2.3.4; rm -rf /"} },
		func(x *Settings) { x.IgnoreIP = []string{"999.1.1.1"} },
		func(x *Settings) { x.Jails = []JailSetting{{Name: "sshd]\n[x"}} },
		func(x *Settings) { x.Jails = []JailSetting{{Name: "sshd"}, {Name: "sshd"}} },
		func(x *Settings) { x.Jails = []JailSetting{{Name: "sshd", BanTime: "x y"}} },
		func(x *Settings) { x.Jails = []JailSetting{{Name: "nginx-botsearch", Enabled: true}} }, // no filter file in the fixture
	}
	for i, mut := range bad {
		in := goodSettings()
		mut(&in)
		if err := s.Validate(&in); !errors.Is(err, ErrBadInput) {
			t.Errorf("case %d should be rejected, got %v", i, err)
		}
	}
	in := goodSettings()
	if err := s.Validate(&in); err != nil {
		t.Errorf("good settings rejected: %v", err)
	}
}

func TestSettingsDefaultsAndRunningJail(t *testing.T) {
	s, _, _ := setup(t)
	set, err := s.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if set.BanTime != "10m" || set.MaxRetry != 5 {
		t.Errorf("defaults %+v", set)
	}
	for _, j := range set.Jails {
		if j.Name == "sshd" && !j.Enabled {
			t.Error("a running jail must show as enabled")
		}
		if j.Name == "nginx-botsearch" && j.Available {
			t.Error("jail without a filter must be unavailable")
		}
	}
}

func TestLogTail(t *testing.T) {
	s, _, p := setup(t)
	os.Remove(p.LogFile)
	if lines, err := s.Log(10); err != nil || len(lines) != 0 {
		t.Fatalf("missing log: %v %v", lines, err)
	}
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("2026-10-28 line " + strings.Repeat("x", 3) + "\n")
	}
	os.WriteFile(p.LogFile, []byte(b.String()), 0o644)
	lines, err := s.Log(5)
	if err != nil || len(lines) != 5 {
		t.Fatalf("tail: %v %d", err, len(lines))
	}
}

func TestControlAndNotInstalled(t *testing.T) {
	s, f, _ := setup(t)
	if err := s.Control(context.Background(), "restart"); err != nil || f.calls[len(f.calls)-1].name != "systemctl" {
		t.Errorf("restart: %v", err)
	}
	if err := s.Control(context.Background(), "rm -rf"); !errors.Is(err, ErrBadInput) {
		t.Error("bad action must be refused")
	}
	s.installed = func() bool { return false }
	if _, err := s.Settings(context.Background()); !errors.Is(err, ErrNotInstalled) {
		t.Error("settings without install")
	}
	if err := s.Unban(context.Background(), "sshd", "1.2.3.4"); !errors.Is(err, ErrNotInstalled) {
		t.Error("unban without install")
	}
}
