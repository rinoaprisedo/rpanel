// Package supervisor manages the Supervisor process manager installed from the App Store: the state of every
// program, start/stop/restart, logs, and programs created by the panel.
//
// Programs created here live in /etc/supervisor/conf.d/rpanel-<name>.conf (one file each, owned by the panel).
// Programs defined by hand elsewhere are listed and controllable but never edited. A change is applied with
// `supervisorctl reread` + `update` and rolled back when Supervisor rejects it.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"rpanel/backend/system"
)

var (
	ErrBadInput     = errors.New("invalid input")
	ErrNotFound     = errors.New("program not found")
	ErrNotInstalled = errors.New("Supervisor is not installed; install it from the App Store")

	nameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)
	procRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,95}$`)
	dirRe     = regexp.MustCompile(`^/[A-Za-z0-9._/-]*$`)
	userRe    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	envKeyRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	statusRe  = regexp.MustCompile(`^(\S+)\s+([A-Z]+)\s*(.*)$`)
	confNameR = regexp.MustCompile(`^rpanel-([a-z0-9][a-z0-9_-]{0,47})\.conf$`)
)

type Paths struct {
	ConfDir string // /etc/supervisor/conf.d
	LogDir  string // /var/log/supervisor
}

func DefaultPaths() Paths {
	return Paths{ConfDir: "/etc/supervisor/conf.d", LogDir: "/var/log/supervisor"}
}

type Service struct {
	run       system.Runner
	p         Paths
	installed func() bool
	userExist func(string) bool
}

func NewService(run system.Runner, p Paths) *Service {
	return &Service{run: run, p: p,
		installed: func() bool { _, err := exec.LookPath("supervisorctl"); return err == nil },
		userExist: func(u string) bool {
			res, err := run.Run(context.Background(), 5*time.Second, "id", "-u", u)
			return err == nil && strings.TrimSpace(res.Stdout) != ""
		},
	}
}

func (s *Service) Installed() bool { return s.installed() }

func tail(str string, n int) string {
	l := strings.Split(strings.TrimSpace(str), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

// ctl runs supervisorctl; it returns stdout even for the non-zero exit supervisorctl uses for "a program is not running".
func (s *Service) ctl(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	res, err := s.run.Run(ctx, timeout, "supervisorctl", args...)
	out := strings.TrimSpace(res.Stdout)
	if err != nil && out == "" {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(tail(msg, 4))
	}
	return out, nil
}

// ---- programs

type Config struct {
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Directory   string   `json:"directory"`
	User        string   `json:"user"`
	NumProcs    int      `json:"numprocs"`
	AutoStart   bool     `json:"autostart"`
	AutoRestart string   `json:"autorestart"` // true | false | unexpected
	Environment []string `json:"environment"` // KEY=value
}

type Process struct {
	Name    string  `json:"name"` // as supervisorctl names it (group:name for groups)
	Program string  `json:"program"`
	State   string  `json:"state"`
	Detail  string  `json:"detail"`
	Managed bool    `json:"managed"` // created by the panel, so it can be edited and deleted
	Config  *Config `json:"config,omitempty"`
}

type Status struct {
	Installed bool      `json:"installed"`
	Running   bool      `json:"running"`
	Version   string    `json:"version"`
	Processes []Process `json:"processes"`
	Error     string    `json:"error,omitempty"`
}

func parseStatus(out string) []Process {
	var ps []Process
	for _, l := range strings.Split(out, "\n") {
		m := statusRe.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil || !procRe.MatchString(m[1]) {
			continue
		}
		prog := m[1]
		if i := strings.Index(prog, ":"); i > 0 {
			prog = prog[:i]
		}
		ps = append(ps, Process{Name: m[1], Program: prog, State: m[2], Detail: strings.TrimSpace(m[3])})
	}
	return ps
}

// Status reads the live state of every process and marks the ones the panel manages.
func (s *Service) Status(ctx context.Context) *Status {
	st := &Status{Installed: s.Installed(), Processes: []Process{}}
	if !st.Installed {
		return st
	}
	if v, err := s.ctl(ctx, 10*time.Second, "version"); err == nil && !strings.Contains(strings.ToLower(v), "error") {
		st.Version = strings.TrimSpace(v)
	}
	out, err := s.ctl(ctx, 20*time.Second, "status")
	if err != nil || strings.Contains(out, "refused") || strings.Contains(out, "no such file") || strings.Contains(out, "unix://") {
		if err != nil {
			st.Error = err.Error()
		}
		return st
	}
	st.Running = true
	managed := map[string]*Config{}
	for _, c := range s.managedConfigs() {
		cc := c
		managed[c.Name] = &cc
	}
	for _, p := range parseStatus(out) {
		if c, ok := managed[p.Program]; ok {
			p.Managed, p.Config = true, c
		}
		st.Processes = append(st.Processes, p)
	}
	// A managed program that is configured but not (yet) known to supervisord still shows up.
	seen := map[string]bool{}
	for _, p := range st.Processes {
		seen[p.Program] = true
	}
	for name, c := range managed {
		if !seen[name] {
			st.Processes = append(st.Processes, Process{Name: name, Program: name, State: "NOT LOADED", Managed: true, Config: c})
		}
	}
	sort.SliceStable(st.Processes, func(i, j int) bool { return st.Processes[i].Name < st.Processes[j].Name })
	return st
}

// ---- control

func (s *Service) Action(ctx context.Context, action, name string) (string, error) {
	if !s.Installed() {
		return "", ErrNotInstalled
	}
	switch action {
	case "start", "stop", "restart":
	default:
		return "", fmt.Errorf("%w: action", ErrBadInput)
	}
	if !procRe.MatchString(name) {
		return "", fmt.Errorf("%w: program name", ErrBadInput)
	}
	out, err := s.ctl(ctx, 90*time.Second, action, name)
	if err != nil {
		return "", err
	}
	if strings.Contains(out, "ERROR") || strings.Contains(out, "FATAL") || strings.Contains(out, "can't") {
		return out, errors.New(tail(out, 3))
	}
	return out, nil
}

// Log returns the end of a process's stdout or stderr (supervisorctl tail).
func (s *Service) Log(ctx context.Context, name, stream string, bytes int) (string, error) {
	if !s.Installed() {
		return "", ErrNotInstalled
	}
	if !procRe.MatchString(name) {
		return "", fmt.Errorf("%w: program name", ErrBadInput)
	}
	if stream != "stdout" && stream != "stderr" {
		return "", fmt.Errorf("%w: stream", ErrBadInput)
	}
	if bytes < 100 || bytes > 200000 {
		bytes = 20000
	}
	out, err := s.run.Run(ctx, 20*time.Second, "supervisorctl", "tail", "-"+strconv.Itoa(bytes), name, stream)
	if err != nil && strings.TrimSpace(out.Stdout) == "" {
		return "", errors.New(tail(out.Stderr+" "+err.Error(), 3))
	}
	return out.Stdout, nil
}

// Daemon starts, stops or restarts supervisord itself.
func (s *Service) Daemon(ctx context.Context, action string) error {
	if !s.Installed() {
		return ErrNotInstalled
	}
	switch action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("%w: action", ErrBadInput)
	}
	if res, err := s.run.Run(ctx, 90*time.Second, "systemctl", action, "supervisor"); err != nil {
		return fmt.Errorf("systemctl %s supervisor failed: %s", action, tail(res.Stderr, 3))
	}
	return nil
}

// ---- managed program files

func (s *Service) confPath(name string) string {
	return filepath.Join(s.p.ConfDir, "rpanel-"+name+".conf")
}

// quoteEnv quotes an environment value the way supervisord's parser reads it: double quotes (with \ escaped),
// or single quotes when the value itself contains double quotes. A value holding both kinds cannot be expressed.
func quoteEnv(v string) (string, bool) {
	v = strings.ReplaceAll(v, "%", "%%")
	switch {
	case !strings.Contains(v, `"`):
		return `"` + strings.ReplaceAll(v, `\`, `\\`) + `"`, true
	case !strings.Contains(v, `'`):
		return `'` + v + `'`, true
	}
	return "", false
}

