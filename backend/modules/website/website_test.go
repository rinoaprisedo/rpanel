package website

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
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

func TestListDirsStaysInsideWebRoot(t *testing.T) {
	e := setup(t)
	for _, d := range []string{"site-a", "site-b/public", ".hidden"} {
		os.MkdirAll(filepath.Join(e.root, d), 0o755)
	}
	os.WriteFile(filepath.Join(e.root, "file.txt"), nil, 0o644)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.root, "escape")); err != nil {
		t.Fatal(err)
	}

	top, err := e.svc.ListDirs("")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range top.Dirs {
		names = append(names, d.Name)
	}
	if strings.Join(names, ",") != "site-a,site-b" {
		t.Errorf("want only real, visible folders; got %v", names)
	}
	if top.Parent != "" || top.Path != filepath.Clean(e.root) {
		t.Errorf("web root listing: %+v", top)
	}

	sub, err := e.svc.ListDirs(filepath.Join(e.root, "site-b"))
	if err != nil || sub.Parent != filepath.Clean(e.root) || len(sub.Dirs) != 1 || sub.Dirs[0].Name != "public" {
		t.Errorf("sub listing: %+v err=%v", sub, err)
	}

	for _, p := range []string{"/etc", filepath.Join(e.root, ".."), filepath.Join(e.root, "escape"), "relative"} {
		if _, err := e.svc.ListDirs(p); err == nil {
			t.Errorf("%q must not be browsable", p)
		}
	}
	if _, err := e.svc.ListDirs(filepath.Join(e.root, "missing")); !errors.Is(err, ErrBadInput) {
		t.Errorf("missing folder: %v", err)
	}
}

func TestProxyRulesRenderAndOverlayAnySiteType(t *testing.T) {
	ng := &Nginx{LogDir: "/var/log/nginx", AcmeRoot: "/var/www/letsencrypt"}
	w := &Website{Domain: "a.com", Type: TypeStatic, RootDir: "/www/a", SSLEnabled: true, ProxyRules: []ProxyRule{
		{Name: "api", Path: "/api/", Target: "http://127.0.0.1:3000", PassHost: true, Enabled: true},
		{Name: "ext", Path: "/ext/", Target: "https://api.example.com", PassHost: false, Enabled: true},
		{Name: "off", Path: "/old/", Target: "http://127.0.0.1:4000", Enabled: false},
	}}
	out, err := ng.Render(w)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "location /api/ {") != 2 { // both :80 and :443 serve the site
		t.Errorf("rule must be present in both server blocks:\n%s", out)
	}
	if !strings.Contains(out, "proxy_pass https://api.example.com;") || !strings.Contains(out, "proxy_set_header Host $proxy_host;") || !strings.Contains(out, "proxy_ssl_server_name on;") {
		t.Errorf("target-host rule not rendered properly:\n%s", out)
	}
	if strings.Contains(out, "/old/") {
		t.Errorf("disabled rule must not be rendered:\n%s", out)
	}

	w.ForceHTTPS = true
	out, _ = ng.Render(w)
	if strings.Count(out, "location /api/ {") != 1 {
		t.Errorf("with force-HTTPS the plain-HTTP server must only redirect, never proxy:\n%s", out)
	}
}

func TestProxyRuleValidation(t *testing.T) {
	ok := ProxyRule{Path: "/api/", Target: "http://127.0.0.1:3000", Enabled: true}
	if err := validateRules([]ProxyRule{ok}); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	bad := map[string]ProxyRule{
		"root path":        {Path: "/", Target: ok.Target},
		"no leading slash": {Path: "api", Target: ok.Target},
		"injection":        {Path: "/a { } server {", Target: ok.Target},
		"semicolon":        {Path: "/a;b", Target: ok.Target},
		"traversal":        {Path: "/a/../b", Target: ok.Target},
		"acme path":        {Path: "/.well-known/x", Target: ok.Target},
		"file target":      {Path: "/a/", Target: "file:///etc/passwd"},
		"target injection": {Path: "/a/", Target: "http://x;rm -rf"},
		"newline in name":  {Path: "/a/", Target: ok.Target, Name: "x\nserver {"},
	}
	for name, r := range bad {
		if err := validateRules([]ProxyRule{r}); !errors.Is(err, ErrBadInput) {
			t.Errorf("%s: want ErrBadInput, got %v", name, err)
		}
	}
	if err := validateRules([]ProxyRule{ok, ok}); err == nil {
		t.Error("duplicate path must be rejected")
	}
}

func TestExtraConfigValidation(t *testing.T) {
	good := []string{
		"client_max_body_size 100m;",
		"location /static/ {\n  expires 30d;\n}",
		"# a } in a comment\nadd_header X-A \"}\";",
		"set $x ${arg_id};",
	}
	for _, g := range good {
		if err := validateExtra(g); err != nil {
			t.Errorf("good config rejected %q: %v", g, err)
		}
	}
	bad := []string{
		"} server { listen 80; server_name evil.com; location / { return 200; } ",
		"location / {",
		"add_header X \"unterminated;",
		"a\x00b",
		strings.Repeat("a", maxExtraConfig+1),
	}
	for _, b := range bad {
		if err := validateExtra(b); err == nil {
			t.Errorf("bad config accepted: %.40q", b)
		}
	}
}

