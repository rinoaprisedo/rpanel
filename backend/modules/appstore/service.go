package appstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"rpanel/backend/system"
)

var (
	ErrNotFound = errors.New("app not found")
	ErrBadState = errors.New("invalid state")
	ErrBadInput = errors.New("invalid input")
)

const (
	StateIdle       = "idle"
	StateInstalling = "installing"
	StateRemoving   = "removing"
	StateFailed     = "failed"
)

// VersionItem is one selectable version of a versioned app.
type VersionItem struct {
	Label     string   `json:"label"`
	Installed bool     `json:"installed"`
	Running   bool     `json:"running"`
	Version   string   `json:"version,omitempty"`
	Source    string   `json:"source"` // "distro" or "repo" (adds a vendor repository)
	Repo      string   `json:"repo,omitempty"`
	Packages  []string `json:"packages"`
}

// Item is the API representation of an App plus its live status on this host.
type Item struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Required    bool   `json:"required"`
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	HasService  bool   `json:"has_service"`
	Version     string `json:"version"`
	// Candidate is the version apt will install; only set for unversioned apps that are not installed.
	Candidate string        `json:"candidate,omitempty"`
	Packages  []string      `json:"packages"`
	Multi     bool          `json:"multi"`
	Versions  []VersionItem `json:"versions,omitempty"`
	State     string        `json:"state"`
	Error     string        `json:"error,omitempty"`
}

type job struct {
	state string
	err   string
}

type candidate struct {
	version string
	raw     string // apt-cache policy output; shows which repositories carry the package
	at      time.Time
}

type Service struct {
	run system.Runner
	// Seams replaced in tests.
	phpVersions func() []string
	osInfo      func() (id, codename string)
	fetch       func(ctx context.Context, url string) ([]byte, error)
	aptDir      string

	candMu sync.Mutex
	cands  map[string]candidate

	mu   sync.Mutex
	jobs map[string]*job
	// apt/dpkg hold a global lock, so only one operation may touch them at a time.
	aptMu sync.Mutex
	wg    sync.WaitGroup
}

func NewService(run system.Runner) *Service {
	return &Service{
		run: run, jobs: map[string]*job{}, cands: map[string]candidate{},
		phpVersions: installedPHP, osInfo: osRelease, fetch: httpFetch, aptDir: "/etc/apt",
	}
}

// installedPHP lists PHP-FPM versions that are really installed. /etc/php/<v>
// alone is not enough: uninstalling keeps the configuration directory.
func installedPHP() []string {
	dirs, _ := filepath.Glob("/etc/php/*/fpm")
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		v := filepath.Base(filepath.Dir(d))
		if _, err := os.Stat("/usr/sbin/php-fpm" + v); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func (a *App) checkPkg(label string) string {
	if a.VersionCheck != "" {
		return a.VersionCheck
	}
	return a.VersionPkgs(label, false)[0]
}

// matches reports whether apt candidate "10.11.14" belongs to version label "10.11".
func matches(cand, label string) bool {
	return cand != "" && (cand == label || strings.HasPrefix(cand, label+"."))
}

func (s *Service) List(ctx context.Context) []Item {
	s.mu.Lock()
	snap := make(map[string]job, len(s.jobs))
	for k, v := range s.jobs {
		snap[k] = *v
	}
	s.mu.Unlock()

	out := make([]Item, 0, len(Catalog))
	for _, a := range Catalog {
		it := Item{
			ID: a.ID, Name: a.Name, Category: a.Category, Description: a.Description,
			Icon: a.Icon, Color: a.Color, Required: a.Required, State: StateIdle,
			Packages: append([]string{}, a.Packages...), Multi: a.Multi,
		}
		s.fill(ctx, &a, &it)
		if a.Versions != nil {
			s.fillVersions(ctx, &a, &it)
		} else if !it.Installed {
			it.Candidate = s.candidateVersion(ctx, a.Packages[0])
		}
		if j, ok := snap[a.ID]; ok {
			it.State, it.Error = j.state, j.err
		}
		out = append(out, it)
	}
	return out
}

func (s *Service) fill(ctx context.Context, a *App, it *Item) {
	if a.Multi {
		vers := s.phpVersions()
		it.Installed = len(vers) > 0
		it.HasService = true
		if it.Installed {
			it.Version = strings.Join(vers, ", ")
			for _, v := range vers {
				if s.active(ctx, a.VersionUnit(v)) {
					it.Running = true
					break
				}
			}
		}
		return
	}
	for _, pkg := range a.Check {
		if v, ok := s.dpkgVersion(ctx, pkg); ok {
			it.Installed, it.Version = true, v
			break
		}
	}
	if a.Unit != "" {
		it.HasService = true
		if it.Installed {
			it.Running = s.active(ctx, a.Unit)
		}
	}
}

func (s *Service) fillVersions(ctx context.Context, a *App, it *Item) {
	installedLabels := s.phpVersions()
	for _, label := range a.Versions {
		v := VersionItem{Label: label}
		if a.Multi {
			v.Installed = slices.Contains(installedLabels, label)
			if v.Installed {
				v.Version = label
				v.Running = s.active(ctx, a.VersionUnit(label))
			}
		} else if it.Installed && matches(it.Version, label) {
			v.Installed, v.Version, v.Running = true, it.Version, it.Running
		}
		viaRepo := s.viaRepo(ctx, a, label, false)
		v.Source = "distro"
		if viaRepo {
			v.Source, v.Repo = "repo", a.Repo.Name
		}
		v.Packages = a.VersionPkgs(label, viaRepo)
		it.Versions = append(it.Versions, v)
	}
}

// candidateInfo asks apt which version an install would pick and what apt-cache
// says about it. apt-cache is cheap but the list is polled while installing, so
// results are cached briefly.
func (s *Service) candidateInfo(ctx context.Context, pkg string, fresh bool) candidate {
	s.candMu.Lock()
	defer s.candMu.Unlock()
	if c, ok := s.cands[pkg]; ok && !fresh && time.Since(c.at) < 10*time.Minute {
		return c
	}
	c := candidate{at: time.Now()}
	if res, err := s.run.Run(ctx, 10*time.Second, "apt-cache", "policy", pkg); err == nil {
		c.raw = res.Stdout
		for _, l := range strings.Split(res.Stdout, "\n") {
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "Candidate:") {
				if v := strings.TrimSpace(strings.TrimPrefix(t, "Candidate:")); v != "(none)" {
					c.version = cleanVersion(v)
				}
			}
		}
	}
	s.cands[pkg] = c
	return c
}

