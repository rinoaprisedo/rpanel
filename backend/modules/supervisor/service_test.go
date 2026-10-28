package supervisor

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

type call struct {
	name string
	args []string
}

type fake struct {
	calls   []call
	outputs map[string]string
	failOut string // output "ERROR ..." for this key
	exit    map[string]bool
}

func (f *fake) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.calls = append(f.calls, call{name, args})
	key := strings.Join(args, " ")
	if f.failOut == key {
		return system.Result{Stdout: "ERROR: CANT_REREAD: section [program:x] bad"}, nil
	}
	if f.exit[key] {
		return system.Result{Stdout: f.outputs[key], ExitCode: 3}, errors.New("exit 3")
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

const statusOut = `queue-worker:queue-worker_00     RUNNING   pid 101, uptime 0:00:12
queue-worker:queue-worker_01     RUNNING   pid 102, uptime 0:00:12
scheduler                        STOPPED   Oct 28 10:00 PM
legacy                           FATAL     Exited too quickly (process log may have details)
`

func setup(t *testing.T) (*Service, *fake, Paths) {
	t.Helper()
	dir := t.TempDir()
	p := Paths{ConfDir: filepath.Join(dir, "conf.d"), LogDir: filepath.Join(dir, "log")}
	os.MkdirAll(p.ConfDir, 0o755)
	f := &fake{outputs: map[string]string{"version": "4.2.5", "status": statusOut}, exit: map[string]bool{"status": true}}
	s := NewService(f, p)
	s.installed = func() bool { return true }
	s.userExist = func(u string) bool { return u == "root" || u == "www-data" }
	return s, f, p
}

func good() Config {
	return Config{Name: "mailer", Command: "php /www/app/artisan queue:work", Directory: "/www/app", User: "www-data", NumProcs: 1, AutoStart: true, AutoRestart: "true",
		Environment: []string{"APP_ENV=production", `GREETING=say "hi" 100%`}}
}

func sched() Config { c := good(); c.Name = "scheduler"; return c }

func TestParseStatus(t *testing.T) {
	ps := parseStatus(statusOut)
	if len(ps) != 4 || ps[0].Name != "queue-worker:queue-worker_00" || ps[0].Program != "queue-worker" || ps[0].State != "RUNNING" || ps[0].Detail != "pid 101, uptime 0:00:12" {
		t.Fatalf("%+v", ps)
	}
	if ps[2].Program != "scheduler" || ps[2].State != "STOPPED" || ps[3].State != "FATAL" {
		t.Errorf("%+v", ps)
	}
	if len(parseStatus("unix:///var/run/supervisor.sock no such file")) != 0 {
		t.Error("error text must not parse as processes")
	}
}

func TestRenderParseRoundTrip(t *testing.T) {
	c := good()
	c.NumProcs = 3
	text := render(c)
	for _, want := range []string{"[program:mailer]", "numprocs=3", "process_name=%(program_name)s_%(process_num)02d", "autorestart=true", "user=www-data", `GREETING='say "hi" 100%%'`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	got, ok := parseConf(text)
	if !ok || !reflect.DeepEqual(got, c) {
		t.Errorf("round trip\n got  %+v\n want %+v", got, c)
	}
	c2 := good()
	c2.Command = "echo 50%"
	if txt := render(c2); !strings.Contains(txt, "command=echo 50%%") {
		t.Errorf("a percent sign must be escaped for supervisord:\n%s", txt)
	}
}

func TestEnvironmentQuotingRoundTrips(t *testing.T) {
	for _, v := range []string{`plain`, `with space`, `back\slash`, `say "hi"`, `it's`, `100%`, `a,b=c`} {
		c := good()
		c.Environment = []string{"K=" + v, "OTHER=x"}
		got, ok := parseConf(render(c))
		if !ok || len(got.Environment) != 2 || got.Environment[0] != "K="+v {
			t.Errorf("value %q did not round trip: %+v", v, got.Environment)
		}
	}
}

func TestValidateRejectsInjection(t *testing.T) {
	s, _, _ := setup(t)
	mut := []func(*Config){
		func(c *Config) { c.Name = "Bad Name" },
		func(c *Config) { c.Name = "x]\n[program:evil" },
		func(c *Config) { c.Command = "" },
		func(c *Config) { c.Command = "echo hi\n[program:evil]\ncommand=rm -rf /" },
		func(c *Config) { c.Directory = "relative/path" },
		func(c *Config) { c.Directory = "/a/../etc" },
		func(c *Config) { c.User = "nobody-such-user" },
		func(c *Config) { c.User = "Root;id" },
		func(c *Config) { c.NumProcs = 0 },
		func(c *Config) { c.NumProcs = 99 },
		func(c *Config) { c.AutoRestart = "sometimes" },
		func(c *Config) { c.Environment = []string{"BAD KEY=1"} },
		func(c *Config) { c.Environment = []string{"A=1\nB=2"} },
		func(c *Config) { c.Environment = []string{"noequals"} },
		func(c *Config) { c.Environment = []string{`MIX=it's "both"`} },
	}
	for i, m := range mut {
		c := good()
		m(&c)
		if err := s.Validate(&c); !errors.Is(err, ErrBadInput) {
			t.Errorf("case %d should be rejected, got %v", i, err)
		}
	}
	c := good()
	if err := s.Validate(&c); err != nil {
		t.Errorf("good config rejected: %v", err)
	}
}

func TestSaveCreatesFileAndReloads(t *testing.T) {
	s, f, p := setup(t)
	if _, err := s.Save(context.Background(), good(), true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p.ConfDir, "rpanel-mailer.conf"))
	if err != nil || !strings.Contains(string(b), "[program:mailer]") {
		t.Fatalf("file: %v %s", err, b)
	}
	if !f.did("reread") || !f.did("update") {
		t.Errorf("expected reread + update: %v", f.calls)
	}
	// a second create with the same name is refused
	if _, err := s.Save(context.Background(), good(), true); !errors.Is(err, ErrBadInput) {
		t.Errorf("duplicate: %v", err)
	}
	// update changes it
	c := good()
	c.Command = "php artisan other"
	if _, err := s.Save(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(p.ConfDir, "rpanel-mailer.conf"))
	if !strings.Contains(string(b), "php artisan other") {
		t.Error("not updated")
	}
	if _, err := s.Save(context.Background(), Config{Name: "ghost", Command: "x", NumProcs: 1, AutoRestart: "true"}, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("update of an unknown program: %v", err)
	}
}

func TestSaveRollsBackWhenSupervisorRejects(t *testing.T) {
	s, f, p := setup(t)
	f.failOut = "reread"
	if _, err := s.Save(context.Background(), good(), true); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.ConfDir, "rpanel-mailer.conf")); err == nil {
		t.Error("a rejected new program must not stay on disk")
	}
	// an update that is rejected restores the previous file
	f.failOut = ""
	if _, err := s.Save(context.Background(), good(), true); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(p.ConfDir, "rpanel-mailer.conf"))
	f.failOut = "reread"
	c := good()
	c.Command = "broken"
	if _, err := s.Save(context.Background(), c, false); err == nil {
		t.Fatal("expected rejection")
	}
	after, _ := os.ReadFile(filepath.Join(p.ConfDir, "rpanel-mailer.conf"))
	if string(before) != string(after) {
		t.Error("previous config not restored")
	}
}

