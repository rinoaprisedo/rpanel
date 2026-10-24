package website

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

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type call struct {
	name string
	args []string
}

type fakeRunner struct {
	mu       sync.Mutex
	calls    []call
	failTest bool // make `nginx -t` fail
	failCert bool
}

func (f *fakeRunner) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{name, args})
	if name == "nginx" && len(args) > 0 && args[0] == "-t" && f.failTest {
		return system.Result{Stderr: "unexpected }", ExitCode: 1}, errors.New("exit 1")
	}
	if name == "certbot" && f.failCert {
		return system.Result{Stderr: "challenge failed", ExitCode: 1}, errors.New("exit 1")
	}
	return system.Result{}, nil
}

func (f *fakeRunner) count(name string, first string) int {
	n := 0
	for _, c := range f.calls {
		if c.name == name && (first == "" || (len(c.args) > 0 && c.args[0] == first)) {
			n++
		}
	}
	return n
}

type env struct {
	svc            *Service
	run            *fakeRunner
	root           string
	avail, enabled string
}

func setup(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{run: &fakeRunner{}, root: filepath.Join(dir, "www"), avail: filepath.Join(dir, "av"), enabled: filepath.Join(dir, "en")}
	os.MkdirAll(e.avail, 0o755)
	os.MkdirAll(e.enabled, 0o755)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Website{}); err != nil {
		t.Fatal(err)
	}
	ng := &Nginx{Run: e.run, Available: e.avail, Enabled: e.enabled, LogDir: "/var/log/nginx", AcmeRoot: filepath.Join(dir, "acme")}
	e.svc = NewService(db, ng, e.run, Settings{WebRoot: e.root, AcmeRoot: ng.AcmeRoot, LogDir: filepath.Join(dir, "logs"), CertbotEmail: "a@example.com"})
	return e
}

func TestValidateRejectsInjection(t *testing.T) {
	bad := []*Website{
		{Domain: "a.com;x", Type: TypeStatic, RootDir: "/www/a"},
		{Domain: "a.com", Type: TypeStatic, RootDir: "/www/a; } server {"},
		{Domain: "a.com", Type: TypeStatic, RootDir: "/www/a b"},
		{Domain: "a.com", Type: TypeStatic, RootDir: "/www/$x"},
		{Domain: "a.com", Type: TypeStatic, RootDir: "/www/../etc"},
		{Domain: "a.com", Type: TypePHP, RootDir: "/www/a", PHPVersion: "8.3;x"},
		{Domain: "a.com", Type: TypeProxy, ProxyTarget: "http://x;rm"},
		{Domain: "a.com", Type: TypeProxy, ProxyTarget: "file:///etc/passwd"},
		{Domain: "a.com", Type: TypeProxy, ProxyTarget: "http://127.0.0.1:3000\nfoo"},
		{Domain: "a.com", Type: "weird"},
	}
	for i, w := range bad {
		if err := Validate(w); err == nil {
			t.Errorf("case %d should be rejected: %+v", i, w)
		}
	}
	good := []*Website{
		{Domain: "a.com", Type: TypeStatic, RootDir: "/www/wwwroot/a.com"},
		{Domain: "a.com", Type: TypePHP, RootDir: "/www/a", PHPVersion: "8.3"},
		{Domain: "a.com", Type: TypeProxy, ProxyTarget: "http://127.0.0.1:3000"},
		{Domain: "a.com", Type: TypeProxy, ProxyTarget: "https://backend.internal:8443/app"},
	}
	for i, w := range good {
		if err := Validate(w); err != nil {
			t.Errorf("good case %d rejected: %v", i, err)
		}
	}
}