func render(c Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Managed by RPanel (Supervisor page). Do not edit: changes are overwritten.\n[program:%s]\n", c.Name)
	fmt.Fprintf(&b, "command=%s\n", strings.ReplaceAll(c.Command, "%", "%%"))
	if c.Directory != "" {
		fmt.Fprintf(&b, "directory=%s\n", c.Directory)
	}
	if c.User != "" {
		fmt.Fprintf(&b, "user=%s\n", c.User)
	}
	fmt.Fprintf(&b, "numprocs=%d\n", c.NumProcs)
	if c.NumProcs > 1 {
		b.WriteString("process_name=%(program_name)s_%(process_num)02d\n")
	}
	fmt.Fprintf(&b, "autostart=%t\nautorestart=%s\n", c.AutoStart, c.AutoRestart)
	b.WriteString("stopasgroup=true\nkillasgroup=true\n")
	if len(c.Environment) > 0 {
		var kv []string
		for _, e := range c.Environment {
			k, v, _ := strings.Cut(e, "=")
			q, _ := quoteEnv(v) // Validate rejected values that cannot be quoted
			kv = append(kv, k+"="+q)
		}
		fmt.Fprintf(&b, "environment=%s\n", strings.Join(kv, ","))
	}
	fmt.Fprintf(&b, "stdout_logfile=/var/log/supervisor/%s.out.log\nstderr_logfile=/var/log/supervisor/%s.err.log\n", c.Name, c.Name)
	b.WriteString("stdout_logfile_maxbytes=10MB\nstderr_logfile_maxbytes=10MB\nstdout_logfile_backups=3\nstderr_logfile_backups=3\n")
	return b.String()
}

