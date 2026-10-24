package dashboard

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"rpanel/backend/modules/appstore"
	"rpanel/backend/modules/website"
	"rpanel/backend/system"

	"gorm.io/gorm"
)

const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
	SevOK       = "ok"
)

// Insight is one finding the operator can act on.
type Insight struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Link     string `json:"link,omitempty"` // panel page where it can be fixed
}

type SiteInfo struct {
	Domain     string `json:"domain"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	SSLEnabled bool   `json:"ssl_enabled"`
	ForceHTTPS bool   `json:"force_https"`
	DaysLeft   *int   `json:"days_left"` // certificate validity; nil when no certificate is on disk
}

type ServiceInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Required bool   `json:"required"`
	Version  string `json:"version"`
}

type Updates struct {
	Known          bool  `json:"known"`
	Total          int   `json:"total"`
	Security       int   `json:"security"`
	RebootRequired bool  `json:"reboot_required"`
	CheckedAt      int64 `json:"checked_at"`
}

type Firewall struct {
	Known  bool `json:"known"`
	Active bool `json:"active"`
}

// InsightsResponse is everything the dashboard's "attention" and "health" panels need.
type InsightsResponse struct {
	Insights []Insight     `json:"insights"`
	Sites    []SiteInfo    `json:"sites"`
	Services []ServiceInfo `json:"services"`
	Updates  Updates       `json:"updates"`
	Firewall Firewall      `json:"firewall"`
	Health   Health        `json:"health"`
	Release  Release       `json:"release"`
}

// insightInput is the snapshot buildInsights reasons about (kept free of I/O so it is testable).
type insightInput struct {
	Overview  Overview
	Recent    []Point // about the last five minutes
	TopMem    ProcessInfo
	Sites     []SiteInfo
	Services  []ServiceInfo
	Updates   Updates
	Firewall  Firewall
	Traffic1h *Traffic
	Release   Release
}

func human(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func avg(ps []Point, f func(Point) float64) float64 {
	if len(ps) == 0 {
		return 0
	}
	var s float64
	for _, p := range ps {
		s += f(p)
	}
	return s / float64(len(ps))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func buildInsights(in insightInput) []Insight {
	var out []Insight
	add := func(id, sev, title, detail, link string) {
		out = append(out, Insight{ID: id, Severity: sev, Title: title, Detail: detail, Link: link})
	}
	o := in.Overview

	for _, d := range o.Disks {
		free := d.Total - d.Used
		switch {
		case d.Percent >= 90:
			add("disk:"+d.Mount, SevCritical, fmt.Sprintf("Disk %s is %.0f%% full", d.Mount, d.Percent),
				fmt.Sprintf("Only %s left. Sites, databases and logs stop working when it reaches 100%%. Clear old backups and logs.", human(free)), "/file-manager")
		case d.Percent >= 80:
			add("disk:"+d.Mount, SevWarning, fmt.Sprintf("Disk %s is %.0f%% full", d.Mount, d.Percent),
				fmt.Sprintf("%s left. Plan some cleanup or more space before it gets critical.", human(free)), "/file-manager")
		}
	}

	if o.MemPct >= 90 {
		detail := "Memory is nearly exhausted; the kernel may start killing processes."
		if in.TopMem.Name != "" {
			detail = fmt.Sprintf("Biggest consumer: %s (%.1f%% of memory). The kernel may start killing processes.", in.TopMem.Name, in.TopMem.MemPct)
		}
		add("mem", SevWarning, fmt.Sprintf("Memory is %.0f%% used", o.MemPct), detail, "")
	}
	if o.SwapTotal > 0 && float64(o.SwapUsed)/float64(o.SwapTotal) >= 0.5 {
		add("swap", SevWarning, "Swap is more than half used",
			fmt.Sprintf("%s of %s swapped out. The server is short on RAM and will feel slow.", human(o.SwapUsed), human(o.SwapTotal)), "")
	}

	cores := float64(o.CPUCores)
	if cores < 1 {
		cores = 1
	}
	switch {
	case o.Load5 > cores*3:
		add("load", SevCritical, fmt.Sprintf("Load is %.1f on %s", o.Load5, plural(o.CPUCores, "core", "cores")),
			"Work is queuing far beyond what the CPU can run. Check the busiest processes.", "")
	case o.Load5 > cores*1.5:
		add("load", SevWarning, fmt.Sprintf("Load is %.1f on %s", o.Load5, plural(o.CPUCores, "core", "cores")),
			"Processes are waiting for CPU time. Check the busiest processes below.", "")
	}
	if len(in.Recent) >= 20 {
		if c := avg(in.Recent, func(p Point) float64 { return p.CPU }); c >= 85 {
			add("cpu", SevWarning, fmt.Sprintf("CPU has averaged %.0f%% for the last minutes", c), "A sustained spike usually means a runaway process or a traffic burst.", "")
		}
	}

	for _, s := range in.Services {
		if s.Running {
			continue
		}
		if s.Required {
			add("svc:"+s.ID, SevCritical, s.Name+" is not running", "RPanel depends on this service.", "/app-store")
		} else {
			add("svc:"+s.ID, SevWarning, s.Name+" is installed but not running", "Anything that relies on it is currently down.", "/app-store")
		}
	}

	var noHTTPS []string
	for _, s := range in.Sites {
		if s.Status != "running" {
			continue
		}
		if s.SSLEnabled && s.DaysLeft != nil {
			switch d := *s.DaysLeft; {
			case d < 0:
				add("ssl:"+s.Domain, SevCritical, s.Domain+": SSL certificate has expired", "Visitors see a security warning. Renew it from Manage → SSL.", "/websites")
			case d < 7:
				add("ssl:"+s.Domain, SevCritical, fmt.Sprintf("%s: certificate expires in %s", s.Domain, plural(d, "day", "days")), "Auto-renewal should have run by now. Renew it from Manage → SSL.", "/websites")
			case d < 14:
				add("ssl:"+s.Domain, SevWarning, fmt.Sprintf("%s: certificate expires in %d days", s.Domain, d), "Auto-renewal normally handles this; check it if the date keeps getting closer.", "/websites")
			}
		}
		if !s.SSLEnabled {
			noHTTPS = append(noHTTPS, s.Domain)
		}
	}
	if len(noHTTPS) > 0 {
		sort.Strings(noHTTPS)
		shown := noHTTPS
		if len(shown) > 3 {
			shown = append(append([]string{}, shown[:3]...), fmt.Sprintf("+%d more", len(noHTTPS)-3))
		}
		add("https", SevInfo, fmt.Sprintf("%s served without HTTPS", plural(len(noHTTPS), "website is", "websites are")),
			strings.Join(shown, ", ")+". Browsers flag plain HTTP as insecure.", "/websites")
	}

	if in.Firewall.Known && !in.Firewall.Active {
		add("firewall", SevWarning, "Firewall is off", "Every open port on this server is reachable from the internet. Enable UFW from the Firewall page.", "/firewall")
	}
	if in.Updates.Security > 0 {
		add("updates", SevWarning, plural(in.Updates.Security, "security update", "security updates")+" pending",
			fmt.Sprintf("%s available in total. Apply them with apt upgrade.", plural(in.Updates.Total, "package update is", "package updates are")), "/terminal")
	} else if in.Updates.Total > 0 {
		add("updates", SevInfo, plural(in.Updates.Total, "package update", "package updates")+" available", "No security fixes among them.", "/terminal")
	}
	if in.Release.Available {
		add("release", SevInfo, "RPanel "+in.Release.Latest+" is available",
			"You are running "+in.Release.Current+". On the server run: sudo rpanel-update  (release notes: "+in.Release.URL+")", "")
	}
	if in.Updates.RebootRequired {
		add("reboot", SevWarning, "A reboot is required", "Installed updates (usually the kernel) only take effect after a restart.", "")
	}

	if t := in.Traffic1h; t != nil {
		for _, s := range t.Sites {
			if s.Requests < 20 {
				continue
			}
			rate := float64(s.ServerErr) / float64(s.Requests) * 100
			switch {
			case rate >= 10:
				add("5xx:"+s.Domain, SevCritical, fmt.Sprintf("%s: %.1f%% server errors", s.Domain, rate),
					fmt.Sprintf("%d of %d requests failed with 5xx in the last hour. Check the site's error log.", s.ServerErr, s.Requests), "/websites")
			case rate >= 2:
				add("5xx:"+s.Domain, SevWarning, fmt.Sprintf("%s: %.1f%% server errors", s.Domain, rate),
					fmt.Sprintf("%d of %d requests failed with 5xx in the last hour. Check the site's error log.", s.ServerErr, s.Requests), "/websites")
			}
		}
		if t.Totals.Requests >= 200 {
			if share := float64(t.Totals.Bots) / float64(t.Totals.Requests) * 100; share >= 60 {
				add("bots", SevInfo, fmt.Sprintf("%.0f%% of recent traffic is bots or scripts", share),
					"Mostly crawlers and automated clients rather than visitors. Look at the top clients below.", "")
			}
		}
	}

	rank := map[string]int{SevCritical: 0, SevWarning: 1, SevInfo: 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	if len(out) == 0 {
		out = append(out, Insight{ID: "ok", Severity: SevOK, Title: "Everything looks healthy", Detail: "Resources, services, certificates and updates are all within normal limits."})
	}
	return out
}

// ---- probes: slow or privileged checks, cached ----

type Probes struct {
	run      system.Runner
	apps     *appstore.Service
	db       *gorm.DB
	certDir  string
	analyzer *Analyzer

	mu       sync.Mutex
	fw       Firewall
	fwAt     time.Time
	svc      []ServiceInfo
	svcAt    time.Time
	top      ProcessInfo
	updates  Updates
	release  Release
	rebootAt string

	releaseBase string // GitHub base URL; replaced in tests
}

func NewProbes(run system.Runner, db *gorm.DB, certDir string, an *Analyzer) *Probes {
	return &Probes{run: run, apps: appstore.NewService(run), db: db, certDir: certDir, analyzer: an, rebootAt: "/var/run/reboot-required", releaseBase: "https://github.com"}
}

// StartUpdateChecks looks for pending package updates now-ish and then hourly.
func (p *Probes) StartUpdateChecks(ctx context.Context) {
	go func() {
		t := time.NewTimer(20 * time.Second)
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
				p.refreshUpdates(ctx)
				t.Reset(time.Hour)
			}
		}
	}()
}

func (p *Probes) refreshUpdates(ctx context.Context) {
	res, err := p.run.Run(ctx, 2*time.Minute, "apt-get", "-s", "upgrade")
	if err != nil {
		return
	}
	total, security := parseUpgrades(res.Stdout)
	p.mu.Lock()
	p.updates = Updates{Known: true, Total: total, Security: security, CheckedAt: time.Now().Unix()}
	p.mu.Unlock()
}

// parseUpgrades counts the packages a simulated upgrade would install ("Inst ..." lines).
func parseUpgrades(out string) (total, security int) {
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "Inst ") {
			continue
		}
		total++
		if strings.Contains(l, "-security") {
			security++
		}
	}
	return total, security
}

func (p *Probes) firewall(ctx context.Context) Firewall {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.fwAt) < time.Minute {
		return p.fw
	}
	fw := Firewall{}
	if res, err := p.run.Run(ctx, 10*time.Second, "ufw", "status"); err == nil {
		fw = Firewall{Known: true, Active: strings.Contains(res.Stdout, "Status: active")}
	}
	p.fw, p.fwAt = fw, time.Now()
	return fw
}

func (p *Probes) services(ctx context.Context) ([]ServiceInfo, ProcessInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.svcAt) < 20*time.Second {
		return p.svc, p.top
	}
	svc := []ServiceInfo{}
	for _, it := range p.apps.List(ctx) {
		if it.Installed && it.HasService {
			svc = append(svc, ServiceInfo{ID: it.ID, Name: it.Name, Running: it.Running, Required: it.Required, Version: it.Version})
		}
	}
	p.svc, p.svcAt = svc, time.Now()
	if top := TopProcesses(ctx, "mem", 1); len(top) > 0 {
		p.top = top[0]
	}
	return p.svc, p.top
}

func (p *Probes) sites() []SiteInfo {
	var ws []website.Website
	if p.db == nil || p.db.Order("domain").Find(&ws).Error != nil {
		return []SiteInfo{}
	}
	out := make([]SiteInfo, 0, len(ws))
	for _, w := range ws {
		s := SiteInfo{Domain: w.Domain, Type: w.Type, Status: w.Status, SSLEnabled: w.SSLEnabled, ForceHTTPS: w.ForceHTTPS}
		if c := website.ReadCert(p.certDir, w.Domain); c != nil {
			d := c.DaysLeft
			s.DaysLeft = &d
		}
		out = append(out, s)
	}
	return out
}

// Domains lists the managed websites (the only logs the traffic analysis may read).
func (p *Probes) Domains() []string {
	out := []string{}
	for _, s := range p.sites() {
		out = append(out, s.Domain)
	}
	return out
}

// Build gathers the live state and runs the rules.
func (p *Probes) Build(ctx context.Context, c *Collector) InsightsResponse {
	svc, top := p.services(ctx)
	sites := p.sites()
	fw := p.firewall(ctx)
	p.mu.Lock()
	up := p.updates
	rel := p.release
	p.mu.Unlock()
	if _, err := os.Stat(p.rebootAt); err == nil {
		up.RebootRequired = true
	}
	var tr *Traffic
	if p.analyzer != nil {
		tr, _ = p.analyzer.Traffic("1h", "")
	}
	in := insightInput{Overview: c.Overview(), Recent: c.History(), TopMem: top, Sites: sites, Services: svc, Updates: up, Firewall: fw, Traffic1h: tr, Release: rel}
	return InsightsResponse{
		Insights: buildInsights(in), Health: buildHealth(in),
		Sites: sites, Services: svc, Updates: up, Firewall: fw, Release: rel,
	}
}