func TestRenderVariants(t *testing.T) {
	ng := &Nginx{LogDir: "/var/log/nginx", AcmeRoot: "/var/www/letsencrypt"}
	php, _ := ng.Render(&Website{Domain: "a.com", Type: TypePHP, RootDir: "/www/a", PHPVersion: "8.3"})
	if !strings.Contains(php, "php8.3-fpm.sock") || !strings.Contains(php, "root /www/a;") || strings.Contains(php, "443") {
		t.Fatalf("php render wrong:\n%s", php)
	}
	px, _ := ng.Render(&Website{Domain: "a.com", Type: TypeProxy, ProxyTarget: "http://127.0.0.1:3000"})
	if !strings.Contains(px, "proxy_pass http://127.0.0.1:3000;") {
		t.Fatalf("proxy render wrong:\n%s", px)
	}
	ssl, _ := ng.Render(&Website{Domain: "a.com", Type: TypeStatic, RootDir: "/www/a", SSLEnabled: true, ForceHTTPS: true})
	if !strings.Contains(ssl, "listen 443 ssl") || !strings.Contains(ssl, "return 301 https://") || !strings.Contains(ssl, "/etc/letsencrypt/live/a.com/fullchain.pem") {
		t.Fatalf("ssl render wrong:\n%s", ssl)
	}
	if strings.Count(ssl, "root /www/a;") != 1 { // body only on 443 when forcing https
		t.Fatalf("force https must not serve content on :80:\n%s", ssl)
	}
}

