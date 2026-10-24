package website

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"rpanel/backend/system"
)

var (
	// Only plain absolute paths may reach the nginx config: no spaces, quotes,
	// semicolons, braces, or `$` (nginx variable expansion).
	safePathRe  = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)
	phpVerRe    = regexp.MustCompile(`^[0-9]\.[0-9]$`)
	proxyRe     = regexp.MustCompile(`^https?://[A-Za-z0-9.-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~/-]*)?$`)
	ErrBadInput = errors.New("invalid input")

	rulePathRe = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)
)

const (
	maxProxyRules  = 30
	maxExtraConfig = 20000
)

// validateRules checks every field that is interpolated into a location block.
func validateRules(rules []ProxyRule) error {
	if len(rules) > maxProxyRules {
		return fmt.Errorf("%w: at most %d proxy rules", ErrBadInput, maxProxyRules)
	}
	seen := map[string]bool{}
	for _, r := range rules {
		switch {
		case !rulePathRe.MatchString(r.Path) || strings.Contains(r.Path, "..") || len(r.Path) > 200:
			return fmt.Errorf("%w: proxy path %q (use letters, digits and . _ ~ - /, starting with /)", ErrBadInput, r.Path)
		case r.Path == "/":
			return fmt.Errorf("%w: use a sub-path such as /api/ (the site's own / is managed by its type)", ErrBadInput)
		case strings.HasPrefix(r.Path, "/.well-known"):
			return fmt.Errorf("%w: /.well-known is reserved for certificate validation", ErrBadInput)
		case seen[r.Path]:
			return fmt.Errorf("%w: duplicate proxy path %s", ErrBadInput, r.Path)
		case !proxyRe.MatchString(r.Target) || strings.Contains(r.Target, ".."):
			return fmt.Errorf("%w: proxy target %q (expected http://host:port or https://host)", ErrBadInput, r.Target)
		case len(r.Name) > 60 || strings.ContainsAny(r.Name, "\r\n\x00"):
			return fmt.Errorf("%w: rule name", ErrBadInput)
		}
		seen[r.Path] = true
	}
	return nil
}

// validateExtra rejects text that could close the server block it is pasted into.
// nginx -t still has the last word; this keeps one stray "}" from smuggling in a
// whole second server block that happens to be balanced.
func validateExtra(text string) error {
	if len(text) > maxExtraConfig || strings.ContainsRune(text, 0) {
		return fmt.Errorf("%w: custom config is too large or contains NUL", ErrBadInput)
	}
	depth := 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '#':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			if depth--; depth < 0 {
				return fmt.Errorf("%w: unbalanced } in custom config", ErrBadInput)
			}
		}
	}
	if depth != 0 || quote != 0 {
		return fmt.Errorf("%w: unbalanced braces or quotes in custom config", ErrBadInput)
	}
	return nil
}

// Validate checks every site field that is interpolated into the nginx config.
func Validate(w *Website) error {
	if !system.ValidDomain(w.Domain) {
		return fmt.Errorf("%w: domain", ErrBadInput)
	}
	switch w.Type {
	case TypeStatic, TypePHP:
		if !safePathRe.MatchString(w.RootDir) || strings.Contains(w.RootDir, "..") {
			return fmt.Errorf("%w: root directory", ErrBadInput)
		}
		if w.Type == TypePHP && !phpVerRe.MatchString(w.PHPVersion) {
			return fmt.Errorf("%w: php version", ErrBadInput)
		}
	case TypeProxy:
		if !proxyRe.MatchString(w.ProxyTarget) || strings.Contains(w.ProxyTarget, "..") {
			return fmt.Errorf("%w: proxy target", ErrBadInput)
		}
	default:
		return fmt.Errorf("%w: type", ErrBadInput)
	}
	if err := validateRules(w.ProxyRules); err != nil {
		return err
	}
	return validateExtra(w.ExtraConfig)
}