func parseConf(text string) (Config, bool) {
	var c Config
	kv := map[string]string{}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[program:") && strings.HasSuffix(l, "]") {
			c.Name = l[len("[program:") : len(l)-1]
			continue
		}
		if i := strings.Index(l, "="); i > 0 && !strings.HasPrefix(l, "#") {
			kv[strings.TrimSpace(l[:i])] = strings.TrimSpace(l[i+1:])
		}
	}
	if c.Name == "" {
		return c, false
	}
	c.Command = strings.ReplaceAll(kv["command"], "%%", "%")
	c.Directory, c.User = kv["directory"], kv["user"]
	c.NumProcs, _ = strconv.Atoi(kv["numprocs"])
	if c.NumProcs < 1 {
		c.NumProcs = 1
	}
	c.AutoStart = kv["autostart"] != "false"
	c.AutoRestart = kv["autorestart"]
	if c.AutoRestart == "" {
		c.AutoRestart = "unexpected"
	}
	if env := kv["environment"]; env != "" {
		for _, part := range splitEnv(env) {
			c.Environment = append(c.Environment, part)
		}
	}
	return c, true
}

// splitEnv splits KEY="value",KEY2='v2' honouring both quote styles, and unquotes the values.
func splitEnv(s string) []string {
	var out []string
	var cur strings.Builder
	var q rune // active quote character, 0 outside quotes
	esc := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, strings.ReplaceAll(cur.String(), "%%", "%"))
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && q == '"':
			esc = true
		case q != 0 && r == q:
			q = 0
		case q == 0 && (r == '"' || r == '\''):
			q = r
		case r == ',' && q == 0:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func (s *Service) managedConfigs() []Config {
	var out []Config
	entries, err := os.ReadDir(s.p.ConfDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !confNameR.MatchString(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.p.ConfDir, e.Name()))
		if err != nil {
			continue
		}
		if c, ok := parseConf(string(b)); ok && confNameR.FindStringSubmatch(e.Name())[1] == c.Name {
			out = append(out, c)
		}
	}
	return out
}

