package appstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rpanel/backend/system"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/gin-gonic/gin"
)

type call struct {
	name string
	args []string
}

type fakeRunner struct {
	mu        sync.Mutex
	calls     []call
	installed map[string]string // package -> Version field
	active    map[string]bool
	failApt   bool
	policy    map[string]string // package -> Candidate field
	origins   map[string]string // package -> extra apt-cache policy lines (repository URLs)
}

func (f *fakeRunner) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{name, args})
	switch {
	case name == "dpkg" && len(args) == 2 && args[0] == "-s":
		v, ok := f.installed[args[1]]
		if !ok {
			return system.Result{ExitCode: 1}, errors.New("exit 1")
		}
		return system.Result{Stdout: "Package: " + args[1] + "\nStatus: install ok installed\nVersion: " + v + "\n"}, nil
	case name == "systemctl" && len(args) == 2 && args[0] == "is-active":
		if f.active[args[1]] {
			return system.Result{Stdout: "active\n"}, nil
		}
		return system.Result{Stdout: "inactive\n", ExitCode: 3}, errors.New("exit 3")
	case name == "apt-cache" && len(args) == 2 && args[0] == "policy":
		c, ok := f.policy[args[1]]
		if !ok {
			c = "(none)"
		}
		return system.Result{Stdout: args[1] + ":\n  Installed: (none)\n  Candidate: " + c + "\n" + f.origins[args[1]]}, nil
	case name == "apt-get" && f.failApt && len(args) > 0 && args[0] == "install":
		return system.Result{Stderr: "E: broken\nE: held packages"}, errors.New("exit 100")
	}
	return system.Result{}, nil
}

func (f *fakeRunner) saw(name string, first string, contains string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.name == name && len(c.args) > 0 && c.args[0] == first && strings.Contains(strings.Join(c.args, " "), contains) {
			return true
		}
	}
	return false
}

func newSvc(f *fakeRunner, php ...string) *Service {
	s := NewService(f)
	s.phpVersions = func() []string { return php }
	return s
}

type repoEnv struct {
	apt     string
	fetched []string
	key     []byte
}

// withRepoSeams points the service at a temp /etc/apt, a fake Ubuntu noble and a fake key download.
func withRepoSeams(t *testing.T, s *Service, key []byte) *repoEnv {
	t.Helper()
	e := &repoEnv{apt: t.TempDir(), key: key}
	s.aptDir = e.apt
	s.osInfo = func() (string, string) { return "ubuntu", "noble" }
	s.fetch = func(_ context.Context, url string) ([]byte, error) {
		e.fetched = append(e.fetched, url)
		return e.key, nil
	}
	return e
}

func testKey(t *testing.T, armored bool) (data []byte, fingerprint string) {
	t.Helper()
	ent, err := openpgp.NewEntity("t", "", "t@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if armored {
		w, _ := armor.Encode(&buf, openpgp.PublicKeyType, nil)
		ent.Serialize(w)
		w.Close()
	} else {
		ent.Serialize(&buf)
	}
	return buf.Bytes(), fmt.Sprintf("%X", ent.PrimaryKey.Fingerprint)
}

func find2(items []Item, id string) Item {
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	return Item{}
}

func TestCatalogIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Catalog {
		if a.ID == "" || a.Name == "" || a.Description == "" || a.Icon == "" || a.Color == "" || a.Category == "" {
			t.Errorf("%q has empty fields", a.ID)
		}
		if seen[a.ID] {
			t.Errorf("duplicate id %q", a.ID)
		}
		seen[a.ID] = true
		if len(a.Packages) == 0 && a.VersionPkgs == nil {
			t.Errorf("%q has no packages", a.ID)
		}
		if !a.Multi && len(a.Check) == 0 {
			t.Errorf("%q has no detection package", a.ID)
		}
		if a.Versions != nil {
			if a.VersionPkgs == nil || a.Repo == nil || len(a.Repo.Fingerprints) == 0 {
				t.Errorf("%q has versions but no packages/pinned repository", a.ID)
			}
			for _, v := range a.Versions {
				if p := a.VersionPkgs(v, true); len(p) == 0 {
					t.Errorf("%q %s has no packages", a.ID, v)
				}
			}
		}
	}
}

