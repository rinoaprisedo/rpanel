// Package updates lists, installs and schedules operating-system package updates (apt) and can
// restart the server, so nobody has to open a terminal for routine patching.
package updates

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"rpanel/backend/system"
)

var (
	ErrBadInput = errors.New("invalid input")
	ErrBusy     = errors.New("an update job is already running")

	instRe = regexp.MustCompile(`^Inst (\S+)(?: \[([^\]]+)\])? \((\S+) ([^)]*)\)`)
	pkgRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9+.:~-]{0,100}$`)
)

// aptOpts keep apt non-interactive, wait for the dpkg lock instead of failing, and never overwrite
// configuration files the administrator edited.
var aptOpts = []string{"-y", "-o", "DPkg::Lock::Timeout=300", "-o", "Dpkg::Options::=--force-confold", "-o", "Dpkg::Options::=--force-confdef"}

// needrestart would otherwise restart services (possibly this panel) in the middle of a job.
var aptEnv = []string{"NEEDRESTART_MODE=l", "NEEDRESTART_SUSPEND=1"}

type Package struct {
	Name     string `json:"name"`
	Current  string `json:"current"`
	New      string `json:"new"`
	Security bool   `json:"security"`
}

type Status struct {
	Known          bool      `json:"known"`
	Total          int       `json:"total"`
	Security       int       `json:"security"`
	Packages       []Package `json:"packages"`
	RebootRequired bool      `json:"reboot_required"`
	RebootPackages []string  `json:"reboot_packages"`
	CheckedAt      int64     `json:"checked_at"`
	AutoSecurity   bool      `json:"auto_security"`
	AutoInstalled  bool      `json:"auto_installed"`
	Job            *Progress `json:"job"`
}

type Paths struct {
	RebootFile string // /var/run/reboot-required (+ ".pkgs")
	AutoConf   string // apt.conf.d file owned by the panel
}

func DefaultPaths() Paths {
	return Paths{RebootFile: "/var/run/reboot-required", AutoConf: "/etc/apt/apt.conf.d/52rpanel-auto-upgrades"}
}

type Service struct {
	run  system.Runner
	p    Paths
	dpkg func() bool // is unattended-upgrades installed

	mu       sync.Mutex
	pkgs     []Package
	checked  time.Time
	job      *job
	onChange func()
	rebootFn func() error
}

func NewService(run system.Runner, p Paths) *Service {
	s := &Service{run: run, p: p}
	s.dpkg = func() bool {
		res, err := run.Run(context.Background(), 15*time.Second, "dpkg", "-s", "unattended-upgrades")
		return err == nil && strings.Contains(res.Stdout, "Status: install ok installed")
	}
	s.rebootFn = func() error {
		_, err := run.Run(context.Background(), 30*time.Second, "systemctl", "reboot")
		return err
	}
	return s
}

// OnChange registers a callback fired when the pending-update numbers changed (the dashboard refreshes its cache).
func (s *Service) OnChange(f func()) { s.onChange = f }

// parseSimulation reads `apt-get -s upgrade` output.
func parseSimulation(out string) []Package {
	var pk []Package
	for _, l := range strings.Split(out, "\n") {
		m := instRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		pk = append(pk, Package{Name: m[1], Current: m[2], New: m[3], Security: strings.Contains(m[4], "-security")})
	}
	sort.SliceStable(pk, func(i, j int) bool {
		if pk[i].Security != pk[j].Security {
			return pk[i].Security
		}
		return pk[i].Name < pk[j].Name
	})
	return pk
}

func (s *Service) simulate(ctx context.Context) ([]Package, error) {
	res, err := s.run.Run(ctx, 3*time.Minute, "apt-get", "-s", "-o", "DPkg::Lock::Timeout=60", "upgrade")
	if err != nil {
		return nil, fmt.Errorf("cannot list updates: %s", tail(res.Stderr+res.Stdout, 3))
	}
	return parseSimulation(res.Stdout), nil
}

func tail(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

// Refresh recounts pending updates from the local package lists (no network).
func (s *Service) Refresh(ctx context.Context) error {
	pk, err := s.simulate(ctx)
	if err != nil {
		return err
	}
	s.setPackages(pk)
	return nil
}

func (s *Service) setPackages(pk []Package) {
	s.mu.Lock()
	s.pkgs, s.checked = pk, time.Now()
	cb := s.onChange
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (s *Service) rebootInfo() (bool, []string) {
	if _, err := os.Stat(s.p.RebootFile); err != nil {
		return false, nil
	}
	var pk []string
	if b, err := os.ReadFile(s.p.RebootFile + ".pkgs"); err == nil {
		for _, l := range strings.Fields(string(b)) {
			pk = append(pk, l)
		}
	}
	return true, pk
}

func (s *Service) autoEnabled() bool {
	b, err := os.ReadFile(s.p.AutoConf)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), `APT::Periodic::Unattended-Upgrade "1"`)
}

// Status returns the cached package list (refreshed on first use and after each job).
func (s *Service) Status(ctx context.Context) *Status {
	s.mu.Lock()
	known := !s.checked.IsZero()
	s.mu.Unlock()
	if !known {
		_ = s.Refresh(ctx)
	}
	autoInstalled := s.dpkg() // runs dpkg: keep it outside the lock
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &Status{Known: !s.checked.IsZero(), Packages: append([]Package{}, s.pkgs...), AutoInstalled: autoInstalled, AutoSecurity: s.autoEnabled()}
	if st.Known {
		st.CheckedAt = s.checked.Unix()
	}
	st.Total = len(st.Packages)
	for _, p := range st.Packages {
		if p.Security {
			st.Security++
		}
	}
	if len(st.Packages) > 500 {
		st.Packages = st.Packages[:500]
	}
	st.RebootRequired, st.RebootPackages = s.rebootInfo()
	if s.job != nil {
		st.Job = s.job.snapshot()
	}
	return st
}

// ---- background job

const (
	KindCheck    = "check"
	KindSecurity = "security"
	KindAll      = "all"
)

type Step struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"` // pending | running | done | failed
	Detail string `json:"detail,omitempty"`
}

