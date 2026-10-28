package website

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"rpanel/backend/modules/panelssl"
)

// Step states of an SSL job.
const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepWarn    = "warn"
	StepFailed  = "failed"
)

// SSLStep is one stage of obtaining a certificate, as shown in the progress dialog.
type SSLStep struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// SSLJob tracks a background certificate request so the UI can show real progress.
type SSLJob struct {
	mu       sync.Mutex
	UUID     string
	Domain   string
	state    string // running | done | failed
	percent  int
	steps    []SSLStep
	log      []string
	errMsg   string
	started  time.Time
	finished time.Time
}

// SSLProgress is the JSON snapshot returned to the client.
type SSLProgress struct {
	Domain  string    `json:"domain"`
	State   string    `json:"state"`
	Percent int       `json:"percent"`
	Steps   []SSLStep `json:"steps"`
	Log     []string  `json:"log"`
	Error   string    `json:"error,omitempty"`
	Elapsed int       `json:"elapsed_seconds"`
}

func newSSLJob(w *Website) *SSLJob {
	return &SSLJob{
		UUID: w.UUID.String(), Domain: w.Domain, state: "running", started: time.Now(),
		steps: []SSLStep{
			{Key: "dns", Label: "Checking DNS records", Status: StepPending},
			{Key: "prepare", Label: "Preparing domain validation", Status: StepPending},
			{Key: "certificate", Label: "Requesting the certificate from Let's Encrypt", Status: StepPending},
			{Key: "enable", Label: "Enabling HTTPS", Status: StepPending},
		},
	}
}

func (j *SSLJob) snapshot() *SSLProgress {
	j.mu.Lock()
	defer j.mu.Unlock()
	end := j.finished
	if end.IsZero() {
		end = time.Now()
	}
	return &SSLProgress{
		Domain: j.Domain, State: j.state, Percent: j.percent, Steps: append([]SSLStep(nil), j.steps...),
		Log: append([]string(nil), j.log...), Error: j.errMsg, Elapsed: int(end.Sub(j.started).Seconds()),
	}
}

func (j *SSLJob) logf(format string, a ...any) {
	j.mu.Lock()
	j.log = append(j.log, fmt.Sprintf(format, a...))
	if len(j.log) > 200 {
		j.log = j.log[len(j.log)-200:]
	}
	j.mu.Unlock()
}