func TestCleanVersion(t *testing.T) {
	for in, want := range map[string]string{
		"1:10.11.14-0ubuntu0.24.04.1": "10.11.14",
		"1.24.0-2ubuntu7.5":           "1.24.0",
		"16+257build1":                "16",
		"7.0.15":                      "7.0.15",
	} {
		if got := cleanVersion(in); got != want {
			t.Errorf("cleanVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListDetectsInstalledAndRunning(t *testing.T) {
	f := &fakeRunner{
		installed: map[string]string{"nginx": "1.24.0-2ubuntu7", "mariadb-server": "1:10.11.14-0ubuntu0.24.04.1"},
		active:    map[string]bool{"nginx": true, "php8.3-fpm": true},
	}
	items := newSvc(f, "8.3").List(context.Background())

	if n := find2(items, "nginx"); !n.Installed || !n.Running || n.Version != "1.24.0" {
		t.Errorf("nginx: %+v", n)
	}
	if m := find2(items, "mariadb"); !m.Installed || m.Running || m.Version != "10.11.14" {
		t.Errorf("mariadb installed but stopped: %+v", m)
	}
	if p := find2(items, "php"); !p.Installed || !p.Running || p.Version != "8.3" || !p.Multi {
		t.Errorf("php: %+v", p)
	}
	if g := find2(items, "git"); g.Installed || g.Running {
		t.Errorf("git should be absent: %+v", g)
	}
	if len(items) != len(Catalog) {
		t.Errorf("got %d items, want %d", len(items), len(Catalog))
	}
}

func TestInstallRunsAptAndStartsService(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{}, active: map[string]bool{}}
	s := newSvc(f)
	if _, err := s.Install("fail2ban", ""); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	if !f.saw("apt-get", "update", "") || !f.saw("apt-get", "install", "fail2ban") || !f.saw("systemctl", "enable", "fail2ban") {
		t.Errorf("unexpected calls: %+v", f.calls)
	}
	if got := find2(s.List(context.Background()), "fail2ban"); got.State != StateIdle {
		t.Errorf("state after success = %q", got.State)
	}
}

func TestInstallFailureIsReported(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{}, failApt: true}
	s := newSvc(f)
	if _, err := s.Install("git", ""); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	got := find2(s.List(context.Background()), "git")
	if got.State != StateFailed || !strings.Contains(got.Error, "held packages") {
		t.Errorf("want failed with apt output, got %+v", got)
	}
	f.failApt = false
	if _, err := s.Install("git", ""); err != nil {
		t.Errorf("retry after failure must be allowed: %v", err)
	}
	s.wg.Wait()
}

func TestInstallRejectsBadRequests(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{"git": "2.43.0"}}
	s := newSvc(f)
	if _, err := s.Install("nope; rm -rf /", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	if _, err := s.Install("git", ""); !errors.Is(err, ErrBadState) {
		t.Errorf("already installed: %v", err)
	}
	if _, err := s.Uninstall("certbot", ""); !errors.Is(err, ErrBadState) {
		t.Errorf("uninstall of absent app: %v", err)
	}
}

func TestRequiredAppsCannotBeRemoved(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{"nginx": "1", "postgresql": "16", "redis-server": "7"}}
	s := newSvc(f)
	for _, id := range []string{"nginx", "postgresql", "redis"} {
		if _, err := s.Uninstall(id, ""); !errors.Is(err, ErrBadState) {
			t.Errorf("%s: expected refusal, got %v", id, err)
		}
	}
	if f.saw("apt-get", "remove", "") {
		t.Error("apt-get remove must not run for required apps")
	}
}

func TestUninstallPHPRemovesOnlyTheChosenVersion(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{}}
	s := newSvc(f, "8.1", "8.3")
	if _, err := s.Uninstall("php", "8.3"); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	for _, p := range []string{"php8.3-fpm", "php8.3-cli"} {
		if !f.saw("apt-get", "remove", p) {
			t.Errorf("expected %s to be removed; calls: %+v", p, f.calls)
		}
	}
	if f.saw("apt-get", "remove", "php8.1-") {
		t.Error("only the chosen PHP version may be removed")
	}
	if f.saw("apt-get", "remove", "purge") {
		t.Error("data must be kept (no purge)")
	}
}

func TestConcurrentInstallOfSameAppIsRejected(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{}}
	s := newSvc(f)
	s.aptMu.Lock() // hold the apt lock so the first job stays "installing"
	if _, err := s.Install("git", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Install("git", ""); !errors.Is(err, ErrBadState) {
		t.Errorf("second install while busy: %v", err)
	}
	s.aptMu.Unlock()
	s.wg.Wait()
}

func TestRoutesRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allow := func(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
	NewHandler().RegisterRoutes(gin.New().Group("/api/v1"), allow)
}

func TestCandidateVersionShownOnlyWhenNotInstalled(t *testing.T) {
	f := &fakeRunner{
		installed: map[string]string{"git": "2.43.0"},
		policy:    map[string]string{"fail2ban2": "1", "git": "1:2.43.0-1", "memcached": "1.6.24-1ubuntu0.3"},
	}
	s := newSvc(f)
	items := s.List(context.Background())
	if c := find2(items, "fail2ban"); c.Candidate != "" {
		t.Errorf("missing from policy => empty candidate: %+v", c)
	}
	if g := find2(items, "git"); g.Candidate != "" {
		t.Errorf("installed apps must not report a candidate: %+v", g)
	}
	if m := find2(items, "memcached"); m.Candidate != "1.6.24" {
		t.Errorf("memcached candidate: %+v", m)
	}

	before := len(f.calls)
	s.List(context.Background())
	for _, c := range f.calls[before:] {
		if c.name == "apt-cache" {
			t.Fatalf("candidate lookups must be cached, saw %v", c)
		}
	}
}

func TestVersionsListSourceAndPackages(t *testing.T) {
	f := &fakeRunner{
		installed: map[string]string{},
		active:    map[string]bool{"php8.3-fpm": true},
		policy:    map[string]string{"php8.3-fpm": "8.3.6-0ubuntu0.24.04.5", "nodejs": "18.19.1+dfsg-6ubuntu5", "mariadb-server": "1:10.11.14-0ubuntu0.24.04.1"},
	}
	items := newSvc(f, "8.3").List(context.Background())
	byLabel := func(it Item, l string) VersionItem {
		for _, v := range it.Versions {
			if v.Label == l {
				return v
			}
		}
		t.Fatalf("%s has no version %s", it.ID, l)
		return VersionItem{}
	}

	php := find2(items, "php")
	if v := byLabel(php, "8.3"); v.Source != "distro" || !v.Installed || !v.Running {
		t.Errorf("php 8.3: %+v", v)
	}
	if v := byLabel(php, "8.4"); v.Source != "repo" || v.Repo == "" || v.Installed || v.Packages[0] != "php8.4-fpm" {
		t.Errorf("php 8.4: %+v", v)
	}
	node := find2(items, "nodejs")
	if v := byLabel(node, "22"); v.Source != "repo" || len(v.Packages) != 1 || v.Packages[0] != "nodejs" {
		t.Errorf("node 22 via NodeSource must not pull Ubuntu's npm: %+v", v)
	}
	maria := find2(items, "mariadb")
	if v := byLabel(maria, "10.11"); v.Source != "distro" {
		t.Errorf("mariadb 10.11 is what Ubuntu ships: %+v", v)
	}
	if v := byLabel(maria, "11.8"); v.Source != "repo" {
		t.Errorf("mariadb 11.8: %+v", v)
	}
}

func TestInstallVersionedValidation(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{}}
	s := newSvc(f, "8.3")
	cases := []struct {
		id, ver string
		want    error
	}{
		{"php", "", ErrBadInput},
		{"php", "5.0", ErrBadInput},
		{"php", "8.3", ErrBadState}, // already installed
		{"git", "1.0", ErrBadInput}, // git has no versions
		{"php", "8.3; rm -rf /", ErrBadInput},
	}
	for _, c := range cases {
		if _, err := s.Install(c.id, c.ver); !errors.Is(err, c.want) {
			t.Errorf("Install(%q,%q) = %v, want %v", c.id, c.ver, err, c.want)
		}
	}
	if len(f.calls) > 0 && f.saw("apt-get", "install", "") {
		t.Error("nothing may reach apt for rejected requests")
	}
}

