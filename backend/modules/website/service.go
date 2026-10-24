package website

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"rpanel/backend/system"

	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("website not found")
	ErrExists   = errors.New("a website with this domain already exists")
	emailRe     = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)
)

// Settings are the host-specific paths and defaults of the manager.
type Settings struct {
	CertDir      string // Let's Encrypt "live" directory; default /etc/letsencrypt/live
	WebRoot      string
	AcmeRoot     string
	LogDir       string
	CertbotEmail string
}

type Service struct {
	db  *gorm.DB
	ng  *Nginx
	run system.Runner
	cfg Settings
}

func NewService(db *gorm.DB, ng *Nginx, run system.Runner, cfg Settings) *Service {
	return &Service{db: db, ng: ng, run: run, cfg: cfg}
}

type CreateInput struct {
	Domain      string  `json:"domain"`
	Type        string  `json:"type"`
	RootDir     string  `json:"root_dir"`
	PHPVersion  string  `json:"php_version"`
	ProxyTarget string  `json:"proxy_target"`
	ActorID     *uint64 `json:"-"`
}

type UpdateInput struct {
	RootDir     *string `json:"root_dir"`
	PHPVersion  *string `json:"php_version"`
	ProxyTarget *string `json:"proxy_target"`
	ForceHTTPS  *bool   `json:"force_https"`
}

func (s *Service) List() ([]Website, error) {
	var out []Website
	return out, s.db.Order("domain").Find(&out).Error
}