func TestSetExtraConfigAppliesAndRollsBack(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	w, err := e.svc.Create(ctx, CreateInput{Domain: "a.com", Type: TypeStatic})
	if err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(e.avail, "rpanel-a.com.conf")

	if _, err := e.svc.SetExtraConfig(ctx, w.UUID.String(), "client_max_body_size 100m;\r\n", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(conf)
	if !strings.Contains(string(b), "client_max_body_size 100m;") {
		t.Fatalf("directive missing from the vhost:\n%s", b)
	}
	cfg, _ := e.svc.Config(w.UUID.String())
	if cfg.ExtraConfig != "client_max_body_size 100m;" || !strings.Contains(cfg.Rendered, "server_name a.com;") {
		t.Errorf("config view: %+v", cfg)
	}

	e.run.failTest = true // nginx rejects the directive
	if _, err := e.svc.SetExtraConfig(ctx, w.UUID.String(), "bogus_directive on;", nil); err == nil {
		t.Fatal("nginx -t failure must surface")
	}
	e.run.failTest = false
	after, _ := os.ReadFile(conf)
	if strings.Contains(string(after), "bogus_directive") {
		t.Error("a rejected config must not stay on disk")
	}
	if got, _ := e.svc.Config(w.UUID.String()); got.ExtraConfig != "client_max_body_size 100m;" {
		t.Errorf("a rejected config must not be stored: %q", got.ExtraConfig)
	}
}

func TestSetProxyRulesPersistsAndSurvivesOtherUpdates(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	w, _ := e.svc.Create(ctx, CreateInput{Domain: "a.com", Type: TypeStatic})
	rules := []ProxyRule{{Name: " api ", Path: " /api/ ", Target: "http://127.0.0.1:3000", PassHost: true, Enabled: true}}
	got, err := e.svc.SetProxyRules(ctx, w.UUID.String(), rules, nil)
	if err != nil || got.ProxyRules[0].Path != "/api/" || got.ProxyRules[0].Name != "api" {
		t.Fatalf("rules not stored/trimmed: %+v err=%v", got, err)
	}
	// Regenerating the vhost for an unrelated reason must keep the rules and custom config.
	yes := true
	e.svc.SetExtraConfig(ctx, w.UUID.String(), "client_max_body_size 5m;", nil)
	e.svc.Update(ctx, w.UUID.String(), UpdateInput{ForceHTTPS: &yes}, nil) // rejected: no SSL, must not wipe anything
	e.svc.SetStatus(ctx, w.UUID.String(), StatusStopped, nil)
	e.svc.SetStatus(ctx, w.UUID.String(), StatusRunning, nil)
	b, _ := os.ReadFile(filepath.Join(e.avail, "rpanel-a.com.conf"))
	if !strings.Contains(string(b), "location /api/ {") || !strings.Contains(string(b), "client_max_body_size 5m;") {
		t.Errorf("rules/custom config lost after regenerating:\n%s", b)
	}
	if _, err := e.svc.SetProxyRules(ctx, w.UUID.String(), []ProxyRule{{Path: "/", Target: "http://x:1"}}, nil); !errors.Is(err, ErrBadInput) {
		t.Errorf("invalid rule: %v", err)
	}
	if again, _ := e.svc.Get(w.UUID.String()); len(again.ProxyRules) != 1 {
		t.Errorf("an invalid rule set must not replace the stored one: %+v", again.ProxyRules)
	}
}

func selfSigned(t *testing.T, dir, domain string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, Issuer: pkix.Name{CommonName: "Test CA"},
		DNSNames: []string{domain}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, domain), 0o755)
	os.WriteFile(filepath.Join(dir, domain, "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

func TestSSLStatusAndDisable(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	certs := t.TempDir()
	e.svc.cfg.CertDir = certs
	w, _ := e.svc.Create(ctx, CreateInput{Domain: "a.com", Type: TypeStatic})

	st, err := e.svc.SSLStatus(w.UUID.String())
	if err != nil || st.Enabled || st.Cert != nil {
		t.Fatalf("fresh site: %+v err=%v", st, err)
	}
	selfSigned(t, certs, "a.com")
	if _, err := e.svc.IssueSSL(ctx, w.UUID.String(), "", nil); err != nil {
		t.Fatal(err)
	}
	e.svc.Update(ctx, w.UUID.String(), UpdateInput{ForceHTTPS: func() *bool { b := true; return &b }()}, nil)

	st, _ = e.svc.SSLStatus(w.UUID.String())
	if !st.Enabled || !st.ForceHTTPS || st.Cert == nil || st.Cert.Issuer != "a.com" || st.Cert.DaysLeft < 29 || st.Cert.Names[0] != "a.com" {
		t.Errorf("status with certificate: %+v cert=%+v", st, st.Cert)
	}

	got, err := e.svc.DisableSSL(ctx, w.UUID.String(), nil)
	if err != nil || got.SSLEnabled || got.ForceHTTPS {
		t.Fatalf("disable: %+v err=%v", got, err)
	}
	b, _ := os.ReadFile(filepath.Join(e.avail, "rpanel-a.com.conf"))
	if strings.Contains(string(b), "443") || strings.Contains(string(b), "return 301") {
		t.Errorf("vhost must drop HTTPS and the redirect:\n%s", b)
	}
	if st, _ := e.svc.SSLStatus(w.UUID.String()); st.Cert == nil {
		t.Error("the certificate file must be kept so SSL can be re-enabled")
	}
}

func TestCreateRejectsPanelDomain(t *testing.T) {
	e := setup(t)
	tls := filepath.Join(t.TempDir(), "panel-tls.conf")
	os.WriteFile(tls, []byte("# rpanel-panel-ssl mode=letsencrypt domain=panel.example.com\n"), 0o644)
	e.svc.cfg.PanelTLSConf = tls
	_, err := e.svc.Create(context.Background(), CreateInput{Domain: "Panel.Example.com", Type: TypeStatic})
	if !errors.Is(err, ErrBadInput) || !strings.Contains(err.Error(), "panel's own domain") {
		t.Fatalf("expected the panel domain to be refused, got %v", err)
	}
	if e.run.count("nginx", "-t") != 0 {
		t.Error("nothing should have been applied")
	}
}