func TestInstallPHPFromVendorRepo(t *testing.T) {
	key, fp := testKey(t, true)
	oldFps := phpRepo.Fingerprints
	phpRepo.Fingerprints = []string{fp}
	defer func() { phpRepo.Fingerprints = oldFps }()

	f := &fakeRunner{installed: map[string]string{}}
	s := newSvc(f, "8.3")
	e := withRepoSeams(t, s, key)

	if _, err := s.Install("php", "8.4"); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	if got := find2(s.List(context.Background()), "php"); got.State != StateIdle {
		t.Fatalf("install failed: %+v", got)
	}
	list, err := os.ReadFile(filepath.Join(e.apt, "sources.list.d", "rpanel-php.list"))
	if err != nil {
		t.Fatal(err)
	}
	want := "deb [signed-by=" + filepath.Join(e.apt, "keyrings", "rpanel-php.asc") + "] https://ppa.launchpadcontent.net/ondrej/php/ubuntu noble main"
	if !strings.Contains(string(list), want) {
		t.Errorf("sources file:\n%s\nwant line %s", list, want)
	}
	if _, err := os.Stat(filepath.Join(e.apt, "keyrings", "rpanel-php.asc")); err != nil {
		t.Errorf("armored key must be stored as .asc: %v", err)
	}
	if !f.saw("apt-get", "install", "php8.4-fpm") || !f.saw("systemctl", "enable", "php8.4-fpm") {
		t.Errorf("unexpected calls: %+v", f.calls)
	}
	if f.count("apt-get", "update") != 2 {
		t.Errorf("apt-get update must run before and after adding the repo, ran %d times", f.count("apt-get", "update"))
	}
}

func (f *fakeRunner) count(name, first string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.name == name && len(c.args) > 0 && c.args[0] == first {
			n++
		}
	}
	return n
}

func TestDistroVersionNeedsNoRepo(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{}, policy: map[string]string{"mariadb-server": "1:10.11.14-0ubuntu0.24.04.1"}}
	s := newSvc(f)
	e := withRepoSeams(t, s, nil)
	if _, err := s.Install("mariadb", "10.11"); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	if len(e.fetched) != 0 {
		t.Errorf("must not download keys for a distro version: %v", e.fetched)
	}
	if !f.saw("apt-get", "install", "mariadb-server") || !f.saw("systemctl", "enable", "mariadb") {
		t.Errorf("unexpected calls: %+v", f.calls)
	}
}

func TestWrongSigningKeyIsRejected(t *testing.T) {
	other, _ := testKey(t, true) // valid key, but not the pinned fingerprint
	f := &fakeRunner{installed: map[string]string{}}
	s := newSvc(f)
	e := withRepoSeams(t, s, other)

	if _, err := s.Install("nodejs", "22"); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	got := find2(s.List(context.Background()), "nodejs")
	if got.State != StateFailed || !strings.Contains(got.Error, "fingerprint") {
		t.Errorf("want a fingerprint failure, got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(e.apt, "sources.list.d", "rpanel-nodejs.list")); err == nil {
		t.Error("repository must not be added with an untrusted key")
	}
	if f.saw("apt-get", "install", "nodejs") {
		t.Error("nothing may be installed after a key mismatch")
	}
}

func TestCheckKeyArmoredAndBinary(t *testing.T) {
	for _, armored := range []bool{true, false} {
		data, fp := testKey(t, armored)
		got, err := checkKey(data, []string{"DEADBEEF", fp})
		if err != nil || got != armored {
			t.Errorf("armored=%v: got armored=%v err=%v", armored, got, err)
		}
		if _, err := checkKey(data, []string{"DEADBEEF"}); !errors.Is(err, errKeyMismatch) {
			t.Errorf("armored=%v: expected mismatch, got %v", armored, err)
		}
	}
	if _, err := checkKey([]byte("<html>not a key</html>"), []string{"X"}); err == nil {
		t.Error("garbage must be rejected")
	}
}

func TestRepoSourceLines(t *testing.T) {
	cases := []struct {
		r               *RepoDef
		os, codename, v string
		want            string
	}{
		{phpRepo, "debian", "bookworm", "8.4", "https://packages.sury.org/php/ bookworm main"},
		{nodeRepo, "ubuntu", "noble", "22", "https://deb.nodesource.com/node_22.x nodistro main"},
		{mariadbRepo, "debian", "bookworm", "11.8", "https://deb.mariadb.org/11.8/debian bookworm main"},
	}
	for _, c := range cases {
		if got, ok := c.r.Source(c.os, c.codename, c.v); !ok || got != c.want {
			t.Errorf("%s: got %q, want %q", c.r.Name, got, c.want)
		}
		if k := c.r.Key(c.os); !strings.HasPrefix(k, "https://") {
			t.Errorf("%s key URL must be https: %s", c.r.Name, k)
		}
	}
	if _, ok := phpRepo.Source("arch", "x", "8.4"); ok {
		t.Error("unsupported OS must be refused")
	}
}

func TestConfiguredVendorRepoCountsAsRepoAndSkipsSetup(t *testing.T) {
	f := &fakeRunner{
		installed: map[string]string{},
		policy:    map[string]string{"nodejs": "22.11.0-1nodesource1"},
		origins:   map[string]string{"nodejs": "     500 https://deb.nodesource.com/node_22.x nodistro/main arm64 Packages\n"},
	}
	s := newSvc(f)
	e := withRepoSeams(t, s, nil)

	node := find2(s.List(context.Background()), "nodejs")
	for _, v := range node.Versions {
		if v.Label == "22" && (v.Source != "repo" || len(v.Packages) != 1) {
			t.Errorf("22 comes from NodeSource and must not install Ubuntu's npm: %+v", v)
		}
	}
	if _, err := s.Install("nodejs", "22"); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	if len(e.fetched) != 0 {
		t.Errorf("repository is already configured, no key download expected: %v", e.fetched)
	}
	if f.saw("apt-get", "install", "npm") {
		t.Errorf("npm from Ubuntu conflicts with NodeSource nodejs: %+v", f.calls)
	}
}

func TestUninstallSingleVersionAppNeedsNoLabel(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{"nodejs": "22.23.4-1nodesource1"}}
	s := newSvc(f)
	if _, err := s.Uninstall("nodejs", ""); err != nil {
		t.Fatalf("node uninstall without a label: %v", err)
	}
	s.wg.Wait()
	if !f.saw("apt-get", "remove", "nodejs") {
		t.Errorf("unexpected calls: %+v", f.calls)
	}
	if _, err := s.Uninstall("php", ""); !errors.Is(err, ErrBadInput) {
		t.Errorf("PHP can have several versions, so the label stays mandatory: %v", err)
	}
}