func (s *Service) Get(uuid string) (*Website, error) {
	var w Website
	if err := s.db.Where("uuid = ?", uuid).First(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &w, nil
}

// webRootPath validates that p lies inside the configured web root.
func (s *Service) webRootPath(p string) (string, error) {
	r, err := system.ResolvePath(p, []string{s.cfg.WebRoot}, false)
	if err != nil {
		return "", fmt.Errorf("%w: root directory must be inside %s", ErrBadInput, s.cfg.WebRoot)
	}
	return r, nil
}

const placeholderHTML = "<!doctype html><meta charset=utf-8><title>%s</title><h1>%s</h1><p>Site created with RPanel.</p>\n"

func (s *Service) Create(ctx context.Context, in CreateInput) (*Website, error) {
	w := &Website{
		Domain: strings.ToLower(strings.TrimSpace(in.Domain)), Type: in.Type,
		PHPVersion: in.PHPVersion, ProxyTarget: in.ProxyTarget, Status: StatusRunning,
	}
	w.CreatedBy, w.UpdatedBy = in.ActorID, in.ActorID
	if w.Type == TypeStatic || w.Type == TypePHP {
		root := in.RootDir
		if root == "" && system.ValidDomain(w.Domain) {
			root = filepath.Join(s.cfg.WebRoot, w.Domain)
		}
		var err error
		if w.RootDir, err = s.webRootPath(root); err != nil {
			return nil, err
		}
	} else {
		w.PHPVersion = ""
	}
	if w.Type != TypePHP {
		w.PHPVersion = ""
	}
	if w.Type != TypeProxy {
		w.ProxyTarget = ""
	}
	if err := Validate(w); err != nil {
		return nil, err
	}
	var n int64
	s.db.Model(&Website{}).Where("domain = ?", w.Domain).Count(&n)
	if n > 0 {
		return nil, ErrExists
	}

	createdDir := false
	if w.RootDir != "" {
		if _, err := os.Stat(w.RootDir); os.IsNotExist(err) {
			if err := os.MkdirAll(w.RootDir, 0o755); err != nil {
				return nil, fmt.Errorf("cannot create root directory: %w", err)
			}
			createdDir = true
			name := "index.html"
			body := fmt.Sprintf(placeholderHTML, w.Domain, w.Domain)
			if w.Type == TypePHP {
				name, body = "index.php", "<?php echo 'Hello from '.htmlspecialchars($_SERVER['HTTP_HOST']); ?>\n"
			}
			_ = os.WriteFile(filepath.Join(w.RootDir, name), []byte(body), 0o644)
		}
	}
	if err := s.ng.Apply(ctx, w, true); err != nil {
		if createdDir {
			_ = os.RemoveAll(w.RootDir)
		}
		return nil, err
	}
	if err := s.db.Create(w).Error; err != nil {
		_ = s.ng.Remove(ctx, w.Domain)
		return nil, err
	}
	return w, nil
}

func (s *Service) Update(ctx context.Context, uuid string, in UpdateInput, actor *uint64) (*Website, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	old := *w
	if in.RootDir != nil && w.Type != TypeProxy {
		if w.RootDir, err = s.webRootPath(*in.RootDir); err != nil {
			return nil, err
		}
	}
	if in.PHPVersion != nil && w.Type == TypePHP {
		w.PHPVersion = *in.PHPVersion
	}
	if in.ProxyTarget != nil && w.Type == TypeProxy {
		w.ProxyTarget = *in.ProxyTarget
	}
	if in.ForceHTTPS != nil {
		if *in.ForceHTTPS && !w.SSLEnabled {
			return nil, fmt.Errorf("%w: enable SSL before forcing HTTPS", ErrBadInput)
		}
		w.ForceHTTPS = *in.ForceHTTPS
	}
	if err := s.ng.Apply(ctx, w, w.Status == StatusRunning); err != nil {
		return nil, err
	}
	w.UpdatedBy = actor
	if err := s.db.Save(w).Error; err != nil {
		_ = s.ng.Apply(ctx, &old, old.Status == StatusRunning)
		return nil, err
	}
	return w, nil
}

// SetStatus starts or stops (disables the vhost of) a site.
func (s *Service) SetStatus(ctx context.Context, uuid, status string, actor *uint64) (*Website, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	if err := s.ng.Apply(ctx, w, status == StatusRunning); err != nil {
		return nil, err
	}
	w.Status, w.UpdatedBy = status, actor
	return w, s.db.Save(w).Error
}

func (s *Service) Delete(ctx context.Context, uuid string, deleteFiles bool) (*Website, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	if err := s.ng.Remove(ctx, w.Domain); err != nil {
		return nil, err
	}
	if deleteFiles && w.RootDir != "" {
		// Only ever inside the web root, and never the web root itself.
		if r, err := s.webRootPath(w.RootDir); err == nil && r != filepath.Clean(s.cfg.WebRoot) {
			_ = os.RemoveAll(r)
		}
	}
	return w, s.db.Unscoped().Delete(w).Error
}

// IssueSSL obtains a Let's Encrypt certificate (webroot challenge) and
// switches the vhost to HTTPS.
func (s *Service) IssueSSL(ctx context.Context, uuid, email string, actor *uint64) (*Website, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	if w.Status != StatusRunning {
		return nil, fmt.Errorf("%w: start the site first", ErrBadInput)
	}
	if email == "" {
		email = s.cfg.CertbotEmail
	}
	if !emailRe.MatchString(email) {
		return nil, fmt.Errorf("%w: a valid email is required for Let's Encrypt", ErrBadInput)
	}
	if err := os.MkdirAll(s.cfg.AcmeRoot, 0o755); err != nil {
		return nil, err
	}
	res, err := s.run.Run(ctx, 5*time.Minute, "certbot", "certonly", "--webroot", "-w", s.cfg.AcmeRoot,
		"-d", w.Domain, "--non-interactive", "--agree-tos", "-m", email, "--keep-until-expiring")
	if err != nil {
		return nil, fmt.Errorf("certbot failed: %s", lastLines(res.Stdout+"\n"+res.Stderr, 6))
	}
	prev := *w
	w.SSLEnabled, w.UpdatedBy = true, actor
	if err := s.ng.Apply(ctx, w, true); err != nil {
		return nil, err
	}
	if err := s.db.Save(w).Error; err != nil {
		_ = s.ng.Apply(ctx, &prev, true)
		return nil, err
	}
	return w, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Logs returns the last n lines of the access or error log of a site.
func (s *Service) Logs(uuid, kind string, n int) (string, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return "", err
	}
	if kind != "access" && kind != "error" {
		return "", fmt.Errorf("%w: log type", ErrBadInput)
	}
	if n < 1 || n > 1000 {
		n = 200
	}
	f, err := os.Open(filepath.Join(s.cfg.LogDir, w.Domain+"."+kind+".log"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	const window = 512 << 10
	if st, err := f.Stat(); err == nil && st.Size() > window {
		f.Seek(st.Size()-window, io.SeekStart)
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n"), nil
}

// Env describes what is installed on the host.
type Env struct {
	Nginx      bool     `json:"nginx"`
	NginxVer   string   `json:"nginx_version"`
	Certbot    bool     `json:"certbot"`
	PHPVersion []string `json:"php_versions"`
}

func (s *Service) Env(ctx context.Context) Env {
	var e Env
	if _, err := exec.LookPath("nginx"); err == nil {
		e.Nginx = true
		if res, _ := s.run.Run(ctx, 5*time.Second, "nginx", "-v"); res.Stderr != "" {
			e.NginxVer = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(res.Stderr), "nginx version:"))
		}
	}
	_, err := exec.LookPath("certbot")
	e.Certbot = err == nil
	dirs, _ := filepath.Glob("/etc/php/*/fpm")
	for _, d := range dirs {
		v := filepath.Base(filepath.Dir(d))
		// Uninstalling PHP keeps /etc/php/<v>; only the FPM binary proves it is still there.
		if _, err := os.Stat("/usr/sbin/php-fpm" + v); err == nil {
			e.PHPVersion = append(e.PHPVersion, v)
		}
	}
	if e.PHPVersion == nil {
		e.PHPVersion = []string{}
	}
	return e
}

// Install installs a supported component through apt (Debian/Ubuntu).
func (s *Service) Install(ctx context.Context, component, version string) error {
	var pkgs []string
	switch component {
	case "nginx":
		pkgs = []string{"nginx"}
	case "certbot":
		pkgs = []string{"certbot"}
	case "php":
		if !phpVerRe.MatchString(version) {
			return fmt.Errorf("%w: php version", ErrBadInput)
		}
		pkgs = []string{"php" + version + "-fpm", "php" + version + "-cli"}
	default:
		return fmt.Errorf("%w: component", ErrBadInput)
	}
	if res, err := s.run.Run(ctx, 5*time.Minute, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt-get update failed: %s", lastLines(res.Stderr, 4))
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if res, err := s.run.Run(ctx, 15*time.Minute, "apt-get", args...); err != nil {
		return fmt.Errorf("install failed: %s", lastLines(res.Stderr, 6))
	}
	return nil
}

// DirEntry is one sub-directory offered by the folder picker.
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// DirListing is the folder picker's view of one directory inside the web root.
type DirListing struct {
	Root   string     `json:"root"`
	Path   string     `json:"path"`
	Parent string     `json:"parent"` // empty at the web root
	Dirs   []DirEntry `json:"dirs"`
}

// ListDirs lists the sub-directories of p (default: the web root). It never
// leaves the web root and never follows symlinks, so the picker cannot be used
// to browse the rest of the server.
func (s *Service) ListDirs(p string) (*DirListing, error) {
	if p == "" {
		p = s.cfg.WebRoot
	}
	dir, err := s.webRootPath(p)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: folder does not exist", ErrBadInput)
		}
		return nil, err
	}
	out := &DirListing{Root: filepath.Clean(s.cfg.WebRoot), Path: dir, Dirs: []DirEntry{}}
	if dir != out.Root {
		out.Parent = filepath.Dir(dir)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out.Dirs = append(out.Dirs, DirEntry{Name: e.Name(), Path: filepath.Join(dir, e.Name())})
	}
	return out, nil
}

// mutate applies change to a site, regenerates its vhost (validated with nginx -t and
// rolled back on failure) and stores the result.
func (s *Service) mutate(ctx context.Context, uuid string, actor *uint64, change func(w *Website) error) (*Website, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	old := *w
	if err := change(w); err != nil {
		return nil, err
	}
	if err := s.ng.Apply(ctx, w, w.Status == StatusRunning); err != nil {
		return nil, err
	}
	w.UpdatedBy = actor
	if err := s.db.Save(w).Error; err != nil {
		_ = s.ng.Apply(ctx, &old, old.Status == StatusRunning)
		return nil, err
	}
	return w, nil
}

// SetProxyRules replaces the reverse-proxy rules of a site.
func (s *Service) SetProxyRules(ctx context.Context, uuid string, rules []ProxyRule, actor *uint64) (*Website, error) {
	clean := make([]ProxyRule, 0, len(rules))
	for _, r := range rules {
		r.Name, r.Path, r.Target = strings.TrimSpace(r.Name), strings.TrimSpace(r.Path), strings.TrimSpace(r.Target)
		clean = append(clean, r)
	}
	return s.mutate(ctx, uuid, actor, func(w *Website) error {
		w.ProxyRules = clean
		return validateRules(clean)
	})
}

// SetExtraConfig replaces the custom nginx directives of a site.
func (s *Service) SetExtraConfig(ctx context.Context, uuid, text string, actor *uint64) (*Website, error) {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), " \t\n")
	return s.mutate(ctx, uuid, actor, func(w *Website) error {
		w.ExtraConfig = text
		return validateExtra(text)
	})
}

