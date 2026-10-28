// Package fail2ban manages the Fail2ban service installed from the App Store: live jail
// status, banned IPs (ban / unban), the main settings and a log viewer.
//
// Settings are written to /etc/fail2ban/jail.d/rpanel.local only, so a hand-written
// jail.local and the distribution files are never touched. A change is validated with
// `fail2ban-client -t` before it is reloaded and is rolled back when it is rejected.
package fail2ban

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rpanel/backend/system"
)

var (
	ErrBadInput     = errors.New("invalid input")
	ErrNotInstalled = errors.New("Fail2ban is not installed; install it from the App Store")

	jailNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	timeRe     = regexp.MustCompile(`^[0-9]{1,9}[smhdw]?$`)
	numRe      = regexp.MustCompile(`:\s*(\d+)\s*$`)
)

// jailDef is a jail the panel can switch on and off. Others keep working but are read-only here.
type jailDef struct {
	Name        string
	Title       string
	Description string
	Filter      string // file in filter.d that must exist
	LogPath     string // optional log file that must exist before the jail is enabled
}

var catalog = []jailDef{
	{Name: "sshd", Title: "SSH", Description: "Repeated failed SSH logins (brute force).", Filter: "sshd.conf"},
	{Name: "nginx-http-auth", Title: "Nginx basic auth", Description: "Failed HTTP basic-auth logins on nginx sites.", Filter: "nginx-http-auth.conf", LogPath: "/var/log/nginx/error.log"},
	{Name: "nginx-botsearch", Title: "Nginx bot scans", Description: "Bots probing for wp-login, phpmyadmin and other missing URLs.", Filter: "nginx-botsearch.conf", LogPath: "/var/log/nginx/access.log"},
	{Name: "nginx-limit-req", Title: "Nginx rate limit", Description: "Clients that exceed limit_req rate limits.", Filter: "nginx-limit-req.conf", LogPath: "/var/log/nginx/error.log"},
	{Name: "recidive", Title: "Repeat offenders", Description: "Bans IPs that keep getting banned, for a long time.", Filter: "recidive.conf", LogPath: "@fail2ban"},
}

// Paths are the host locations used; tests point them into a temp dir.
type Paths struct {
	ConfFile  string // jail.d/rpanel.local
	FilterDir string
	LogFile   string // fail2ban.log
}

// DefaultPaths are the Debian/Ubuntu locations.
func DefaultPaths() Paths {
	return Paths{ConfFile: "/etc/fail2ban/jail.d/rpanel.local", FilterDir: "/etc/fail2ban/filter.d", LogFile: "/var/log/fail2ban.log"}
}

type Service struct {
	run       system.Runner
	p         Paths
	installed func() bool
}

func NewService(run system.Runner, p Paths) *Service {
	return &Service{run: run, p: p, installed: func() bool { _, err := exec.LookPath("fail2ban-client"); return err == nil }}
}

func (s *Service) cmd(ctx context.Context, args ...string) (string, error) {
	res, err := s.run.Run(ctx, 20*time.Second, "fail2ban-client", args...)
	out := strings.TrimSpace(res.Stdout)
	if err != nil {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = out
		}
		if msg == "" {
			msg = err.Error()
		}
		return out, errors.New(lastLines(msg, 4))
	}
	return out, nil
}

func lastLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

// Installed reports whether the fail2ban-client binary exists (cheap; used by the sidebar).
func (s *Service) Installed() bool { return s.installed() }

type Jail struct {
	Name            string   `json:"name"`
	Title           string   `json:"title"`
	Managed         bool     `json:"managed"` // can be toggled from the panel
	CurrentlyFailed int      `json:"currently_failed"`
	TotalFailed     int      `json:"total_failed"`
	CurrentlyBanned int      `json:"currently_banned"`
	TotalBanned     int      `json:"total_banned"`
	BannedIPs       []string `json:"banned_ips"`
	BanTime         string   `json:"ban_time"`
	FindTime        string   `json:"find_time"`
	MaxRetry        string   `json:"max_retry"`
}

type Status struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version"`
	Jails     []Jail `json:"jails"`
	ClientIP  string `json:"client_ip"`
	Error     string `json:"error,omitempty"`
}

func title(name string) string {
	for _, d := range catalog {
		if d.Name == name {
			return d.Title
		}
	}
	return name
}

func managed(name string) bool {
	for _, d := range catalog {
		if d.Name == name {
			return true
		}
	}
	return false
}

