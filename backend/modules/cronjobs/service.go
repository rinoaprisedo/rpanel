// Package cronjobs manages scheduled jobs. Jobs live in the database; for each
// enabled job a file in the cron.d directory makes the system cron daemon call
// "<panel binary> cron-run <uuid>", which executes the job and records the run.
package cronjobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"rpanel/backend/system"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const (
	maxOutput  = 64 << 10
	keepRuns   = 30
	maxCommand = 64 << 10
	minTimeout = 1
	maxTimeout = 86400
	defTimeout = 3600
)

var (
	ErrNotFound = errors.New("cron job not found")
	ErrBadInput = errors.New("invalid input")

	fieldRe    = regexp.MustCompile(`^[0-9*/,\-]+$`)
	safeArgRe  = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)
	cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
)

// NormalizeSchedule validates a 5-field cron expression and returns it with
// single spaces. Only digits and * / , - are accepted (no names, no @aliases),
// so the value can never carry anything but schedule syntax into a cron file.
func NormalizeSchedule(s string) (string, error) {
	f := strings.Fields(s)
	if len(f) != 5 {
		return "", fmt.Errorf("%w: schedule needs 5 fields (minute hour day month weekday)", ErrBadInput)
	}
	for _, x := range f {
		if !fieldRe.MatchString(x) {
			return "", fmt.Errorf("%w: schedule field %q", ErrBadInput, x)
		}
	}
	n := strings.Join(f, " ")
	if _, err := cronParser.Parse(n); err != nil {
		return "", fmt.Errorf("%w: schedule: %v", ErrBadInput, err)
	}
	return n, nil
}

type Service struct {
	db      *gorm.DB
	cronDir string
	dataDir string
	exe     string // panel binary invoked by cron
	wd      string // working directory holding .env
	client  *http.Client
}

func NewService(db *gorm.DB, cronDir, dataDir string) *Service {
	exe, _ := os.Executable()
	wd, _ := os.Getwd()
	return &Service{db: db, cronDir: cronDir, dataDir: dataDir, exe: exe, wd: wd, client: &http.Client{}}
}

func (s *Service) scriptPath(j *Job) string {
	return filepath.Join(s.dataDir, "scripts", j.UUID.String()+".sh")
}
func (s *Service) cronFile(j *Job) string { return filepath.Join(s.cronDir, "rpanel-"+j.UUID.String()) }

type Input struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Schedule string  `json:"schedule"`
	Command  string  `json:"command"`
	Timeout  int     `json:"timeout"`
	Enabled  *bool   `json:"enabled"`
	ActorID  *uint64 `json:"-"`
}

func validate(in *Input, j *Job) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("%w: name", ErrBadInput)
	}
	sched, err := NormalizeSchedule(in.Schedule)
	if err != nil {
		return err
	}
	cmd := strings.ReplaceAll(in.Command, "\r\n", "\n")
	if strings.TrimSpace(cmd) == "" || len(cmd) > maxCommand || strings.ContainsRune(cmd, 0) {
		return fmt.Errorf("%w: command", ErrBadInput)
	}
	switch in.Type {
	case TypeShell:
	case TypeURL:
		cmd = strings.TrimSpace(cmd)
		if !(strings.HasPrefix(cmd, "http://") || strings.HasPrefix(cmd, "https://")) || strings.ContainsAny(cmd, " \r\n\t") {
			return fmt.Errorf("%w: URL must start with http:// or https://", ErrBadInput)
		}
	default:
		return fmt.Errorf("%w: type", ErrBadInput)
	}
	t := in.Timeout
	if t == 0 {
		t = defTimeout
	}
	if t < minTimeout || t > maxTimeout {
		return fmt.Errorf("%w: timeout must be %d-%d seconds", ErrBadInput, minTimeout, maxTimeout)
	}
	j.Name, j.Type, j.Schedule, j.Command, j.Timeout = name, in.Type, sched, cmd, t
	return nil
}

func (s *Service) List() ([]Job, error) {
	var out []Job
	return out, s.db.Order("name").Find(&out).Error
}

func (s *Service) Get(id string) (*Job, error) {
	var j Job
	if err := s.db.Where("uuid = ?", id).First(&j).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &j, nil
}

func (s *Service) Create(in Input) (*Job, error) {
	j := &Job{Enabled: true}
	if in.Enabled != nil {
		j.Enabled = *in.Enabled
	}
	if err := validate(&in, j); err != nil {
		return nil, err
	}
	j.CreatedBy, j.UpdatedBy = in.ActorID, in.ActorID
	if err := s.db.Create(j).Error; err != nil {
		return nil, err
	}
	if err := s.Sync(j); err != nil {
		s.db.Unscoped().Delete(j)
		return nil, err
	}
	return j, nil
}

func (s *Service) Update(id string, in Input) (*Job, error) {
	j, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	old := *j
	if in.Enabled != nil {
		j.Enabled = *in.Enabled
	}
	if err := validate(&in, j); err != nil {
		return nil, err
	}
	j.UpdatedBy = in.ActorID
	if err := s.Sync(j); err != nil {
		_ = s.Sync(&old)
		return nil, err
	}
	return j, s.db.Save(j).Error
}