// DisableSSL switches the vhost back to plain HTTP. The certificate stays on disk.
func (s *Service) DisableSSL(ctx context.Context, uuid string, actor *uint64) (*Website, error) {
	return s.mutate(ctx, uuid, actor, func(w *Website) error {
		w.SSLEnabled, w.ForceHTTPS = false, false
		return nil
	})
}

// SiteConfig is what the config editor shows: the editable directives and the
// complete vhost nginx actually runs.
type SiteConfig struct {
	ExtraConfig string `json:"extra_config"`
	Rendered    string `json:"rendered"`
}

func (s *Service) Config(uuid string) (*SiteConfig, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	text, err := s.ng.Render(w)
	if err != nil {
		return nil, err
	}
	return &SiteConfig{ExtraConfig: w.ExtraConfig, Rendered: text}, nil
}

// CertInfo describes the certificate currently on disk for a domain.
type CertInfo struct {
	Issuer   string    `json:"issuer"`
	Names    []string  `json:"names"`
	NotAfter time.Time `json:"not_after"`
	DaysLeft int       `json:"days_left"`
}

type SSLStatus struct {
	Enabled    bool      `json:"enabled"`
	ForceHTTPS bool      `json:"force_https"`
	Cert       *CertInfo `json:"cert"`
}

func (s *Service) SSLStatus(uuid string) (*SSLStatus, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	st := &SSLStatus{Enabled: w.SSLEnabled, ForceHTTPS: w.ForceHTTPS}
	st.Cert = ReadCert(s.cfg.CertDir, w.Domain)
	return st, nil
}

// ReadCert describes the Let's Encrypt certificate on disk for domain, or nil when there is none.
// certDir defaults to /etc/letsencrypt/live.
func ReadCert(certDir, domain string) *CertInfo {
	if certDir == "" {
		certDir = "/etc/letsencrypt/live"
	}
	pemBytes, err := os.ReadFile(filepath.Join(certDir, domain, "fullchain.pem"))
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	info := &CertInfo{
		Issuer: cert.Issuer.CommonName, Names: cert.DNSNames, NotAfter: cert.NotAfter,
		DaysLeft: int(time.Until(cert.NotAfter).Hours() / 24),
	}
	if info.Issuer == "" && len(cert.Issuer.Organization) > 0 {
		info.Issuer = cert.Issuer.Organization[0]
	}
	return info
}