var vhostTmpl = template.Must(template.New("vhost").Parse(`# Managed by RPanel — manual edits will be overwritten.
{{define "body" -}}
    {{if eq .Type "proxy" -}}
    location / {
        proxy_pass {{.ProxyTarget}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $http_connection;
    }
    {{- else if eq .Type "php" -}}
    root {{.RootDir}};
    index index.php index.html index.htm;
    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }
    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/php{{.PHPVersion}}-fpm.sock;
    }
    location ~ /\.(?!well-known) {
        deny all;
    }
    {{- else -}}
    root {{.RootDir}};
    index index.html index.htm;
    location / {
        try_files $uri $uri/ =404;
    }
    location ~ /\.(?!well-known) {
        deny all;
    }
    {{- end}}
{{- end}}
{{define "extras" -}}
{{range .ProxyRules}}{{if .Enabled}}
    location {{.Path}} {
        proxy_pass {{.Target}};
        proxy_http_version 1.1;
        proxy_set_header Host {{if .PassHost}}$host{{else}}$proxy_host{{end}};
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $http_connection;{{if not .PassHost}}
        proxy_ssl_server_name on;{{end}}
    }
{{end}}{{end}}{{if .ExtraConfig}}
    # custom directives
{{.ExtraConfig}}
{{end}}
{{- end}}
{{define "common" -}}
    server_name {{.Domain}};
    access_log {{.LogDir}}/{{.Domain}}.access.log;
    error_log {{.LogDir}}/{{.Domain}}.error.log;
    location ^~ /.well-known/acme-challenge/ {
        root {{.AcmeRoot}};
        default_type "text/plain";
        try_files $uri =404;
    }
{{- end}}
server {
    listen 80;
    listen [::]:80;
{{template "common" .}}
{{- if .ForceHTTPS}}
    location / {
        return 301 https://$host$request_uri;
    }
{{- else}}
{{template "body" .}}
{{template "extras" .}}
{{- end}}
}
{{if .SSL}}
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
{{template "common" .}}
    ssl_certificate /etc/letsencrypt/live/{{.Domain}}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{{.Domain}}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:SSL:10m;
{{template "body" .}}
{{template "extras" .}}
}
{{end}}`))

type vhostData struct {
	*Website
	SSL      bool
	LogDir   string
	AcmeRoot string
}

// Nginx manages vhost files under sites-available/sites-enabled.
type Nginx struct {
	Run       system.Runner
	Available string
	Enabled   string
	LogDir    string
	AcmeRoot  string
}

func (n *Nginx) confName(domain string) string { return "rpanel-" + domain + ".conf" }
func (n *Nginx) confPath(d string) string      { return filepath.Join(n.Available, n.confName(d)) }
func (n *Nginx) linkPath(d string) string      { return filepath.Join(n.Enabled, n.confName(d)) }

// Render produces the vhost text for a site.
func (n *Nginx) Render(w *Website) (string, error) {
	if err := Validate(w); err != nil {
		return "", err
	}
	var b bytes.Buffer
	err := vhostTmpl.Execute(&b, vhostData{Website: w, SSL: w.SSLEnabled, LogDir: n.LogDir, AcmeRoot: n.AcmeRoot})
	return b.String(), err
}

func (n *Nginx) test(ctx context.Context) error {
	res, err := n.Run.Run(ctx, 30*time.Second, "nginx", "-t")
	if err != nil {
		return fmt.Errorf("nginx config test failed: %s", strings.TrimSpace(res.Stderr+" "+err.Error()))
	}
	return nil
}

func (n *Nginx) reload(ctx context.Context) error {
	if res, err := n.Run.Run(ctx, 30*time.Second, "systemctl", "reload", "nginx"); err != nil {
		return fmt.Errorf("nginx reload failed: %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
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
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (n *Nginx) setLink(domain string, enabled bool) error {
	link := n.linkPath(domain)
	if !enabled {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if _, err := os.Lstat(link); err == nil {
		return nil
	}
	return os.Symlink(n.confPath(domain), link)
}

func (n *Nginx) isLinked(domain string) bool {
	_, err := os.Lstat(n.linkPath(domain))
	return err == nil
}

// Apply writes the vhost, enables or disables it, validates with `nginx -t`
// and reloads. On any failure the previous files/links are restored, so a bad
// config never stays on disk.
func (n *Nginx) Apply(ctx context.Context, w *Website, enable bool) error {
	text, err := n.Render(w)
	if err != nil {
		return err
	}
	conf := n.confPath(w.Domain)
	old, readErr := os.ReadFile(conf)
	hadOld := readErr == nil
	wasLinked := n.isLinked(w.Domain)

	rollback := func() {
		if hadOld {
			_ = writeAtomic(conf, old)
		} else {
			_ = os.Remove(conf)
		}
		_ = n.setLink(w.Domain, wasLinked)
	}
	if err := writeAtomic(conf, []byte(text)); err != nil {
		return err
	}
	if err := n.setLink(w.Domain, enable); err != nil {
		rollback()
		return err
	}
	if err := n.test(ctx); err != nil {
		rollback()
		return err
	}
	if err := n.reload(ctx); err != nil {
		return err // config is valid; leave it in place
	}
	return nil
}

// Remove deletes the vhost and reloads nginx.
func (n *Nginx) Remove(ctx context.Context, domain string) error {
	if !system.ValidDomain(domain) {
		return ErrBadInput
	}
	_ = n.setLink(domain, false)
	if err := os.Remove(n.confPath(domain)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := n.test(ctx); err != nil {
		return err
	}
	return n.reload(ctx)
}