type Progress struct {
	Kind    string   `json:"kind"`
	State   string   `json:"state"` // running | done | failed
	Percent int      `json:"percent"`
	Steps   []Step   `json:"steps"`
	Log     []string `json:"log"`
	Error   string   `json:"error,omitempty"`
	Elapsed int      `json:"elapsed_seconds"`
	Done    int      `json:"installed"`
	Total   int      `json:"total"`
}

type job struct {
	mu       sync.Mutex
	kind     string
	state    string
	percent  int
	steps    []Step
	log      []string
	err      string
	started  time.Time
	finished time.Time
	done     int
	total    int
}

func newJob(kind string) *job {
	steps := []Step{{Key: "refresh", Label: "Refreshing the package lists", Status: "pending"}}
	switch kind {
	case KindCheck:
	default:
		steps = append(steps, Step{Key: "install", Label: "Downloading and installing updates", Status: "pending"})
	}
	steps = append(steps, Step{Key: "verify", Label: "Verifying the result", Status: "pending"})
	return &job{kind: kind, state: "running", steps: steps, started: time.Now()}
}

func (j *job) snapshot() *Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	end := j.finished
	if end.IsZero() {
		end = time.Now()
	}
	return &Progress{Kind: j.kind, State: j.state, Percent: j.percent, Steps: append([]Step(nil), j.steps...), Log: append([]string(nil), j.log...),
		Error: j.err, Elapsed: int(end.Sub(j.started).Seconds()), Done: j.done, Total: j.total}
}

func (j *job) logLine(l string) {
	j.mu.Lock()
	j.log = append(j.log, l)
	if len(j.log) > 400 {
		j.log = j.log[len(j.log)-400:]
	}
	j.mu.Unlock()
}

func (j *job) step(key, status, detail string, percent int) {
	j.mu.Lock()
	for i := range j.steps {
		if j.steps[i].Key == key {
			j.steps[i].Status, j.steps[i].Detail = status, detail
		}
	}
	if percent > j.percent {
		j.percent = percent
	}
	j.mu.Unlock()
}

func (j *job) bump(p int) {
	j.mu.Lock()
	if p > j.percent {
		j.percent = p
	}
	j.mu.Unlock()
}

// Start begins a job in the background. Only one job runs at a time.
func (s *Service) Start(kind string) (*Progress, error) {
	switch kind {
	case KindCheck, KindSecurity, KindAll:
	default:
		return nil, fmt.Errorf("%w: kind", ErrBadInput)
	}
	s.mu.Lock()
	if s.job != nil && s.job.snapshot().State == "running" {
		s.mu.Unlock()
		return nil, ErrBusy
	}
	j := newJob(kind)
	s.job = j
	s.mu.Unlock()
	go s.runJob(j)
	return j.snapshot(), nil
}

// Progress returns the latest job (nil when none ran since the panel started).
func (s *Service) Progress() *Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return nil
	}
	return s.job.snapshot()
}

func (s *Service) fail(j *job, key, detail string, err error) {
	j.step(key, "failed", detail, 0)
	j.mu.Lock()
	j.state, j.err, j.finished = "failed", err.Error(), time.Now()
	j.mu.Unlock()
}

