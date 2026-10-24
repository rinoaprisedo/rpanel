package dashboard

import (
	"strings"
	"testing"
)

func ids(in []Insight) string {
	var s []string
	for _, i := range in {
		s = append(s, i.Severity+":"+i.ID)
	}
	return strings.Join(s, " ")
}

func find3(in []Insight, id string) *Insight {
	for i := range in {
		if in[i].ID == id {
			return &in[i]
		}
	}
	return nil
}

func ptr(n int) *int { return &n }

func TestHealthyServerReportsAllClear(t *testing.T) {
	got := buildInsights(insightInput{
		Overview: Overview{CPUCores: 2, MemPct: 40, Load5: 0.3, Disks: []DiskInfo{{Mount: "/", Total: 100 << 30, Used: 30 << 30, Percent: 30}}},
		Sites:    []SiteInfo{{Domain: "a.com", Status: "running", SSLEnabled: true, DaysLeft: ptr(60)}},
		Services: []ServiceInfo{{ID: "nginx", Name: "Nginx", Running: true, Required: true}},
		Firewall: Firewall{Known: true, Active: true},
		Updates:  Updates{Known: true},
	})
	if len(got) != 1 || got[0].Severity != SevOK {
		t.Errorf("want a single ok insight, got %s", ids(got))
	}
}

func TestResourceRules(t *testing.T) {
	got := buildInsights(insightInput{
		Overview: Overview{
			CPUCores: 2, MemPct: 95, Load5: 7, SwapTotal: 4 << 30, SwapUsed: 3 << 30,
			Disks: []DiskInfo{{Mount: "/", Total: 100 << 30, Used: 93 << 30, Percent: 93}, {Mount: "/data", Total: 100 << 30, Used: 82 << 30, Percent: 82}},
		},
		TopMem: ProcessInfo{Name: "mysqld", MemPct: 61.5},
	})
	for id, sev := range map[string]string{"disk:/": SevCritical, "disk:/data": SevWarning, "mem": SevWarning, "swap": SevWarning, "load": SevCritical} {
		i := find3(got, id)
		if i == nil || i.Severity != sev {
			t.Errorf("%s: want %s, got %+v (all: %s)", id, sev, i, ids(got))
		}
	}
	if m := find3(got, "mem"); !strings.Contains(m.Detail, "mysqld") {
		t.Errorf("memory insight must name the biggest consumer: %q", m.Detail)
	}
	if d := find3(got, "disk:/"); !strings.Contains(d.Detail, "7.0 GB") {
		t.Errorf("disk insight must say how much is left: %q", d.Detail)
	}
	if got[0].Severity != SevCritical || got[len(got)-1].Severity == SevCritical && len(got) > 3 {
		t.Errorf("critical findings must come first: %s", ids(got))
	}
}

func TestSustainedCPUNeedsHistory(t *testing.T) {
	hot := make([]Point, 30)
	for i := range hot {
		hot[i].CPU = 92
	}
	if find3(buildInsights(insightInput{Overview: Overview{CPUCores: 2}, Recent: hot}), "cpu") == nil {
		t.Error("sustained high CPU must be reported")
	}
	if find3(buildInsights(insightInput{Overview: Overview{CPUCores: 2}, Recent: hot[:5]}), "cpu") != nil {
		t.Error("a few samples are not a trend")
	}
}

func TestSiteAndSecurityRules(t *testing.T) {
	got := buildInsights(insightInput{
		Overview: Overview{CPUCores: 1},
		Sites: []SiteInfo{
			{Domain: "expired.com", Status: "running", SSLEnabled: true, DaysLeft: ptr(-2)},
			{Domain: "soon.com", Status: "running", SSLEnabled: true, DaysLeft: ptr(5)},
			{Domain: "later.com", Status: "running", SSLEnabled: true, DaysLeft: ptr(10)},
			{Domain: "fine.com", Status: "running", SSLEnabled: true, DaysLeft: ptr(80)},
			{Domain: "plain.com", Status: "running"},
			{Domain: "off.com", Status: "stopped"}, // a stopped site is nobody's problem here
		},
		Services: []ServiceInfo{{ID: "nginx", Name: "Nginx", Required: true}, {ID: "mariadb", Name: "MariaDB"}, {ID: "redis", Name: "Redis", Running: true, Required: true}},
		Firewall: Firewall{Known: true},
		Updates:  Updates{Known: true, Total: 12, Security: 3, RebootRequired: true},
	})
	want := map[string]string{
		"ssl:expired.com": SevCritical, "ssl:soon.com": SevCritical, "ssl:later.com": SevWarning,
		"https": SevInfo, "firewall": SevWarning, "updates": SevWarning, "reboot": SevWarning,
		"svc:nginx": SevCritical, "svc:mariadb": SevWarning,
	}
	for id, sev := range want {
		if i := find3(got, id); i == nil || i.Severity != sev {
			t.Errorf("%s: want %s, got %+v", id, sev, i)
		}
	}
	for _, id := range []string{"ssl:fine.com", "ssl:off.com", "svc:redis"} {
		if find3(got, id) != nil {
			t.Errorf("%s must not be reported", id)
		}
	}
	if h := find3(got, "https"); !strings.Contains(h.Detail, "plain.com") || strings.Contains(h.Detail, "off.com") {
		t.Errorf("https insight must list running sites without TLS only: %q", h.Detail)
	}
	if u := find3(got, "updates"); !strings.Contains(u.Title, "3 security updates") {
		t.Errorf("updates title: %q", u.Title)
	}
	if find3(buildInsights(insightInput{Overview: Overview{CPUCores: 1}, Firewall: Firewall{}}), "firewall") != nil {
		t.Error("an unknown firewall state (ufw missing) must not raise an alarm")
	}
}

func TestTrafficRules(t *testing.T) {
	tr := &Traffic{
		Totals: Totals{Requests: 300, Bots: 240},
		Sites: []SiteTraffic{
			{Domain: "broken.com", Totals: Totals{Requests: 100, ServerErr: 30}},
			{Domain: "shaky.com", Totals: Totals{Requests: 100, ServerErr: 3}},
			{Domain: "fine.com", Totals: Totals{Requests: 100, ServerErr: 1}},
			{Domain: "tiny.com", Totals: Totals{Requests: 5, ServerErr: 5}}, // too little data to judge
		},
	}
	got := buildInsights(insightInput{Overview: Overview{CPUCores: 1}, Traffic1h: tr})
	if i := find3(got, "5xx:broken.com"); i == nil || i.Severity != SevCritical {
		t.Errorf("broken.com: %+v", i)
	}
	if i := find3(got, "5xx:shaky.com"); i == nil || i.Severity != SevWarning {
		t.Errorf("shaky.com: %+v", i)
	}
	if find3(got, "5xx:fine.com") != nil || find3(got, "5xx:tiny.com") != nil {
		t.Errorf("below threshold or too few requests must be ignored: %s", ids(got))
	}
	if i := find3(got, "bots"); i == nil || !strings.Contains(i.Title, "80%") {
		t.Errorf("bots: %+v", i)
	}
}

func TestParseUpgrades(t *testing.T) {
	out := `Reading package lists...
Inst libssl3 [3.0.13] (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [arm64])
Inst vim [2:9.1] (2:9.1.1 Ubuntu:24.04/noble-updates [arm64])
Conf libssl3 (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-updates [arm64])
`
	if total, sec := parseUpgrades(out); total != 2 || sec != 1 {
		t.Errorf("got total=%d security=%d", total, sec)
	}
}
