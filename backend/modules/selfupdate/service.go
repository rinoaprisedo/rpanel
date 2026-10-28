// Package selfupdate tells the panel when a newer RPanel release exists (with its GitHub release notes)
// and runs the official update script, detached from the panel process so it survives the restart.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"rpanel/backend/system"
)

var (
	ErrBadInput = errors.New("invalid input")
	ErrBusy     = errors.New("an update is already running")
	ErrNoScript = errors.New("the update script is not installed (/usr/local/bin/rpanel-update); update from the terminal once with the install command")

	versionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	ansiRe    = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
)

const unitName = "rpanel-self-update"

type Settings struct {
	Repo      string // owner/name
	APIBase   string // https://api.github.com
	Current   string // running version
	LogPath   string
	UpdateBin string // /usr/local/bin/rpanel-update
}

type Release struct {
	Tag         string `json:"tag"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"url"`
}

type Info struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest"`
	Available   bool      `json:"available"`
	Name        string    `json:"name"`
	PublishedAt string    `json:"published_at"`
	URL         string    `json:"url"`
	Releases    []Release `json:"releases"` // every release newer than the running one, newest first
	History     []Release `json:"history"`  // the running release and older ones, newest first
	CanUpdate   bool      `json:"can_update"`
	CheckedAt   int64     `json:"checked_at"`
	Job         *Job      `json:"job"`
}

// Job is the state of the detached update process.
type Job struct {
	State string   `json:"state"` // idle | running | done | failed
	Log   []string `json:"log"`
}

type Service struct {
	run    system.Runner
	cfg    Settings
	client *http.Client
	ttl    time.Duration

	mu      sync.Mutex
	cache   *Info
	fetched time.Time
}

func NewService(run system.Runner, cfg Settings) *Service {
	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.github.com"
	}
	if cfg.UpdateBin == "" {
		cfg.UpdateBin = "/usr/local/bin/rpanel-update"
	}
	return &Service{run: run, cfg: cfg, client: &http.Client{Timeout: 12 * time.Second}, ttl: 30 * time.Minute}
}

func versionParts(v string) [3]int {
	var out [3]int
	for i, s := range strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".", 3) {
		n, _ := strconv.Atoi(strings.SplitN(s, "-", 2)[0])
		out[i] = n
	}
	return out
}