func (j *SSLJob) step(key, status, detail string, percent int) {
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

func (j *SSLJob) bump(to int) {
	j.mu.Lock()
	if to > j.percent {
		j.percent = to
	}
	j.mu.Unlock()
}

func (j *SSLJob) finish(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.finished = time.Now()
	if err != nil {
		j.state, j.errMsg = "failed", err.Error()
		for i := range j.steps {
			if j.steps[i].Status == StepRunning {
				j.steps[i].Status = StepFailed
			}
		}
		return
	}
	j.state, j.percent = "done", 100
}

const jobKeep = 15 * time.Minute

// StartSSL begins issuing a Let's Encrypt certificate in the background and returns immediately. Starting
// again while a job for the same site is running returns the running job. With forceHTTPS the site also
// redirects HTTP to HTTPS once the certificate is installed.
func (s *Service) StartSSL(uuid, email string, forceHTTPS bool, actor *uint64) (*SSLProgress, error) {
	w, err := s.Get(uuid)
	if err != nil {
		return nil, err
	}
	if w.Status != StatusRunning {
		return nil, fmt.Errorf("%w: start the site first", ErrBadInput)
	}
	reg, err := s.registration(email)
	if err != nil {
		return nil, err
	}
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if s.jobs == nil {
		s.jobs = map[string]*SSLJob{}
	}
	for k, j := range s.jobs {
		if p := j.snapshot(); p.State != "running" && time.Since(j.finished) > jobKeep {
			delete(s.jobs, k)
		}
	}
	if j, ok := s.jobs[uuid]; ok && j.snapshot().State == "running" {
		return j.snapshot(), nil
	}
	j := newSSLJob(w)
	s.jobs[uuid] = j
	go s.runSSL(j, w, reg, forceHTTPS, actor)
	return j.snapshot(), nil
}

// SSLProgress returns the latest job for a site (nil when none was started).
func (s *Service) SSLProgress(uuid string) *SSLProgress {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if j, ok := s.jobs[uuid]; ok {
		return j.snapshot()
	}
	return nil
}

func (s *Service) runSSL(j *SSLJob, w *Website, reg []string, forceHTTPS bool, actor *uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	j.finish(s.issueWithProgress(ctx, j, w, reg, forceHTTPS, actor))
}

func (s *Service) issueWithProgress(ctx context.Context, j *SSLJob, w *Website, reg []string, forceHTTPS bool, actor *uint64) error {
	// 1. DNS (advisory: NAT and CDNs make the local-address comparison unreliable)
	j.step("dns", StepRunning, "", 3)
	j.logf("Checking DNS records of %s", w.Domain)
	lookup := s.lookup
	if lookup == nil {
		lookup = defaultLookup
	}
	ips, hint, err := panelssl.CheckDNS(lookup, w.Domain)
	var dnsHint string
	switch {
	case err != nil:
		dnsHint = fmt.Sprintf("%s has no DNS record yet. Create an A record pointing at this server; Let's Encrypt will fail until it exists.", w.Domain)
		j.logf("WARNING: %s", dnsHint)
		j.step("dns", StepWarn, "No DNS record found", 10)
	case hint != "":
		dnsHint = hint
		j.logf("WARNING: %s", hint)
		j.step("dns", StepWarn, "Does not point at this server", 10)
	default:
		j.logf("%s -> %s", w.Domain, strings.Join(ips, ", "))
		j.step("dns", StepDone, strings.Join(ips, ", "), 10)
	}

	// 2. make sure nginx answers the challenge for this domain
	j.step("prepare", StepRunning, "", 15)
	if _, e := exec.LookPath("certbot"); e != nil && s.certbotOptional() {
		j.step("prepare", StepFailed, "Certbot is not installed", 15)
		return errors.New("Certbot is not installed. Install it from the App Store (Certbot) and try again.")
	}
	if err := os.MkdirAll(s.cfg.AcmeRoot, 0o755); err != nil {
		j.step("prepare", StepFailed, err.Error(), 15)
		return err
	}
	if err := s.ng.Apply(ctx, w, true); err != nil {
		j.step("prepare", StepFailed, "nginx rejected the configuration", 15)
		return err
	}
	j.logf("nginx serves http://%s/.well-known/acme-challenge/", w.Domain)
	j.step("prepare", StepDone, "", 25)

	// 3. certbot (progress creeps towards 85% while it runs; there is no real signal inside certbot)
	j.step("certificate", StepRunning, "This usually takes 10-30 seconds", 30)
	j.logf("$ certbot certonly --webroot -d %s", w.Domain)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(400 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				j.mu.Lock()
				if j.percent < 85 {
					j.percent += max(1, (85-j.percent)/25)
				}
				j.mu.Unlock()
			}
		}
	}()
	res, err := s.run.Run(ctx, 5*time.Minute, "certbot", s.certbotArgs(w.Domain, reg)...)
	close(done)
	for _, l := range strings.Split(strings.TrimSpace(res.Stdout+"\n"+res.Stderr), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			j.logf("%s", l)
		}
	}
	if err != nil {
		msg := "certbot failed: " + lastLines(res.Stdout+"\n"+res.Stderr, 6)
		if dnsHint != "" {
			msg += "\n" + dnsHint
		}
		j.step("certificate", StepFailed, "Let's Encrypt refused the request", 30)
		return errors.New(msg)
	}
	j.step("certificate", StepDone, "", 88)

	// 4. switch the vhost to HTTPS
	j.step("enable", StepRunning, "", 90)
	prev := *w
	w.SSLEnabled, w.UpdatedBy = true, actor
	if forceHTTPS {
		w.ForceHTTPS = true
	}
	if err := s.ng.Apply(ctx, w, true); err != nil {
		j.step("enable", StepFailed, "nginx rejected the HTTPS configuration", 90)
		return err
	}
	if err := s.db.Save(w).Error; err != nil {
		_ = s.ng.Apply(ctx, &prev, true)
		j.step("enable", StepFailed, err.Error(), 90)
		return err
	}
	j.logf("HTTPS is enabled for %s", w.Domain)
	j.step("enable", StepDone, "", 100)
	return nil
}