func (s *Service) candidateVersion(ctx context.Context, pkg string) string {
	return s.candidateInfo(ctx, pkg, false).version
}

// viaRepo reports whether installing label means using a vendor repository:
// either the distribution lacks that version, or the vendor repository is
// already configured and carries it.
func (s *Service) viaRepo(ctx context.Context, a *App, label string, fresh bool) bool {
	c := s.candidateInfo(ctx, a.checkPkg(label), fresh)
	if !matches(c.version, label) {
		return true
	}
	osID, codename := s.osInfo()
	src, ok := a.Repo.Source(osID, codename, label)
	if !ok {
		return false
	}
	uri := strings.TrimPrefix(strings.Fields(src)[0], "https://")
	return strings.Contains(c.raw, uri)
}

func (s *Service) dropCandidates() {
	s.candMu.Lock()
	s.cands = map[string]candidate{}
	s.candMu.Unlock()
}

func (s *Service) active(ctx context.Context, unit string) bool {
	res, err := s.run.Run(ctx, 5*time.Second, "systemctl", "is-active", unit)
	return err == nil && strings.TrimSpace(res.Stdout) == "active"
}

// dpkgVersion reports whether pkg is fully installed and its upstream version.
func (s *Service) dpkgVersion(ctx context.Context, pkg string) (string, bool) {
	res, err := s.run.Run(ctx, 5*time.Second, "dpkg", "-s", pkg)
	if err != nil {
		return "", false
	}
	installed, version := false, ""
	for _, l := range strings.Split(res.Stdout, "\n") {
		switch {
		case strings.HasPrefix(l, "Status:"):
			installed = strings.HasSuffix(strings.TrimSpace(l), "installed") && !strings.Contains(l, "not-installed") && !strings.Contains(l, "config-files")
		case strings.HasPrefix(l, "Version:"):
			version = cleanVersion(strings.TrimSpace(strings.TrimPrefix(l, "Version:")))
		}
	}
	return version, installed
}

// cleanVersion turns "1:10.11.14-0ubuntu0.24.04.1" into "10.11.14".
func cleanVersion(v string) string {
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:]
	}
	if i := strings.IndexAny(v, "-+~"); i >= 0 {
		v = v[:i]
	}
	return v
}

// validate resolves the app and the requested version label. Removing an app
// that can only have one version installed needs no label.
func (s *Service) validate(id, label string, removing bool) (*App, error) {
	a := find(id)
	if a == nil {
		return nil, ErrNotFound
	}
	if a.Versions == nil {
		if label != "" {
			return nil, fmt.Errorf("%w: %s has no versions", ErrBadInput, a.Name)
		}
		return a, nil
	}
	if removing && !a.Multi && label == "" {
		return a, nil
	}
	if !slices.Contains(a.Versions, label) {
		return nil, fmt.Errorf("%w: choose a %s version", ErrBadInput, a.Name)
	}
	return a, nil
}

// isInstalled tells whether this app (or, for multi-version apps, this version) is present.
func (s *Service) isInstalled(a *App, label string) bool {
	if a.Multi {
		return slices.Contains(s.phpVersions(), label)
	}
	var it Item
	s.fill(context.Background(), a, &it)
	return it.Installed
}

// Install starts installing app id (and version label, if it has versions) in the background.
func (s *Service) Install(id, label string) (*App, error) {
	a, err := s.validate(id, label, false)
	if err != nil {
		return nil, err
	}
	if s.isInstalled(a, label) {
		return nil, fmt.Errorf("%w: %s is already installed", ErrBadState, a.Name)
	}
	return s.start(a, StateInstalling, func(ctx context.Context) error { return s.doInstall(ctx, a, label) })
}