// Validate checks every field that is written into the config file.
func (s *Service) Validate(c *Config) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Command = strings.TrimSpace(c.Command)
	if !nameRe.MatchString(c.Name) {
		return fmt.Errorf("%w: name must be lowercase letters, digits, - or _ (max 48)", ErrBadInput)
	}
	if c.Command == "" || len(c.Command) > 2000 || strings.ContainsAny(c.Command, "\x00\r\n") {
		return fmt.Errorf("%w: command must be one line", ErrBadInput)
	}
	if c.Directory != "" && (!dirRe.MatchString(c.Directory) || strings.Contains(c.Directory, "..")) {
		return fmt.Errorf("%w: directory must be an absolute path", ErrBadInput)
	}
	if c.User != "" {
		if !userRe.MatchString(c.User) {
			return fmt.Errorf("%w: user name", ErrBadInput)
		}
		if !s.userExist(c.User) {
			return fmt.Errorf("%w: the system user %q does not exist", ErrBadInput, c.User)
		}
	}
	if c.NumProcs < 1 || c.NumProcs > 32 {
		return fmt.Errorf("%w: processes must be between 1 and 32", ErrBadInput)
	}
	switch c.AutoRestart {
	case "true", "false", "unexpected":
	default:
		return fmt.Errorf("%w: autorestart", ErrBadInput)
	}
	if len(c.Environment) > 50 {
		return fmt.Errorf("%w: too many environment variables", ErrBadInput)
	}
	for _, e := range c.Environment {
		k, v, ok := strings.Cut(e, "=")
		if !ok || !envKeyRe.MatchString(k) || strings.ContainsAny(v, "\x00\r\n") || len(v) > 1000 {
			return fmt.Errorf("%w: environment entries look like KEY=value", ErrBadInput)
		}
		if _, ok := quoteEnv(v); !ok {
			return fmt.Errorf("%w: the value of %s cannot contain both ' and \" characters", ErrBadInput, k)
		}
	}
	return nil
}

func (s *Service) apply(ctx context.Context) error {
	out, err := s.ctl(ctx, 60*time.Second, "reread")
	if err != nil {
		return err
	}
	if strings.Contains(out, "ERROR") {
		return errors.New(tail(out, 3))
	}
	out, err = s.ctl(ctx, 120*time.Second, "update")
	if err != nil {
		return err
	}
	if strings.Contains(out, "ERROR") {
		return errors.New(tail(out, 3))
	}
	return nil
}

// Save creates or updates a managed program and loads it into supervisord. The file is restored if Supervisor rejects it.
func (s *Service) Save(ctx context.Context, c Config, create bool) (*Config, error) {
	if !s.Installed() {
		return nil, ErrNotInstalled
	}
	if err := s.Validate(&c); err != nil {
		return nil, err
	}
	path := s.confPath(c.Name)
	old, readErr := os.ReadFile(path)
	if create && readErr == nil {
		return nil, fmt.Errorf("%w: a program named %q already exists", ErrBadInput, c.Name)
	}
	if !create && readErr != nil {
		return nil, ErrNotFound
	}
	if create {
		for _, p := range parseStatusSafe(s, ctx) {
			if p.Program == c.Name {
				return nil, fmt.Errorf("%w: %q is already defined outside the panel", ErrBadInput, c.Name)
			}
		}
	}
	if err := os.MkdirAll(s.p.ConfDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(render(c)), 0o644); err != nil {
		return nil, err
	}
	if err := s.apply(ctx); err != nil {
		if readErr == nil {
			_ = os.WriteFile(path, old, 0o644)
		} else {
			_ = os.Remove(path)
		}
		_ = s.apply(ctx)
		return nil, fmt.Errorf("Supervisor rejected the configuration: %w", err)
	}
	return &c, nil
}

func parseStatusSafe(s *Service, ctx context.Context) []Process {
	out, err := s.ctl(ctx, 20*time.Second, "status")
	if err != nil {
		return nil
	}
	return parseStatus(out)
}

// Delete stops and removes a managed program.
func (s *Service) Delete(ctx context.Context, name string) error {
	if !s.Installed() {
		return ErrNotInstalled
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: program name", ErrBadInput)
	}
	path := s.confPath(name)
	if _, err := os.Stat(path); err != nil {
		return ErrNotFound
	}
	_, _ = s.ctl(ctx, 90*time.Second, "stop", name+":*")
	_, _ = s.ctl(ctx, 90*time.Second, "stop", name)
	if err := os.Remove(path); err != nil {
		return err
	}
	return s.apply(ctx)
}