func TestCreateRefusesNamesDefinedOutsideThePanel(t *testing.T) {
	s, _, _ := setup(t)
	c := good()
	c.Name = "legacy" // appears in `supervisorctl status`, but has no rpanel- file
	if _, err := s.Save(context.Background(), c, true); !errors.Is(err, ErrBadInput) || !strings.Contains(err.Error(), "outside the panel") {
		t.Fatalf("got %v", err)
	}
}

func TestStatusMarksManagedPrograms(t *testing.T) {
	s, _, p := setup(t)
	os.WriteFile(filepath.Join(p.ConfDir, "rpanel-scheduler.conf"), []byte(render(sched())), 0o644)
	st := s.Status(context.Background())
	if !st.Running || st.Version != "4.2.5" || len(st.Processes) != 4 {
		t.Fatalf("%+v", st)
	}
	for _, pr := range st.Processes {
		switch pr.Program {
		case "scheduler":
			if !pr.Managed || pr.Config == nil || pr.Config.Command == "" {
				t.Errorf("scheduler should be managed: %+v", pr)
			}
		case "legacy", "queue-worker":
			if pr.Managed {
				t.Errorf("%s was not created by the panel", pr.Program)
			}
		}
	}
	// configured by the panel but unknown to supervisord yet
	os.WriteFile(filepath.Join(p.ConfDir, "rpanel-newone.conf"), []byte(render(Config{Name: "newone", Command: "x", NumProcs: 1, AutoRestart: "true", AutoStart: true})), 0o644)
	st = s.Status(context.Background())
	found := false
	for _, pr := range st.Processes {
		if pr.Program == "newone" && pr.State == "NOT LOADED" && pr.Managed {
			found = true
		}
	}
	if !found {
		t.Error("an unloaded managed program must still be listed")
	}
}