// Status reads the live state from the running daemon.
func (s *Service) Status(ctx context.Context, clientIP string) *Status {
	st := &Status{Installed: s.Installed(), ClientIP: clientIP, Jails: []Jail{}}
	if !st.Installed {
		return st
	}
	if v, err := s.cmd(ctx, "version"); err == nil {
		st.Version = strings.TrimSpace(v)
	}
	if out, err := s.cmd(ctx, "ping"); err != nil || !strings.Contains(out, "pong") {
		return st
	}
	st.Running = true
	out, err := s.cmd(ctx, "status")
	if err != nil {
		st.Error = err.Error()
		return st
	}
	for _, name := range parseJailList(out) {
		j := s.jailStatus(ctx, name)
		st.Jails = append(st.Jails, j)
	}
	return st
}

func parseJailList(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "Jail list:"); i >= 0 {
			for _, n := range strings.Split(line[i+len("Jail list:"):], ",") {
				if n = strings.TrimSpace(n); n != "" && jailNameRe.MatchString(n) {
					names = append(names, n)
				}
			}
		}
	}
	return names
}

func (s *Service) jailStatus(ctx context.Context, name string) Jail {
	j := Jail{Name: name, Title: title(name), Managed: managed(name), BannedIPs: []string{}}
	out, err := s.cmd(ctx, "status", name)
	if err == nil {
		parseJail(out, &j)
	}
	j.BanTime, _ = s.cmd(ctx, "get", name, "bantime")
	j.FindTime, _ = s.cmd(ctx, "get", name, "findtime")
	j.MaxRetry, _ = s.cmd(ctx, "get", name, "maxretry")
	return j
}

func parseJail(out string, j *Jail) {
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimLeft(line, "|`- \t")
		switch {
		case strings.HasPrefix(l, "Currently failed"):
			j.CurrentlyFailed = lastInt(l)
		case strings.HasPrefix(l, "Total failed"):
			j.TotalFailed = lastInt(l)
		case strings.HasPrefix(l, "Currently banned"):
			j.CurrentlyBanned = lastInt(l)
		case strings.HasPrefix(l, "Total banned"):
			j.TotalBanned = lastInt(l)
		case strings.HasPrefix(l, "Banned IP list"):
			if i := strings.Index(l, ":"); i >= 0 {
				for _, ip := range strings.Fields(l[i+1:]) {
					if net.ParseIP(ip) != nil {
						j.BannedIPs = append(j.BannedIPs, ip)
					}
				}
			}
		}
	}
}

func lastInt(l string) int {
	if m := numRe.FindStringSubmatch(l); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// ---- ban / unban

func (s *Service) activeJail(ctx context.Context, name string) error {
	if !jailNameRe.MatchString(name) {
		return fmt.Errorf("%w: jail name", ErrBadInput)
	}
	out, err := s.cmd(ctx, "status")
	if err != nil {
		return err
	}
	for _, n := range parseJailList(out) {
		if n == name {
			return nil
		}
	}
	return fmt.Errorf("%w: jail %q is not running", ErrBadInput, name)
}

func parseIP(v string) (string, error) {
	v = strings.TrimSpace(v)
	ip := net.ParseIP(v)
	if ip == nil {
		return "", fmt.Errorf("%w: %q is not an IP address", ErrBadInput, v)
	}
	return ip.String(), nil
}

// Ban bans ip in jail. The requester's own address, loopback and unspecified addresses are refused.
func (s *Service) Ban(ctx context.Context, jail, ip, clientIP string) error {
	if !s.Installed() {
		return ErrNotInstalled
	}
	addr, err := parseIP(ip)
	if err != nil {
		return err
	}
	parsed := net.ParseIP(addr)
	if parsed.IsLoopback() || parsed.IsUnspecified() {
		return fmt.Errorf("%w: this address cannot be banned", ErrBadInput)
	}
	if c := net.ParseIP(clientIP); c != nil && c.Equal(parsed) {
		return fmt.Errorf("%w: that is your own IP address; banning it would lock you out", ErrBadInput)
	}
	if err := s.activeJail(ctx, jail); err != nil {
		return err
	}
	_, err = s.cmd(ctx, "set", jail, "banip", addr)
	return err
}

// Unban removes ip from jail.
func (s *Service) Unban(ctx context.Context, jail, ip string) error {
	if !s.Installed() {
		return ErrNotInstalled
	}
	addr, err := parseIP(ip)
	if err != nil {
		return err
	}
	if err := s.activeJail(ctx, jail); err != nil {
		return err
	}
	_, err = s.cmd(ctx, "set", jail, "unbanip", addr)
	return err
}

// ---- settings

type JailSetting struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Available   bool   `json:"available"`   // filter exists (and log file when needed)
	Unavailable string `json:"unavailable"` // reason when not available
	Enabled     bool   `json:"enabled"`
	BanTime     string `json:"ban_time"` // "" = inherit the defaults
	FindTime    string `json:"find_time"`
	MaxRetry    int    `json:"max_retry"` // 0 = inherit
}

