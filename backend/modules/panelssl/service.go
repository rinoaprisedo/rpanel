// Package panelssl manages the public hostname and TLS certificate of the panel itself.
//
// The panel's nginx site includes two small files owned by this package, so the
// settings survive upgrades (the installer never re-renders them):
//
//	panel-tls.conf   server_name + ssl_certificate(_key) of the panel server block
//	panel-guard.conf an optional catch-all server on the panel port that closes
//	                 connections addressed to anything but the panel domain
//
// A small port-80 vhost for the domain serves the ACME challenge and redirects to HTTPS.
package panelssl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"rpanel/backend/system"
)

var (
	ErrBadInput   = errors.New("invalid input")
	ErrNotSet     = errors.New("panel SSL is not configured on this installation")
	domainRe      = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	emailRe       = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)
	headerRe      = regexp.MustCompile(`(?m)^# rpanel-panel-ssl mode=(\S+) domain=(\S*)$`)
	certDirective = regexp.MustCompile(`(?m)^\s*ssl_certificate\s+(\S+);`)
)

// Modes of the panel certificate.
const (
	ModeLegacy      = "legacy" // installed before 1.3.3: reachable by IP (self-signed unless edited by hand)
	ModeLetsEncrypt = "letsencrypt"
	ModeCustom      = "custom"
)

type Settings struct {
	TLSConf   string // include file with server_name + certificate
	GuardConf string // include file with the catch-all server
	SSLDir    string // where the self-signed and custom certificates live
	Available string // nginx sites-available
	Enabled   string // nginx sites-enabled
	AcmeRoot  string
	CertDir   string // Let's Encrypt live dir, default /etc/letsencrypt/live
	Port      string // the panel's public port (for the HTTP -> HTTPS redirect)
}

type Service struct {
	run      system.Runner
	cfg      Settings
	lookupIP func(host string) ([]string, error)
}

func NewService(run system.Runner, cfg Settings) *Service {
	if cfg.CertDir == "" {
		cfg.CertDir = "/etc/letsencrypt/live"
	}
	return &Service{run: run, cfg: cfg, lookupIP: func(h string) ([]string, error) { return net.LookupHost(h) }}
}

// Enabled reports whether this installation has the Panel SSL include files (development setups do not).
func (s *Service) Enabled() bool { return s.cfg.TLSConf != "" }

type CertInfo struct {
	Issuer   string    `json:"issuer"`
	Names    []string  `json:"names"`
	NotAfter time.Time `json:"not_after"`
	DaysLeft int       `json:"days_left"`
}

type Status struct {
	Enabled     bool      `json:"enabled"`
	Mode        string    `json:"mode"`
	Domain      string    `json:"domain"`
	Port        string    `json:"port"`
	URL         string    `json:"url"`
	NeedsDomain bool      `json:"needs_domain"` // legacy install still reachable by IP with a self-signed certificate
	Cert        *CertInfo `json:"cert"`
}

// CurrentDomain returns the panel domain recorded in tlsConf ("" when unset).
func CurrentDomain(tlsConf string) string {
	if tlsConf == "" {
		return ""
	}
	b, err := os.ReadFile(tlsConf)
	if err != nil {
		return ""
	}
	if m := headerRe.FindSubmatch(b); m != nil {
		return string(m[2])
	}
	return ""
}

func (s *Service) Status() *Status {
	st := &Status{Enabled: s.Enabled(), Port: s.cfg.Port}
	if !st.Enabled {
		return st
	}
	b, err := os.ReadFile(s.cfg.TLSConf)
	if err != nil {
		st.Mode, st.NeedsDomain = ModeLegacy, true
		return st
	}
	st.Mode = ModeLegacy
	if m := headerRe.FindSubmatch(b); m != nil {
		st.Mode, st.Domain = string(m[1]), string(m[2])
	}
	var certPath string
	if m := certDirective.FindSubmatch(b); m != nil {
		certPath = string(m[1])
		st.Cert = readCert(certPath)
	}
	st.NeedsDomain = st.Mode == ModeLegacy && (certPath == "" || strings.HasSuffix(certPath, "/panel.crt"))
	if st.Domain != "" {
		st.URL = "https://" + st.Domain + portSuffix(s.cfg.Port)
	}
	return st
}

func portSuffix(p string) string {
	if p == "" || p == "443" {
		return ""
	}
	return ":" + p
}

func readCert(path string) *CertInfo {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	info := &CertInfo{Issuer: c.Issuer.CommonName, Names: c.DNSNames, NotAfter: c.NotAfter, DaysLeft: int(time.Until(c.NotAfter).Hours() / 24)}
	if info.Issuer == "" && len(c.Issuer.Organization) > 0 {
		info.Issuer = c.Issuer.Organization[0]
	}
	return info
}