func newer(a, b string) bool {
	x, y := versionParts(a), versionParts(b)
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

type ghRelease struct {
	Tag         string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"html_url"`
}

func (s *Service) fetch(ctx context.Context) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases?per_page=30", s.cfg.APIBase, s.cfg.Repo), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "rpanel/"+s.cfg.Current)
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %d", res.StatusCode)
	}
	var list []ghRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&list); err != nil {
		return nil, err
	}
	info := &Info{Current: s.cfg.Current, Releases: []Release{}, History: []Release{}, CheckedAt: time.Now().Unix()}
	for _, r := range list {
		if r.Draft || r.Prerelease || !versionRe.MatchString(strings.TrimPrefix(r.Tag, "v")) {
			continue
		}
		body := r.Body
		if len(body) > 20000 {
			body = body[:20000] + "\n\n…"
		}
		rel := Release{Tag: strings.TrimPrefix(r.Tag, "v"), Name: r.Name, Body: body, PublishedAt: r.PublishedAt, URL: r.URL}
		if newer(r.Tag, s.cfg.Current) {
			info.Releases = append(info.Releases, rel)
		} else {
			info.History = append(info.History, rel)
		}
	}
	byNewest := func(l []Release) {
		sort.SliceStable(l, func(i, j int) bool { return newer(l[i].Tag, l[j].Tag) })
	}
	byNewest(info.Releases)
	byNewest(info.History)
	if len(info.Releases) > 0 {
		top := info.Releases[0]
		info.Available, info.Latest, info.Name, info.PublishedAt, info.URL = true, top.Tag, top.Name, top.PublishedAt, top.URL
	} else {
		info.Latest = s.cfg.Current
	}
	return info, nil
}

// Info returns the (cached) release information; force bypasses the cache.
func (s *Service) Info(ctx context.Context, force bool) (*Info, error) {
	s.mu.Lock()
	if s.cache != nil && !force && time.Since(s.fetched) < s.ttl {
		c := *s.cache
		s.mu.Unlock()
		return &c, nil
	}
	s.mu.Unlock()
	info, err := s.fetch(ctx)
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cache != nil { // keep showing the last known answer when GitHub is unreachable
			c := *s.cache
			return &c, nil
		}
		return nil, err
	}
	s.mu.Lock()
	s.cache, s.fetched = info, time.Now()
	s.mu.Unlock()
	c := *info
	return &c, nil
}

func (s *Service) unitActive(ctx context.Context) bool {
	res, _ := s.run.Run(ctx, 10*time.Second, "systemctl", "is-active", unitName)
	st := strings.TrimSpace(res.Stdout)
	return st == "active" || st == "activating"
}

func (s *Service) readLog() []string {
	b, err := os.ReadFile(s.cfg.LogPath)
	if err != nil {
		return []string{}
	}
	if len(b) > 64<<10 {
		b = b[len(b)-64<<10:]
	}
	var out []string
	for _, l := range strings.Split(ansiRe.ReplaceAllString(string(b), ""), "\n") {
		if l = strings.TrimRight(l, " \r\t"); l != "" {
			out = append(out, l)
		}
	}
	if len(out) > 200 {
		out = out[len(out)-200:]
	}
	return out
}

// jobState reads the detached update: running while its systemd unit is active, otherwise judged by the log.
func (s *Service) jobState(ctx context.Context) *Job {
	log := s.readLog()
	j := &Job{State: "idle", Log: log}
	if s.unitActive(ctx) {
		j.State = "running"
		return j
	}
	joined := strings.Join(log, "\n")
	switch {
	case strings.Contains(joined, "Upgrade finished") || strings.Contains(joined, "is already installed"):
		j.State = "done"
	case strings.Contains(joined, "error:") || strings.Contains(joined, "rolled back") || strings.Contains(joined, "failed"):
		j.State = "failed"
	}
	return j
}

// Status is Info plus the update job.
func (s *Service) Status(ctx context.Context, force bool) (*Info, error) {
	info, err := s.Info(ctx, force)
	if err != nil {
		return nil, err
	}
	info.CanUpdate = true
	if _, e := os.Stat(s.cfg.UpdateBin); e != nil {
		info.CanUpdate = false
	}
	info.Job = s.jobState(ctx)
	return info, nil
}

// JobOnly returns just the job (cheap; polled during the update).
func (s *Service) JobOnly(ctx context.Context) *Job { return s.jobState(ctx) }

// Start launches the update script as its own systemd unit, so stopping the panel does not stop the update.
func (s *Service) Start(ctx context.Context) error {
	info, err := s.Info(ctx, true)
	if err != nil {
		return fmt.Errorf("cannot reach GitHub: %w", err)
	}
	if !info.Available || !versionRe.MatchString(info.Latest) {
		return fmt.Errorf("%w: RPanel is already up to date", ErrBadInput)
	}
	if _, err := os.Stat(s.cfg.UpdateBin); err != nil {
		return ErrNoScript
	}
	if s.unitActive(ctx) {
		return ErrBusy
	}
	if strings.ContainsAny(s.cfg.LogPath, " \t\n\"'$;|&<>") {
		return fmt.Errorf("%w: log path", ErrBadInput)
	}
	args := []string{"--unit=" + unitName, "--collect", "--quiet",
		"-p", "StandardOutput=file:" + s.cfg.LogPath, "-p", "StandardError=file:" + s.cfg.LogPath,
		s.cfg.UpdateBin, info.Latest}
	if res, err := s.run.Run(ctx, 20*time.Second, "systemd-run", args...); err != nil {
		return fmt.Errorf("cannot start the update: %s", strings.TrimSpace(res.Stderr+" "+err.Error()))
	}
	return nil
}
