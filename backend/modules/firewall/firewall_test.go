package firewall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"rpanel/backend/system"
)

const statusOut = `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 80,443/tcp                 ALLOW IN    Anywhere
[ 3] 3306/tcp                   DENY IN     10.0.0.5
[ 4] 22/tcp (v6)                ALLOW IN    Anywhere (v6)
[ 5] 8000:8100/tcp              ALLOW IN    192.168.1.0/24
[ 6] Anywhere                   ALLOW OUT   10.1.1.1
`

const verboseOut = `Status: active
Logging: on (low)
Default: deny (incoming), allow (outgoing), disabled (routed)
New profiles: skip
`

type call struct{ args []string }

type fake struct {
	calls  []call
	status string
	failOn string // first ufw arg that should fail
}

func (f *fake) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	if name != "ufw" {
		f.calls = append(f.calls, call{append([]string{name}, args...)})
		return system.Result{}, nil
	}
	f.calls = append(f.calls, call{args})
	if len(args) >= 2 && args[0] == "status" {
		if args[1] == "numbered" {
			return system.Result{Stdout: f.status}, nil
		}
		return system.Result{Stdout: verboseOut}, nil
	}
	if f.failOn != "" && len(args) > 0 && args[0] == f.failOn {
		return system.Result{Stderr: "ERROR: boom"}, errors.New("exit 1")
	}
	return system.Result{}, nil
}

func (f *fake) mutations() [][]string {
	var out [][]string
	for _, c := range f.calls {
		if len(c.args) > 0 && c.args[0] != "status" {
			out = append(out, c.args)
		}
	}
	return out
}

func newSvc(t *testing.T, sshd string) (*Service, *fake) {
	t.Helper()
	conf := filepath.Join(t.TempDir(), "sshd_config")
	os.WriteFile(conf, []byte(sshd), 0o644)
	f := &fake{status: statusOut}
	return NewService(f, conf, "8800", []string{"9999", "bad", "70000"}), f
}

func TestParseRules(t *testing.T) {
	rules := ParseRules(statusOut)
	if len(rules) != 6 {
		t.Fatalf("got %d rules: %+v", len(rules), rules)
	}
	if r := rules[1]; r.Number != 2 || r.To != "80,443/tcp" || r.Action != "ALLOW" || r.From != "Anywhere" || r.V6 {
		t.Fatalf("rule 2 = %+v", r)
	}
	if r := rules[2]; r.Action != "DENY" || r.From != "10.0.0.5" {
		t.Fatalf("rule 3 = %+v", r)
	}
	if !rules[3].V6 {
		t.Fatal("rule 4 must be v6")
	}
	if rules[5].Direction != "OUT" {
		t.Fatalf("rule 6 = %+v", rules[5])
	}
	if got := ParseRules("Status: inactive\n"); len(got) != 0 {
		t.Fatal("inactive output has no rules")
	}
}

func TestStatus(t *testing.T) {
	s, _ := newSvc(t, "Port 2222\n")
	st := s.Status(context.Background())
	if !st.Installed || !st.Active || st.Incoming != "deny" || st.Outgoing != "allow" || len(st.Rules) != 6 {
		t.Fatalf("status %+v", st)
	}
	if !reflect.DeepEqual(st.Protected, []int{2222, 8800, 9999}) {
		t.Fatalf("protected = %v", st.Protected)
	}
}

func TestProtectedPortsDefaultsToSSH22(t *testing.T) {
	s, _ := newSvc(t, "# no port line\nPermitRootLogin no\n")
	if got := s.ProtectedPorts(); !reflect.DeepEqual(got, []int{22, 8800, 9999}) {
		t.Fatalf("got %v", got)
	}
	s, _ = newSvc(t, "Port 22\nPort 2200\n#Port 3333\n")
	if got := s.ProtectedPorts(); !reflect.DeepEqual(got, []int{22, 2200, 8800, 9999}) {
		t.Fatalf("got %v", got)
	}
}