type Settings struct {
	BanTime  string        `json:"ban_time"`
	FindTime string        `json:"find_time"`
	MaxRetry int           `json:"max_retry"`
	IgnoreIP []string      `json:"ignore_ip"`
	Jails    []JailSetting `json:"jails"`
}

func defaults() Settings {
	return Settings{BanTime: "10m", FindTime: "10m", MaxRetry: 5, IgnoreIP: []string{"127.0.0.1/8", "::1"}}
}

func (s *Service) available(d jailDef) (bool, string) {
	if _, err := os.Stat(filepath.Join(s.p.FilterDir, d.Filter)); err != nil {
		return false, "this Fail2ban version has no filter for it"
	}
	if d.LogPath != "" {
		lp := d.LogPath
		if lp == "@fail2ban" {
			lp = s.p.LogFile
		}
		if _, err := os.Stat(lp); err != nil {
			return false, lp + " does not exist yet (install the service that writes it first)"
		}
	}
	return true, ""
}

// Settings returns the values stored in rpanel.local merged with the defaults.
func (s *Service) Settings(ctx context.Context) (*Settings, error) {
	if !s.Installed() {
		return nil, ErrNotInstalled
	}
	set := defaults()
	sections := map[string]map[string]string{}
	if b, err := os.ReadFile(s.p.ConfFile); err == nil {
		sections = parseINI(string(b))
	}
	if d := sections["DEFAULT"]; d != nil {
		if v := d["bantime"]; v != "" {
			set.BanTime = v
		}
		if v := d["findtime"]; v != "" {
			set.FindTime = v
		}
		if n, err := strconv.Atoi(d["maxretry"]); err == nil && n > 0 {
			set.MaxRetry = n
		}
		if v := d["ignoreip"]; v != "" {
			set.IgnoreIP = strings.Fields(strings.ReplaceAll(v, ",", " "))
		}
	}
	// A jail that is running without being in our file (e.g. sshd from the distribution) shows as enabled.
	running := map[string]bool{}
	if out, err := s.cmd(ctx, "status"); err == nil {
		for _, n := range parseJailList(out) {
			running[n] = true
		}
	}
	for _, d := range catalog {
		ok, why := s.available(d)
		js := JailSetting{Name: d.Name, Title: d.Title, Description: d.Description, Available: ok, Unavailable: why, Enabled: running[d.Name]}
		if sec := sections[d.Name]; sec != nil {
			if v, ok := sec["enabled"]; ok {
				js.Enabled = strings.EqualFold(v, "true")
			}
			js.BanTime, js.FindTime = sec["bantime"], sec["findtime"]
			if n, err := strconv.Atoi(sec["maxretry"]); err == nil {
				js.MaxRetry = n
			}
		}
		set.Jails = append(set.Jails, js)
	}
	return &set, nil
}

func validTime(v string, allowForever bool) bool {
	if allowForever && v == "-1" {
		return true
	}
	return timeRe.MatchString(v)
}

// Validate checks every value that is written into the config file.
func (s *Service) Validate(in *Settings) error {
	if !validTime(in.BanTime, true) {
		return fmt.Errorf("%w: ban time must look like 600, 10m, 1h, 7d or -1 (permanent)", ErrBadInput)
	}
	if !validTime(in.FindTime, false) {
		return fmt.Errorf("%w: find time must look like 600, 10m or 1h", ErrBadInput)
	}
	if in.MaxRetry < 1 || in.MaxRetry > 1000 {
		return fmt.Errorf("%w: max retries must be between 1 and 1000", ErrBadInput)
	}
	if len(in.IgnoreIP) > 100 {
		return fmt.Errorf("%w: too many ignored addresses", ErrBadInput)
	}
	for i, v := range in.IgnoreIP {
		v = strings.TrimSpace(v)
		if net.ParseIP(v) == nil {
			if _, _, err := net.ParseCIDR(v); err != nil {
				return fmt.Errorf("%w: %q is not an IP address or CIDR range", ErrBadInput, v)
			}
		}
		in.IgnoreIP[i] = v
	}
	seen := map[string]bool{}
	for _, j := range in.Jails {
		def := findDef(j.Name)
		if def == nil {
			return fmt.Errorf("%w: unknown jail %q", ErrBadInput, j.Name)
		}
		if seen[j.Name] {
			return fmt.Errorf("%w: duplicate jail %q", ErrBadInput, j.Name)
		}
		seen[j.Name] = true
		if j.BanTime != "" && !validTime(j.BanTime, true) {
			return fmt.Errorf("%w: %s: ban time", ErrBadInput, j.Name)
		}
		if j.FindTime != "" && !validTime(j.FindTime, false) {
			return fmt.Errorf("%w: %s: find time", ErrBadInput, j.Name)
		}
		if j.MaxRetry < 0 || j.MaxRetry > 1000 {
			return fmt.Errorf("%w: %s: max retries", ErrBadInput, j.Name)
		}
		if j.Enabled {
			if ok, why := s.available(*def); !ok {
				return fmt.Errorf("%w: %s cannot be enabled: %s", ErrBadInput, j.Name, why)
			}
		}
	}
	return nil
}