// ValidDomain normalises and validates a host name (no IPs, no ports).
func ValidDomain(d string) (string, error) {
	d = strings.ToLower(strings.TrimSpace(d))
	if len(d) > 253 || !domainRe.MatchString(d) {
		return "", fmt.Errorf("%w: %q is not a valid domain name (use e.g. panel.example.com, not an IP address)", ErrBadInput, d)
	}
	return d, nil
}

func (s *Service) tlsText(mode, domain, cert, key string) string {
	return fmt.Sprintf(`# Managed by RPanel (Settings > Panel SSL). Do not edit: changes are overwritten.
# rpanel-panel-ssl mode=%s domain=%s
server_name %s;
ssl_certificate %s;
ssl_certificate_key %s;
add_header Strict-Transport-Security "max-age=31536000" always;
`, mode, domain, domain, cert, key)
}

func (s *Service) guardText() string {
	selfCert := filepath.Join(s.cfg.SSLDir, "panel.crt")
	selfKey := filepath.Join(s.cfg.SSLDir, "panel.key")
	return fmt.Sprintf(`# Managed by RPanel. Anything that is not the panel domain (e.g. the bare IP) is dropped.
server {
    listen %[1]s ssl default_server;
    listen [::]:%[1]s ssl default_server;
    server_name _;
    ssl_certificate %[2]s;
    ssl_certificate_key %[3]s;
    ssl_protocols TLSv1.2 TLSv1.3;
    return 444;
}
`, s.cfg.Port, selfCert, selfKey)
}

func (s *Service) acmeText(domain string) string {
	target := "https://$host" + portSuffix(s.cfg.Port) + "$request_uri"
	return fmt.Sprintf(`# Managed by RPanel: ACME validation and HTTP -> HTTPS redirect for the panel domain.
server {
    listen 80;
    listen [::]:80;
    server_name %s;
    server_tokens off;
    location /.well-known/acme-challenge/ {
        root %s;
    }
    location / {
        return 301 %s;
    }
}
`, domain, s.cfg.AcmeRoot, target)
}

func (s *Service) acmeConf() string { return filepath.Join(s.cfg.Available, "rpanel-panel-acme.conf") }
func (s *Service) acmeLink() string { return filepath.Join(s.cfg.Enabled, "rpanel-panel-acme.conf") }

type snapshot struct {
	path string
	data []byte
	had  bool
}

func take(path string) snapshot {
	b, err := os.ReadFile(path)
	return snapshot{path: path, data: b, had: err == nil}
}

func (sn snapshot) restore() {
	if sn.had {
		_ = writeAtomic(sn.path, sn.data, 0o644)
	} else {
		_ = os.Remove(sn.path)
	}
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rpanel-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (s *Service) nginxTestReload(ctx context.Context) error {
	if res, err := s.run.Run(ctx, 30*time.Second, "nginx", "-t"); err != nil {
		return fmt.Errorf("nginx config test failed: %s", strings.TrimSpace(res.Stderr+" "+err.Error()))
	}
	if res, err := s.run.Run(ctx, 30*time.Second, "systemctl", "reload", "nginx"); err != nil {
		return fmt.Errorf("nginx reload failed: %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

// writeAcme installs the port-80 vhost; the returned func undoes it.
func (s *Service) writeAcme(ctx context.Context, domain string) (undo func(), err error) {
	conf := take(s.acmeConf())
	_, linkErr := os.Lstat(s.acmeLink())
	hadLink := linkErr == nil
	undo = func() {
		conf.restore()
		if !hadLink {
			_ = os.Remove(s.acmeLink())
		}
		_ = s.nginxTestReload(ctx)
	}
	if err := os.MkdirAll(s.cfg.AcmeRoot, 0o755); err != nil {
		return nil, err
	}
	if err := writeAtomic(s.acmeConf(), []byte(s.acmeText(domain)), 0o644); err != nil {
		return nil, err
	}
	if !hadLink {
		if err := os.Symlink(s.acmeConf(), s.acmeLink()); err != nil {
			conf.restore()
			return nil, err
		}
	}
	if err := s.nginxTestReload(ctx); err != nil {
		undo()
		return nil, err
	}
	return undo, nil
}

// activate points the panel at domain/cert/key, enables the guard and reloads nginx; rolls back on failure.
func (s *Service) activate(ctx context.Context, mode, domain, cert, key string) error {
	tls, guard := take(s.cfg.TLSConf), take(s.cfg.GuardConf)
	rollback := func() {
		tls.restore()
		guard.restore()
		_ = s.nginxTestReload(ctx)
	}
	if err := writeAtomic(s.cfg.TLSConf, []byte(s.tlsText(mode, domain, cert, key)), 0o644); err != nil {
		return err
	}
	if err := writeAtomic(s.cfg.GuardConf, []byte(s.guardText()), 0o644); err != nil {
		tls.restore()
		return err
	}
	if err := s.nginxTestReload(ctx); err != nil {
		rollback()
		return err
	}
	return nil
}

// CheckDNS reports whether domain resolves. It returns a hint (not an error) when the records do not
// point at an address of this machine, since NAT and CDNs make that check advisory only.
func CheckDNS(lookup func(string) ([]string, error), domain string) (ips []string, hint string, err error) {
	ips, lerr := lookup(domain)
	if lerr != nil || len(ips) == 0 {
		return nil, "", fmt.Errorf("%w: %s does not resolve yet; create an A record pointing at this server's public IP and try again", ErrBadInput, domain)
	}
	local := map[string]bool{}
	if addrs, e := net.InterfaceAddrs(); e == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				local[ipn.IP.String()] = true
			}
		}
	}
	for _, ip := range ips {
		if local[ip] {
			return ips, "", nil
		}
	}
	return ips, fmt.Sprintf("%s resolves to %s, which is not an address of this server; check the DNS record", domain, strings.Join(ips, ", ")), nil
}

// CertbotRegistration returns the certbot arguments that identify the ACME account: the e-mail when given
// (validated), otherwise an anonymous registration. Let's Encrypt does not require an e-mail address.
func CertbotRegistration(email string) ([]string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return []string{"--register-unsafely-without-email"}, nil
	}
	if !emailRe.MatchString(email) {
		return nil, fmt.Errorf("%w: %q is not a valid e-mail address (leave it empty to skip)", ErrBadInput, email)
	}
	return []string{"-m", email}, nil
}