func TestEnableOpensProtectedPortsFirst(t *testing.T) {
	s, f := newSvc(t, "Port 22\n")
	if err := s.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := f.mutations()
	want := [][]string{{"allow", "22/tcp"}, {"allow", "8800/tcp"}, {"allow", "9999/tcp"}, {"--force", "enable"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order\n got %v\nwant %v", got, want)
	}
	// If opening a protected port fails, the firewall must not be enabled.
	s, f = newSvc(t, "Port 22\n")
	f.failOn = "allow"
	if err := s.Enable(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	for _, m := range f.mutations() {
		if m[0] == "--force" {
			t.Fatal("must not enable after a failed allow")
		}
	}
}

func TestDefaultIncoming(t *testing.T) {
	s, f := newSvc(t, "Port 22\n")
	if err := s.SetDefaultIncoming(context.Background(), "deny"); err != nil {
		t.Fatal(err)
	}
	m := f.mutations()
	if last := m[len(m)-1]; !reflect.DeepEqual(last, []string{"default", "deny", "incoming"}) || len(m) != 4 {
		t.Fatalf("mutations %v", m)
	}
	f.calls = nil
	if err := s.SetDefaultIncoming(context.Background(), "allow"); err != nil || len(f.mutations()) != 1 {
		t.Fatalf("allow policy needs no protected rules: %v %v", err, f.mutations())
	}
	if err := s.SetDefaultIncoming(context.Background(), "drop; reboot"); !errors.Is(err, ErrBadInput) {
		t.Fatalf("bad policy: %v", err)
	}
}

func TestAddRuleArgv(t *testing.T) {
	s, f := newSvc(t, "Port 22\n")
	ctx := context.Background()
	cases := []struct {
		in   RuleInput
		want string
	}{
		{RuleInput{Action: "allow", Port: "80", Proto: "tcp"}, "allow in proto tcp from any to any port 80"},
		{RuleInput{Action: "allow", Port: "53", Proto: "any", Source: "any"}, "allow in from any to any port 53"},
		{RuleInput{Action: "deny", Port: "3306", Proto: "tcp", Source: "10.0.0.0/8", Comment: "block db"}, "deny in proto tcp from 10.0.0.0/8 to any port 3306 comment block db"},
		{RuleInput{Action: "allow", Port: "8000:8100", Proto: "udp", Source: "2001:db8::1"}, "allow in proto udp from 2001:db8::1 to any port 8000:8100"},
		{RuleInput{Action: "limit", Port: "22", Source: "any"}, "limit in proto tcp from any to any port 22"},
	}
	for _, c := range cases {
		f.calls = nil
		if err := s.AddRule(ctx, c.in, "1.2.3.4"); err != nil {
			t.Fatalf("%+v: %v", c.in, err)
		}
		if got := strings.Join(f.mutations()[0], " "); got != c.want {
			t.Errorf("argv %q want %q", got, c.want)
		}
	}
}

func TestAddRuleValidation(t *testing.T) {
	s, f := newSvc(t, "Port 22\n")
	bad := []RuleInput{
		{Action: "drop", Port: "80"},
		{Action: "allow", Port: "0"},
		{Action: "allow", Port: "70000"},
		{Action: "allow", Port: "80;reboot"},
		{Action: "allow", Port: "90:80", Proto: "tcp"},
		{Action: "allow", Port: "80:90", Proto: "any"},
		{Action: "allow", Port: "80", Proto: "icmp"},
		{Action: "allow", Port: "80", Source: "10.0.0.1; rm"},
		{Action: "allow", Port: "80", Source: "1.2.3.4/99"},
		{Action: "allow", Port: "80", Source: "example.com"},
		{Action: "allow", Port: "80", Comment: "x' ; reboot"},
		{Action: "allow", Port: "80", Comment: strings.Repeat("a", 61)},
	}
	for i, in := range bad {
		if err := s.AddRule(context.Background(), in, "1.2.3.4"); !errors.Is(err, ErrBadInput) {
			t.Errorf("case %d (%+v): want ErrBadInput, got %v", i, in, err)
		}
	}
	if len(f.mutations()) != 0 {
		t.Fatalf("no ufw call expected: %v", f.mutations())
	}
}

func TestAddRuleLockoutGuards(t *testing.T) {
	s, f := newSvc(t, "Port 22\n")
	ctx := context.Background()
	// deny SSH from anywhere
	if err := s.AddRule(ctx, RuleInput{Action: "deny", Port: "22", Proto: "tcp"}, "1.2.3.4"); !errors.Is(err, ErrLockout) {
		t.Fatalf("deny ssh from anywhere: %v", err)
	}
	// a range that contains the panel port
	if err := s.AddRule(ctx, RuleInput{Action: "reject", Port: "8000:9000", Proto: "tcp"}, "1.2.3.4"); !errors.Is(err, ErrLockout) {
		t.Fatalf("range over panel port: %v", err)
	}
	// deny SSH from a network containing the admin
	if err := s.AddRule(ctx, RuleInput{Action: "deny", Port: "22", Proto: "tcp", Source: "1.2.3.0/24"}, "1.2.3.4"); !errors.Is(err, ErrLockout) {
		t.Fatalf("deny own network: %v", err)
	}
	if len(f.mutations()) != 0 {
		t.Fatal("blocked rules must not reach ufw")
	}
	// deny SSH from someone else is fine
	if err := s.AddRule(ctx, RuleInput{Action: "deny", Port: "22", Proto: "tcp", Source: "5.6.7.8"}, "1.2.3.4"); err != nil {
		t.Fatalf("deny other host: %v", err)
	}
	// deny a non-protected port from anywhere is fine
	if err := s.AddRule(ctx, RuleInput{Action: "deny", Port: "3306", Proto: "tcp"}, "1.2.3.4"); err != nil {
		t.Fatalf("deny mysql: %v", err)
	}
	// explicit confirmation overrides
	if err := s.AddRule(ctx, RuleInput{Action: "deny", Port: "22", Proto: "tcp", Confirm: true}, "1.2.3.4"); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
}

func TestDeleteRule(t *testing.T) {
	s, f := newSvc(t, "Port 22\n")
	ctx := context.Background()
	// rule 3 (deny mysql from 10.0.0.5): fine
	if err := s.DeleteRule(ctx, 3, "3306/tcp", "DENY", false); err != nil {
		t.Fatal(err)
	}
	if m := f.mutations(); !reflect.DeepEqual(m, [][]string{{"--force", "delete", "3"}}) {
		t.Fatalf("mutations %v", m)
	}
	f.calls = nil
	// stale: list shifted, rule 3 no longer what the UI saw
	if err := s.DeleteRule(ctx, 3, "80,443/tcp", "ALLOW", false); !errors.Is(err, ErrStale) {
		t.Fatalf("stale: %v", err)
	}
	if err := s.DeleteRule(ctx, 99, "x", "ALLOW", false); !errors.Is(err, ErrStale) {
		t.Fatalf("missing: %v", err)
	}
	// allow rule for SSH needs confirmation
	if err := s.DeleteRule(ctx, 1, "22/tcp", "ALLOW", false); !errors.Is(err, ErrLockout) {
		t.Fatalf("ssh allow: %v", err)
	}
	if err := s.DeleteRule(ctx, 4, "22/tcp (v6)", "ALLOW", false); !errors.Is(err, ErrLockout) {
		t.Fatalf("ssh v6 allow: %v", err)
	}
	if len(f.mutations()) != 0 {
		t.Fatalf("guarded deletes must not reach ufw: %v", f.mutations())
	}
	if err := s.DeleteRule(ctx, 1, "22/tcp", "ALLOW", true); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
	// a range containing a protected port is guarded too (rule 5 covers 8000-8100 but not 8800 → allowed)
	f.calls = nil
	if err := s.DeleteRule(ctx, 5, "8000:8100/tcp", "ALLOW", false); err != nil {
		t.Fatalf("range without protected port: %v", err)
	}
	if err := s.DeleteRule(ctx, 0, "", "", false); !errors.Is(err, ErrBadInput) {
		t.Fatalf("number 0: %v", err)
	}
}

func TestStatusNotInstalled(t *testing.T) {
	s := NewService(notFound{}, "/nonexistent", "8800", nil)
	st := s.Status(context.Background())
	if st.Installed {
		t.Fatalf("%+v", st)
	}
}

type notFound struct{}

func (notFound) Run(context.Context, time.Duration, string, ...string) (system.Result, error) {
	return system.Result{}, errors.New(`ufw: exec: "ufw": executable file not found in $PATH`)
}