func TestControlStartStopRestart(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{"fail2ban": "1.0.2-3", "mariadb-server": "1:10.11.14-0"}}
	s := newSvc(f, "8.3", "8.4")
	ctx := context.Background()

	for _, action := range []string{"start", "stop", "restart"} {
		if _, err := s.Control(ctx, "fail2ban", "", action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !f.saw("systemctl", action, "fail2ban") {
			t.Errorf("systemctl %s fail2ban not run: %+v", action, f.calls)
		}
	}
	if _, err := s.Control(ctx, "php", "8.4", "stop"); err != nil || !f.saw("systemctl", "stop", "php8.4-fpm") {
		t.Errorf("php 8.4 must be controlled by its own unit: %v %+v", err, f.calls)
	}
	if f.saw("systemctl", "stop", "php8.3-fpm") {
		t.Error("other PHP versions must stay untouched")
	}
}

func TestControlRefusals(t *testing.T) {
	f := &fakeRunner{installed: map[string]string{"nginx": "1.24.0", "postgresql": "16", "redis-server": "7", "git": "2.43"}}
	s := newSvc(f, "8.3")
	ctx := context.Background()
	cases := []struct {
		id, ver, action string
		want            error
	}{
		{"nginx", "", "stop", ErrBadState},         // the panel is served through it
		{"postgresql", "", "restart", ErrBadState}, // the panel's own database
		{"redis", "", "stop", ErrBadState},
		{"git", "", "start", ErrBadInput},       // no daemon
		{"memcached", "", "start", ErrBadState}, // not installed
		{"php", "", "start", ErrBadInput},       // version required
		{"php", "8.1", "start", ErrBadState},    // that version is not installed
		{"nope", "", "start", ErrNotFound},
		{"nginx", "", "reload; rm", ErrBadInput}, // only start/stop/restart
	}
	for _, c := range cases {
		if _, err := s.Control(ctx, c.id, c.ver, c.action); !errors.Is(err, c.want) {
			t.Errorf("Control(%s,%s,%s) = %v, want %v", c.id, c.ver, c.action, err, c.want)
		}
	}
	if _, err := s.Control(ctx, "nginx", "", "start"); err != nil {
		t.Errorf("starting a required service is harmless and must work: %v", err)
	}
	for _, c := range f.calls {
		if c.name == "systemctl" && c.args[0] != "start" && c.args[0] != "is-active" {
			t.Errorf("refused request reached systemctl: %+v", c)
		}
	}
}