func (s *Service) runJob(j *job) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	line := func(l string) { j.logLine(l) }

	j.step("refresh", "running", "", 3)
	j.logLine("$ apt-get update")
	if res, err := system.RunLines(ctx, s.run, 10*time.Minute, aptEnv, line, "apt-get", "update", "-o", "DPkg::Lock::Timeout=300"); err != nil {
		s.fail(j, "refresh", "apt-get update failed", fmt.Errorf("apt-get update failed: %s", tail(res.Stderr+res.Stdout, 4)))
		return
	}
	j.step("refresh", "done", "", 12)

	pk, err := s.simulate(ctx)
	if err != nil {
		s.fail(j, "refresh", "cannot list updates", err)
		return
	}
	s.setPackages(pk)

	if j.kind != KindCheck {
		var names []string
		for _, p := range pk {
			if (j.kind == KindAll || p.Security) && pkgRe.MatchString(p.Name) {
				names = append(names, p.Name)
			}
		}
		j.mu.Lock()
		j.total = len(names)
		j.mu.Unlock()
		if len(names) == 0 {
			j.step("install", "done", "Nothing to install", 90)
		} else if err := s.install(ctx, j, names); err != nil {
			s.fail(j, "install", "installation failed", err)
			return
		}
	}

	j.step("verify", "running", "", 95)
	if pk, err := s.simulate(ctx); err == nil {
		s.setPackages(pk)
	}
	j.step("verify", "done", "", 100)
	j.mu.Lock()
	j.state, j.percent, j.finished = "done", 100, time.Now()
	j.mu.Unlock()
}

func (s *Service) install(ctx context.Context, j *job, names []string) error {
	j.step("install", "running", fmt.Sprintf("%d packages", len(names)), 15)
	var args []string
	if j.kind == KindAll {
		args = append(append([]string{}, aptOpts...), "upgrade")
	} else {
		args = append(append(append([]string{}, aptOpts...), "install", "--only-upgrade"), names...)
	}
	j.logLine("$ apt-get " + strings.Join(args[:min(len(args), 12)], " "))
	n := len(names)
	var get, unpack, setup int
	onLine := func(l string) {
		if !strings.HasPrefix(l, "(Reading database") && !strings.HasPrefix(l, "Reading database ...") {
			j.logLine(l) // dpkg prints one progress line per percent: skip that noise
		}
		switch {
		case strings.HasPrefix(l, "Get:"):
			get++
		case strings.HasPrefix(l, "Unpacking "):
			unpack++
		case strings.HasPrefix(l, "Setting up "):
			setup++
			j.mu.Lock()
			j.done = min(setup, n)
			j.mu.Unlock()
		}
		// downloads 15-40%, unpacking 40-70%, configuring 70-94%
		pct := 15 + 25*min(get, n)/n + 30*min(unpack, n)/n + 24*min(setup, n)/n
		j.bump(min(pct, 94))
	}
	res, err := system.RunLines(ctx, s.run, 40*time.Minute, aptEnv, onLine, "apt-get", args...)
	if err != nil {
		return fmt.Errorf("apt-get failed: %s", tail(res.Stderr+"\n"+res.Stdout, 6))
	}
	j.step("install", "done", fmt.Sprintf("%d packages", n), 92)
	return nil
}

// ---- automatic security updates

// SetAutoSecurity turns unattended security upgrades on or off (installing unattended-upgrades when needed).
func (s *Service) SetAutoSecurity(ctx context.Context, enabled bool) error {
	if enabled && !s.dpkg() {
		s.mu.Lock()
		busy := s.job != nil && s.job.snapshot().State == "running"
		s.mu.Unlock()
		if busy {
			return ErrBusy
		}
		if res, err := system.RunLines(ctx, s.run, 10*time.Minute, aptEnv, func(string) {}, "apt-get", append(append([]string{}, aptOpts...), "install", "unattended-upgrades")...); err != nil {
			return fmt.Errorf("cannot install unattended-upgrades: %s", tail(res.Stderr+res.Stdout, 4))
		}
	}
	v := "0"
	if enabled {
		v = "1"
	}
	body := fmt.Sprintf("// Managed by RPanel (Updates page).\nAPT::Periodic::Update-Package-Lists \"%s\";\nAPT::Periodic::Unattended-Upgrade \"%s\";\n", v, v)
	if err := os.MkdirAll(filepath.Dir(s.p.AutoConf), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.p.AutoConf, []byte(body), 0o644); err != nil {
		return err
	}
	if enabled {
		_, _ = s.run.Run(ctx, time.Minute, "systemctl", "enable", "--now", "unattended-upgrades")
	}
	return nil
}

// Reboot restarts the machine shortly after returning, so the HTTP response can be delivered first.
func (s *Service) Reboot(confirm bool) error {
	if !confirm {
		return fmt.Errorf("%w: confirmation required", ErrBadInput)
	}
	s.mu.Lock()
	busy := s.job != nil && s.job.snapshot().State == "running"
	s.mu.Unlock()
	if busy {
		return ErrBusy
	}
	go func() {
		time.Sleep(2 * time.Second)
		_ = s.rebootFn()
	}()
	return nil
}