func TestStatusWhenNotInstalledOrStopped(t *testing.T) {
	s, f, _ := setup(t)
	s.installed = func() bool { return false }
	if st := s.Status(context.Background()); st.Installed || len(f.calls) != 0 {
		t.Error("must not call supervisorctl when not installed")
	}
	s.installed = func() bool { return true }
	f.outputs["status"] = "unix:///var/run/supervisor.sock refused connection"
	if st := s.Status(context.Background()); st.Running {
		t.Error("should report stopped")
	}
}

func TestActionsAndDelete(t *testing.T) {
	s, f, p := setup(t)
	ctx := context.Background()
	if _, err := s.Action(ctx, "restart", "queue-worker:queue-worker_00"); err != nil || !f.did("restart queue-worker:queue-worker_00") {
		t.Errorf("restart: %v", err)
	}
	for _, bad := range [][2]string{{"rm", "x"}, {"start", "x;rm"}, {"start", "../x"}, {"start", ""}} {
		if _, err := s.Action(ctx, bad[0], bad[1]); !errors.Is(err, ErrBadInput) {
			t.Errorf("%v should be refused: %v", bad, err)
		}
	}
	f.outputs["start scheduler"] = "scheduler: ERROR (spawn error)"
	if _, err := s.Action(ctx, "start", "scheduler"); err == nil {
		t.Error("an ERROR answer must surface")
	}
	os.WriteFile(filepath.Join(p.ConfDir, "rpanel-scheduler.conf"), []byte(render(sched())), 0o644)
	if err := s.Delete(ctx, "scheduler"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.ConfDir, "rpanel-scheduler.conf")); err == nil {
		t.Error("file should be gone")
	}
	if !f.did("stop scheduler:*") || !f.did("update") {
		t.Errorf("delete should stop and update: %v", f.calls)
	}
	if err := s.Delete(ctx, "legacy"); !errors.Is(err, ErrNotFound) {
		t.Errorf("only panel-managed programs can be deleted: %v", err)
	}
	if err := s.Delete(ctx, "../x"); !errors.Is(err, ErrBadInput) {
		t.Errorf("bad name: %v", err)
	}
}

func TestLogAndDaemonValidation(t *testing.T) {
	s, f, _ := setup(t)
	f.outputs["tail -20000 scheduler stderr"] = "boom\n"
	out, err := s.Log(context.Background(), "scheduler", "stderr", 0)
	if err != nil || out != "boom\n" {
		t.Errorf("log: %q %v", out, err)
	}
	if _, err := s.Log(context.Background(), "scheduler", "../x", 0); !errors.Is(err, ErrBadInput) {
		t.Error("bad stream")
	}
	if err := s.Daemon(context.Background(), "restart"); err != nil {
		t.Error(err)
	}
	if err := s.Daemon(context.Background(), "rm"); !errors.Is(err, ErrBadInput) {
		t.Error("bad action")
	}
	s.installed = func() bool { return false }
	if _, err := s.Action(context.Background(), "start", "x"); !errors.Is(err, ErrNotInstalled) {
		t.Error("not installed")
	}
}