func findDef(name string) *jailDef {
	for i := range catalog {
		if catalog[i].Name == name {
			return &catalog[i]
		}
	}
	return nil
}

func render(in *Settings) string {
	var b strings.Builder
	b.WriteString("# Managed by RPanel (Fail2ban page). Do not edit: changes are overwritten.\n[DEFAULT]\n")
	fmt.Fprintf(&b, "bantime = %s\nfindtime = %s\nmaxretry = %d\n", in.BanTime, in.FindTime, in.MaxRetry)
	fmt.Fprintf(&b, "ignoreip = %s\n", strings.Join(in.IgnoreIP, " "))
	for _, j := range in.Jails {
		fmt.Fprintf(&b, "\n[%s]\nenabled = %t\n", j.Name, j.Enabled)
		if j.BanTime != "" {
			fmt.Fprintf(&b, "bantime = %s\n", j.BanTime)
		}
		if j.FindTime != "" {
			fmt.Fprintf(&b, "findtime = %s\n", j.FindTime)
		}
		if j.MaxRetry > 0 {
			fmt.Fprintf(&b, "maxretry = %d\n", j.MaxRetry)
		}
	}
	return b.String()
}

func parseINI(text string) map[string]map[string]string {
	out := map[string]map[string]string{}
	cur := ""
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";"):
		case strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]"):
			cur = l[1 : len(l)-1]
			out[cur] = map[string]string{}
		case cur != "":
			if i := strings.Index(l, "="); i > 0 {
				out[cur][strings.TrimSpace(l[:i])] = strings.TrimSpace(l[i+1:])
			}
		}
	}
	return out
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
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

// Save validates, writes rpanel.local, tests the whole configuration and reloads Fail2ban.
// The previous file is restored when Fail2ban rejects the result.
func (s *Service) Save(ctx context.Context, in Settings) (*Settings, error) {
	if !s.Installed() {
		return nil, ErrNotInstalled
	}
	if err := s.Validate(&in); err != nil {
		return nil, err
	}
	old, readErr := os.ReadFile(s.p.ConfFile)
	rollback := func() {
		if readErr == nil {
			_ = writeAtomic(s.p.ConfFile, old)
		} else {
			_ = os.Remove(s.p.ConfFile)
		}
	}
	if err := writeAtomic(s.p.ConfFile, []byte(render(&in))); err != nil {
		return nil, err
	}
	if _, err := s.cmd(ctx, "-t"); err != nil {
		rollback()
		return nil, fmt.Errorf("Fail2ban rejected the configuration: %w", err)
	}
	if _, err := s.cmd(ctx, "reload"); err != nil {
		rollback()
		_, _ = s.cmd(ctx, "reload")
		return nil, fmt.Errorf("Fail2ban could not reload: %w", err)
	}
	return s.Settings(ctx)
}

// ---- service & logs

// Control starts, stops, restarts or reloads the fail2ban unit.
func (s *Service) Control(ctx context.Context, action string) error {
	if !s.Installed() {
		return ErrNotInstalled
	}
	switch action {
	case "start", "stop", "restart", "reload":
	default:
		return fmt.Errorf("%w: action", ErrBadInput)
	}
	if res, err := s.run.Run(ctx, 60*time.Second, "systemctl", action, "fail2ban"); err != nil {
		return fmt.Errorf("systemctl %s fail2ban failed: %s", action, lastLines(res.Stderr, 3))
	}
	return nil
}

// Log returns the last n lines of fail2ban.log, newest last.
func (s *Service) Log(n int) ([]string, error) {
	if n < 1 || n > 1000 {
		n = 200
	}
	f, err := os.Open(s.p.LogFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	const window = 256 << 10
	start := int64(0)
	if fi.Size() > window {
		start = fi.Size() - window
	}
	buf := make([]byte, fi.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && len(buf) > 0 {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // first line may be cut
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}
	return lines, nil
}