func (s *Service) resolves(domain string) (hint string, err error) {
	_, hint, err = CheckDNS(s.lookupIP, domain)
	return hint, err
}

// IssueLetsEncrypt validates the domain over port 80, obtains a certificate with certbot and switches the panel to it.
func (s *Service) IssueLetsEncrypt(ctx context.Context, domain, email string) error {
	if !s.Enabled() {
		return ErrNotSet
	}
	d, err := ValidDomain(domain)
	if err != nil {
		return err
	}
	reg, err := CertbotRegistration(email)
	if err != nil {
		return err
	}
	hint, err := s.resolves(d)
	if err != nil {
		return err
	}
	undo, err := s.writeAcme(ctx, d)
	if err != nil {
		return err
	}
	args := append([]string{"certonly", "--webroot", "-w", s.cfg.AcmeRoot, "-d", d, "--non-interactive", "--agree-tos", "--keep-until-expiring"}, reg...)
	res, err := s.run.Run(ctx, 5*time.Minute, "certbot", args...)
	if err != nil {
		undo()
		msg := "certbot failed: " + lastLines(res.Stdout+"\n"+res.Stderr, 6)
		if hint != "" {
			msg += "\n" + hint
		}
		return errors.New(msg)
	}
	live := filepath.Join(s.cfg.CertDir, d)
	if err := s.activate(ctx, ModeLetsEncrypt, d, filepath.Join(live, "fullchain.pem"), filepath.Join(live, "privkey.pem")); err != nil {
		undo()
		return err
	}
	return nil
}

// SaveCustom installs a user-supplied certificate (wildcard, Cloudflare origin, commercial...) for domain.
func (s *Service) SaveCustom(ctx context.Context, domain, certPEM, keyPEM string) error {
	if !s.Enabled() {
		return ErrNotSet
	}
	d, err := ValidDomain(domain)
	if err != nil {
		return err
	}
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("%w: certificate and private key do not match or are not PEM: %v", ErrBadInput, err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadInput, err)
	}
	if err := leaf.VerifyHostname(d); err != nil {
		return fmt.Errorf("%w: the certificate does not cover %s", ErrBadInput, d)
	}
	if time.Now().After(leaf.NotAfter) {
		return fmt.Errorf("%w: the certificate has expired", ErrBadInput)
	}
	crt, key := filepath.Join(s.cfg.SSLDir, "panel-custom.crt"), filepath.Join(s.cfg.SSLDir, "panel-custom.key")
	oldCrt, oldKey := take(crt), take(key)
	if err := writeAtomic(crt, []byte(certPEM), 0o644); err != nil {
		return err
	}
	if err := writeAtomic(key, []byte(keyPEM), 0o600); err != nil {
		oldCrt.restore()
		return err
	}
	undo, err := s.writeAcme(ctx, d)
	if err != nil {
		oldCrt.restore()
		oldKey.restore()
		return err
	}
	if err := s.activate(ctx, ModeCustom, d, crt, key); err != nil {
		undo()
		oldCrt.restore()
		oldKey.restore()
		return err
	}
	return nil
}
