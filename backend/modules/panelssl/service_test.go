package panelssl

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
	"testing"
	"time"

	"rpanel/backend/system"
)

type call struct {
	name string
	args []string
}

type fakeRunner struct {
	calls    []call
	failTest bool
	failCert bool
}

func (f *fakeRunner) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.calls = append(f.calls, call{name, args})
	if name == "nginx" && f.failTest {
		return system.Result{Stderr: "bad config", ExitCode: 1}, errors.New("exit 1")
	}
	if name == "certbot" && f.failCert {
		return system.Result{Stderr: "challenge failed", ExitCode: 1}, errors.New("exit 1")
	}
	return system.Result{}, nil
}

func (f *fakeRunner) n(name string) int {
	c := 0
	for _, x := range f.calls {
		if x.name == name {
			c++
		}
	}
	return c
}

func setup(t *testing.T) (*Service, *fakeRunner, Settings) {
	t.Helper()
	dir := t.TempDir()
	cfg := Settings{
		TLSConf: filepath.Join(dir, "ssl", "panel-tls.conf"), GuardConf: filepath.Join(dir, "ssl", "panel-guard.conf"),
		SSLDir: filepath.Join(dir, "ssl"), Available: filepath.Join(dir, "av"), Enabled: filepath.Join(dir, "en"),
		AcmeRoot: filepath.Join(dir, "acme"), CertDir: filepath.Join(dir, "live"), Port: "25173",
	}
	for _, d := range []string{cfg.SSLDir, cfg.Available, cfg.Enabled} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(cfg.TLSConf, []byte("# rpanel-panel-ssl mode=legacy domain=\nserver_name _;\nssl_certificate "+cfg.SSLDir+"/panel.crt;\nssl_certificate_key "+cfg.SSLDir+"/panel.key;\n"), 0o644)
	os.WriteFile(cfg.GuardConf, []byte("# guard disabled\n"), 0o644)
	run := &fakeRunner{}
	s := NewService(run, cfg)
	s.lookupIP = func(string) ([]string, error) { return []string{"203.0.113.9"}, nil }
	return s, run, cfg
}