func TestCreateWritesConfLinksAndReloads(t *testing.T) {
	e := setup(t)
	w, err := e.svc.Create(context.Background(), CreateInput{Domain: "Example.COM", Type: TypeStatic})
	if err != nil {
		t.Fatal(err)
	}
	if w.Domain != "example.com" || w.RootDir != filepath.Join(e.root, "example.com") {
		t.Fatalf("unexpected site %+v", w)
	}
	if _, err := os.Stat(filepath.Join(e.avail, "rpanel-example.com.conf")); err != nil {
		t.Fatal("conf missing")
	}
	if _, err := os.Lstat(filepath.Join(e.enabled, "rpanel-example.com.conf")); err != nil {
		t.Fatal("symlink missing")
	}
	if _, err := os.Stat(filepath.Join(w.RootDir, "index.html")); err != nil {
		t.Fatal("placeholder missing")
	}
	if e.run.count("nginx", "-t") != 1 || e.run.count("systemctl", "reload") != 1 {
		t.Fatalf("expected one test + one reload: %+v", e.run.calls)
	}
	if _, err := e.svc.Create(context.Background(), CreateInput{Domain: "example.com", Type: TypeStatic}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestCreateRollsBackOnNginxTestFailure(t *testing.T) {
	e := setup(t)
	e.run.failTest = true
	if _, err := e.svc.Create(context.Background(), CreateInput{Domain: "bad.com", Type: TypeStatic}); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(filepath.Join(e.avail, "rpanel-bad.com.conf")); err == nil {
		t.Fatal("conf must be removed after failed test")
	}
	if _, err := os.Lstat(filepath.Join(e.enabled, "rpanel-bad.com.conf")); err == nil {
		t.Fatal("link must be removed after failed test")
	}
	if _, err := os.Stat(filepath.Join(e.root, "bad.com")); err == nil {
		t.Fatal("created root dir must be cleaned up")
	}
	if list, _ := e.svc.List(); len(list) != 0 {
		t.Fatal("no DB row expected")
	}
	if e.run.count("systemctl", "reload") != 0 {
		t.Fatal("must not reload after failed test")
	}
}

func TestUpdateFailureRestoresPreviousConf(t *testing.T) {
	e := setup(t)
	w, _ := e.svc.Create(context.Background(), CreateInput{Domain: "a.com", Type: TypeProxy, ProxyTarget: "http://127.0.0.1:3000"})
	before, _ := os.ReadFile(filepath.Join(e.avail, "rpanel-a.com.conf"))
	e.run.failTest = true
	target := "http://127.0.0.1:4000"
	if _, err := e.svc.Update(context.Background(), w.UUID.String(), UpdateInput{ProxyTarget: &target}, nil); err == nil {
		t.Fatal("expected failure")
	}
	after, _ := os.ReadFile(filepath.Join(e.avail, "rpanel-a.com.conf"))
	if string(before) != string(after) {
		t.Fatal("previous conf not restored")
	}
	if got, _ := e.svc.Get(w.UUID.String()); got.ProxyTarget != "http://127.0.0.1:3000" {
		t.Fatal("DB must be unchanged")
	}
}

func TestRootMustStayInsideWebRoot(t *testing.T) {
	e := setup(t)
	for _, r := range []string{"/etc", filepath.Join(e.root, "..", "x"), "relative"} {
		if _, err := e.svc.Create(context.Background(), CreateInput{Domain: "a.com", Type: TypeStatic, RootDir: r}); err == nil {
			t.Errorf("root %q should be rejected", r)
		}
	}
}

func TestStopStartAndDelete(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	w, _ := e.svc.Create(ctx, CreateInput{Domain: "a.com", Type: TypeStatic})
	link := filepath.Join(e.enabled, "rpanel-a.com.conf")
	if _, err := e.svc.SetStatus(ctx, w.UUID.String(), StatusStopped, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("stopped site must be unlinked")
	}
	if _, err := e.svc.SetStatus(ctx, w.UUID.String(), StatusRunning, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("started site must be linked")
	}
	if _, err := e.svc.Delete(ctx, w.UUID.String(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.avail, "rpanel-a.com.conf")); err == nil {
		t.Fatal("conf must be deleted")
	}
	if _, err := os.Stat(w.RootDir); err != nil {
		t.Fatal("files must be kept unless requested")
	}
	if _, err := e.svc.Get(w.UUID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatal("row must be gone")
	}
}

func TestSSLIssueAndFailure(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	w, _ := e.svc.Create(ctx, CreateInput{Domain: "a.com", Type: TypeStatic})

	e.run.failCert = true
	if _, err := e.svc.IssueSSL(ctx, w.UUID.String(), "", nil); err == nil {
		t.Fatal("expected certbot failure")
	}
	if got, _ := e.svc.Get(w.UUID.String()); got.SSLEnabled {
		t.Fatal("ssl must stay disabled")
	}
	e.run.failCert = false
	if _, err := e.svc.IssueSSL(ctx, w.UUID.String(), "not-an-email", nil); err == nil {
		t.Fatal("bad email must be rejected")
	}
	got, err := e.svc.IssueSSL(ctx, w.UUID.String(), "", nil)
	if err != nil || !got.SSLEnabled {
		t.Fatalf("ssl issue: %v", err)
	}
	conf, _ := os.ReadFile(filepath.Join(e.avail, "rpanel-a.com.conf"))
	if !strings.Contains(string(conf), "listen 443 ssl") {
		t.Fatal("443 block missing")
	}
	force := true
	if _, err := e.svc.Update(ctx, w.UUID.String(), UpdateInput{ForceHTTPS: &force}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestForceHTTPSRequiresSSL(t *testing.T) {
	e := setup(t)
	w, _ := e.svc.Create(context.Background(), CreateInput{Domain: "a.com", Type: TypeStatic})
	force := true
	if _, err := e.svc.Update(context.Background(), w.UUID.String(), UpdateInput{ForceHTTPS: &force}, nil); !errors.Is(err, ErrBadInput) {
		t.Fatalf("want ErrBadInput, got %v", err)
	}
}

func TestLogsTail(t *testing.T) {
	e := setup(t)
	w, _ := e.svc.Create(context.Background(), CreateInput{Domain: "a.com", Type: TypeStatic})
	os.MkdirAll(e.svc.cfg.LogDir, 0o755)
	os.WriteFile(filepath.Join(e.svc.cfg.LogDir, "a.com.access.log"), []byte("1\n2\n3\n4\n5\n"), 0o644)
	got, err := e.svc.Logs(w.UUID.String(), "access", 2)
	if err != nil || got != "4\n5" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := e.svc.Logs(w.UUID.String(), "../../etc/passwd", 2); err == nil {
		t.Fatal("log type must be validated")
	}
}

func TestInstallValidatesInput(t *testing.T) {
	e := setup(t)
	for _, c := range [][2]string{{"php", "8.3;rm"}, {"mysql", ""}, {"php", ""}} {
		if err := e.svc.Install(context.Background(), c[0], c[1]); !errors.Is(err, ErrBadInput) {
			t.Errorf("%v: want ErrBadInput, got %v", c, err)
		}
	}
	if err := e.svc.Install(context.Background(), "php", "8.3"); err != nil {
		t.Fatal(err)
	}
	last := e.run.calls[len(e.run.calls)-1]
	if last.name != "apt-get" || last.args[len(last.args)-2] != "php8.3-fpm" {
		t.Fatalf("unexpected install call %+v", last)
	}
}