// Uninstall removes the packages (configuration and data are kept).
func (s *Service) Uninstall(id, label string) (*App, error) {
	a, err := s.validate(id, label, true)
	if err != nil {
		return nil, err
	}
	if a.Required {
		return nil, fmt.Errorf("%w: %s is required by RPanel", ErrBadState, a.Name)
	}
	if !s.isInstalled(a, label) {
		return nil, fmt.Errorf("%w: %s is not installed", ErrBadState, a.Name)
	}
	return s.start(a, StateRemoving, func(ctx context.Context) error { return s.doRemove(ctx, a, label) })
}

func (s *Service) start(a *App, state string, fn func(context.Context) error) (*App, error) {
	s.mu.Lock()
	if j, ok := s.jobs[a.ID]; ok && (j.state == StateInstalling || j.state == StateRemoving) {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: %s is busy", ErrBadState, a.Name)
	}
	s.jobs[a.ID] = &job{state: state}
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.aptMu.Lock()
		defer s.aptMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		err := fn(ctx)
		s.dropCandidates()
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.jobs[a.ID] = &job{state: StateFailed, err: err.Error()}
			return
		}
		delete(s.jobs, a.ID)
	}()
	return a, nil
}

func (s *Service) aptUpdate(ctx context.Context) error {
	if res, err := s.run.Run(ctx, 5*time.Minute, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt-get update failed: %s", tail(res.Stderr, 4))
	}
	return nil
}

func (s *Service) doInstall(ctx context.Context, a *App, label string) error {
	if err := s.aptUpdate(ctx); err != nil {
		return err
	}
	pkgs, unit := a.Packages, a.Unit
	if a.Versions != nil {
		viaRepo := s.viaRepo(ctx, a, label, true)
		if viaRepo && !matches(s.candidateVersion(ctx, a.checkPkg(label)), label) {
			if err := s.setupRepo(ctx, a, label); err != nil {
				return err
			}
			if err := s.aptUpdate(ctx); err != nil {
				return err
			}
		}
		pkgs = a.VersionPkgs(label, viaRepo)
		if a.VersionUnit != nil {
			unit = a.VersionUnit(label)
		}
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if res, err := s.run.Run(ctx, 20*time.Minute, "apt-get", args...); err != nil {
		return fmt.Errorf("install failed: %s", tail(res.Stderr, 6))
	}
	for _, p := range a.OptionalAny {
		if _, err := s.run.Run(ctx, 10*time.Minute, "apt-get", "install", "-y", p); err == nil {
			break
		}
	}
	if unit != "" {
		if res, err := s.run.Run(ctx, time.Minute, "systemctl", "enable", "--now", unit); err != nil {
			return fmt.Errorf("installed, but cannot start %s: %s", unit, tail(res.Stderr, 4))
		}
	}
	return nil
}

func (s *Service) doRemove(ctx context.Context, a *App, label string) error {
	pkgs := append([]string{}, a.Packages...)
	if a.Multi {
		pkgs = a.VersionPkgs(label, false)
	}
	args := append([]string{"remove", "-y"}, pkgs...)
	if res, err := s.run.Run(ctx, 15*time.Minute, "apt-get", args...); err != nil {
		return fmt.Errorf("remove failed: %s", tail(res.Stderr, 6))
	}
	return nil
}

func tail(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

// Control starts, stops or restarts the systemd unit of an installed app.
// Stopping the services RPanel itself runs on is refused.
func (s *Service) Control(ctx context.Context, id, label, action string) (*App, error) {
	if action != "start" && action != "stop" && action != "restart" {
		return nil, fmt.Errorf("%w: action", ErrBadInput)
	}
	a, err := s.validate(id, label, true)
	if err != nil {
		return nil, err
	}
	unit := a.Unit
	if a.VersionUnit != nil {
		if label == "" {
			return nil, fmt.Errorf("%w: choose a %s version", ErrBadInput, a.Name)
		}
		unit = a.VersionUnit(label)
	}
	if unit == "" {
		return nil, fmt.Errorf("%w: %s has no service to control", ErrBadInput, a.Name)
	}
	if !s.isInstalled(a, label) {
		return nil, fmt.Errorf("%w: %s is not installed", ErrBadState, a.Name)
	}
	if a.Required && action != "start" {
		return nil, fmt.Errorf("%w: %s is required by RPanel and cannot be %s from here", ErrBadState, a.Name, map[string]string{"stop": "stopped", "restart": "restarted"}[action])
	}
	s.mu.Lock()
	j, busy := s.jobs[a.ID]
	s.mu.Unlock()
	if busy && (j.state == StateInstalling || j.state == StateRemoving) {
		return nil, fmt.Errorf("%w: %s is busy", ErrBadState, a.Name)
	}
	if res, err := s.run.Run(ctx, time.Minute, "systemctl", action, unit); err != nil {
		return nil, fmt.Errorf("cannot %s %s: %s", action, unit, tail(res.Stderr, 4))
	}
	return a, nil
}