func TestValidDomain(t *testing.T) {
	for _, ok := range []string{"panel.example.com", "A.B.example.id"} {
		if _, err := ValidDomain(ok); err != nil {
			t.Errorf("%s should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "localhost", "213.163.199.221", "a b.com", "x.com;rm", "-a.com", "a.com:25173", "https://a.com"} {
		if _, err := ValidDomain(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestStatusLegacyNeedsDomain(t *testing.T) {
	s, _, _ := setup(t)
	st := s.Status()
	if st.Mode != ModeLegacy || !st.NeedsDomain || st.Domain != "" {
		t.Fatalf("unexpected status %+v", st)
	}
}

func TestIssueLetsEncryptSwitchesPanel(t *testing.T) {
	s, run, cfg := setup(t)
	if err := s.IssueLetsEncrypt(context.Background(), "Panel.Example.com", "me@example.com"); err != nil {
		t.Fatal(err)
	}
	tls, _ := os.ReadFile(cfg.TLSConf)
	for _, want := range []string{"mode=letsencrypt domain=panel.example.com", "server_name panel.example.com;", filepath.Join(cfg.CertDir, "panel.example.com", "fullchain.pem"), "Strict-Transport-Security"} {
		if !strings.Contains(string(tls), want) {
			t.Errorf("tls conf missing %q:\n%s", want, tls)
		}
	}
	guard, _ := os.ReadFile(cfg.GuardConf)
	if !strings.Contains(string(guard), "default_server") || !strings.Contains(string(guard), "return 444") {
		t.Errorf("guard not enabled:\n%s", guard)
	}
	acme, _ := os.ReadFile(filepath.Join(cfg.Available, "rpanel-panel-acme.conf"))
	if !strings.Contains(string(acme), "return 301 https://$host:25173$request_uri") || !strings.Contains(string(acme), "acme-challenge") {
		t.Errorf("acme vhost wrong:\n%s", acme)
	}
	if _, err := os.Lstat(filepath.Join(cfg.Enabled, "rpanel-panel-acme.conf")); err != nil {
		t.Error("acme vhost not enabled")
	}
	if run.n("certbot") != 1 {
		t.Errorf("certbot calls = %d", run.n("certbot"))
	}
	if st := s.Status(); st.Mode != ModeLetsEncrypt || st.Domain != "panel.example.com" || st.URL != "https://panel.example.com:25173" || st.NeedsDomain {
		t.Errorf("status %+v", st)
	}
	if CurrentDomain(cfg.TLSConf) != "panel.example.com" {
		t.Error("CurrentDomain")
	}
}

func TestIssueRollsBackWhenCertbotFails(t *testing.T) {
	s, run, cfg := setup(t)
	run.failCert = true
	before, _ := os.ReadFile(cfg.TLSConf)
	err := s.IssueLetsEncrypt(context.Background(), "panel.example.com", "me@example.com")
	if err == nil || !strings.Contains(err.Error(), "challenge failed") {
		t.Fatalf("expected certbot error, got %v", err)
	}
	after, _ := os.ReadFile(cfg.TLSConf)
	if string(before) != string(after) {
		t.Error("tls conf changed after failure")
	}
	if _, err := os.Stat(filepath.Join(cfg.Available, "rpanel-panel-acme.conf")); err == nil {
		t.Error("acme vhost should be removed after failure")
	}
	if g, _ := os.ReadFile(cfg.GuardConf); strings.Contains(string(g), "default_server") {
		t.Error("guard must stay disabled")
	}
}

func TestIssueRollsBackWhenNginxRejects(t *testing.T) {
	s, run, cfg := setup(t)
	run.failTest = true
	before, _ := os.ReadFile(cfg.TLSConf)
	if err := s.IssueLetsEncrypt(context.Background(), "panel.example.com", "me@example.com"); err == nil {
		t.Fatal("expected an error")
	}
	if after, _ := os.ReadFile(cfg.TLSConf); string(before) != string(after) {
		t.Error("tls conf not restored")
	}
}

func TestIssueRejectsBadInput(t *testing.T) {
	s, _, _ := setup(t)
	s.lookupIP = func(string) ([]string, error) { return nil, errors.New("nxdomain") }
	for _, tc := range [][2]string{{"1.2.3.4", "me@example.com"}, {"panel.example.com", "not-an-email"}, {"panel.example.com", "me@example.com"}} {
		if err := s.IssueLetsEncrypt(context.Background(), tc[0], tc[1]); !errors.Is(err, ErrBadInput) {
			t.Errorf("%v: expected ErrBadInput, got %v", tc, err)
		}
	}
}

func selfSigned(t *testing.T, host string) (certPEM, keyPEM string) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	kb, _ := x509.MarshalECPrivateKey(k)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}))
}

func TestSaveCustom(t *testing.T) {
	s, _, cfg := setup(t)
	crt, key := selfSigned(t, "panel.example.com")
	if err := s.SaveCustom(context.Background(), "panel.example.com", crt, key); err != nil {
		t.Fatal(err)
	}
	st := s.Status()
	if st.Mode != ModeCustom || st.Cert == nil {
		t.Fatalf("status %+v", st)
	}
	if fi, _ := os.Stat(filepath.Join(cfg.SSLDir, "panel-custom.key")); fi.Mode().Perm() != 0o600 {
		t.Errorf("key mode %v", fi.Mode().Perm())
	}
	// wrong host, mismatched key
	if err := s.SaveCustom(context.Background(), "other.example.com", crt, key); !errors.Is(err, ErrBadInput) {
		t.Errorf("host mismatch should fail, got %v", err)
	}
	_, key2 := selfSigned(t, "panel.example.com")
	if err := s.SaveCustom(context.Background(), "panel.example.com", crt, key2); !errors.Is(err, ErrBadInput) {
		t.Errorf("key mismatch should fail, got %v", err)
	}
}

func TestDisabledWithoutConfig(t *testing.T) {
	s := NewService(&fakeRunner{}, Settings{})
	if err := s.IssueLetsEncrypt(context.Background(), "panel.example.com", "me@example.com"); !errors.Is(err, ErrNotSet) {
		t.Errorf("got %v", err)
	}
	if s.Status().Enabled {
		t.Error("should be disabled")
	}
}