func (s *Service) SetEnabled(id string, enabled bool, actor *uint64) (*Job, error) {
	j, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	old := *j
	j.Enabled, j.UpdatedBy = enabled, actor
	if err := s.Sync(j); err != nil {
		_ = s.Sync(&old)
		return nil, err
	}
	return j, s.db.Save(j).Error
}

func (s *Service) Delete(id string) (*Job, error) {
	j, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	j.Enabled = false
	if err := s.Sync(j); err != nil {
		return nil, err
	}
	_ = os.Remove(s.scriptPath(j))
	s.db.Where("job_id = ?", j.ID).Delete(&Run{})
	return j, s.db.Unscoped().Delete(j).Error
}

// Sync makes the filesystem match the job: script + cron.d file when enabled,
// nothing when disabled.
func (s *Service) Sync(j *Job) error {
	file := s.cronFile(j)
	if !j.Enabled {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if !safeArgRe.MatchString(s.exe) || !safeArgRe.MatchString(s.wd) {
		return fmt.Errorf("cannot schedule: panel path %q / %q contains unsupported characters", s.exe, s.wd)
	}
	if j.Type == TypeShell {
		if err := os.MkdirAll(filepath.Dir(s.scriptPath(j)), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(s.scriptPath(j), []byte(j.Command+"\n"), 0o700); err != nil {
			return err
		}
	}
	line := fmt.Sprintf("%s root cd %s && %s cron-run %s >/dev/null 2>&1\n", j.Schedule, s.wd, s.exe, j.UUID)
	content := "# Managed by RPanel — edit the job in the panel instead.\n" +
		"SHELL=/bin/bash\nPATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin\n" + line
	if err := os.MkdirAll(s.cronDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.cronDir, ".rpanel-tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // cron ignores group/world-writable files
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// SyncAll rewrites every enabled job's files (e.g. after the binary moved).
func (s *Service) SyncAll() error {
	jobs, err := s.List()
	if err != nil {
		return err
	}
	var errs []error
	for i := range jobs {
		if err := s.Sync(&jobs[i]); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", jobs[i].Name, err))
		}
	}
	return errors.Join(errs...)
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-n:]
}

// Execute runs a job now and records the run. trigger is "schedule" or "manual".
// A job that is still running (within its timeout) is not started twice.
func (s *Service) Execute(ctx context.Context, j *Job, trigger string) *Run {
	now := time.Now()
	var active int64
	s.db.Model(&Run{}).Where("job_id = ? AND status = ? AND started_at > ?", j.ID, RunRunning, now.Add(-time.Duration(j.Timeout)*time.Second-time.Minute)).Count(&active)
	run := &Run{JobID: j.ID, Trigger: trigger, StartedAt: now}
	if active > 0 {
		run.Status, run.ExitCode, run.FinishedAt = RunSkipped, -1, &now
		run.Output = "skipped: previous run is still in progress"
		s.db.Create(run)
		return run
	}
	run.Status = RunRunning
	s.db.Create(run)

	timeout := time.Duration(j.Timeout) * time.Second
	var output string
	var code int
	var err error
	switch j.Type {
	case TypeURL:
		output, code, err = s.fetch(ctx, j.Command, timeout)
	default:
		var res system.Result
		res, err = system.RunScript(ctx, timeout, s.scriptPath(j))
		output, code = res.Stdout+res.Stderr, res.ExitCode
		if err != nil && code == -1 || err != nil && strings.Contains(err.Error(), "timed out") {
			output += "\n[" + err.Error() + "]"
		}
	}
	end := time.Now()
	run.FinishedAt, run.ExitCode, run.Output = &end, code, tailString(output, maxOutput)
	if err == nil {
		run.Status = RunSuccess
	} else {
		run.Status = RunFailed
	}
	s.db.Save(run)
	s.db.Model(&Job{}).Where("id = ?", j.ID).Updates(map[string]any{"last_run_at": now, "last_status": run.Status})
	s.prune(j.ID)
	return run
}

// fetch performs the HTTP GET of a URL job. Success means a 2xx status; the
// exit code field carries the HTTP status.
func (s *Service) fetch(ctx context.Context, url string, timeout time.Duration) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err.Error(), -1, err
	}
	req.Header.Set("User-Agent", "RPanel-Cron/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return err.Error(), -1, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxOutput))
	out := fmt.Sprintf("HTTP %s\n\n%s", resp.Status, body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return out, resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return out, resp.StatusCode, nil
}

func (s *Service) prune(jobID uint64) {
	var ids []uint64
	s.db.Model(&Run{}).Where("job_id = ?", jobID).Order("id DESC").Offset(keepRuns).Pluck("id", &ids)
	if len(ids) > 0 {
		s.db.Where("id IN ?", ids).Delete(&Run{})
	}
}

func (s *Service) Runs(id string, limit int) ([]Run, error) {
	j, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > keepRuns {
		limit = 20
	}
	var out []Run
	return out, s.db.Where("job_id = ?", j.ID).Order("id DESC").Limit(limit).Find(&out).Error
}

// RunByUUID is the entry point of the "cron-run" CLI subcommand.
func (s *Service) RunByUUID(ctx context.Context, id string) (*Run, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: job id", ErrBadInput)
	}
	j, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if !j.Enabled {
		return nil, errors.New("job is disabled")
	}
	return s.Execute(ctx, j, "schedule"), nil
}
