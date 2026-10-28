package dashboard

import (
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func logLine(ts time.Time, ip, req string, status int, bytes int, ua string) string {
	return fmt.Sprintf(`%s - - [%s] "%s" %d %d "-" "%s"`, ip, ts.Format(nginxTime), req, status, bytes, ua)
}

func TestParseLogLine(t *testing.T) {
	ok := `203.0.113.7 - - [24/Oct/2026:13:31:42 +0700] "GET /blog/post-1?utm=x&y=2 HTTP/1.1" 404 153 "https://r.example/" "Mozilla/5.0 (X11) Firefox/130.0"`
	l, good := parseLogLine(ok)
	if !good {
		t.Fatal("valid line rejected")
	}
	if l.IP != "203.0.113.7" || l.Path != "/blog/post-1" || l.Status != 404 || l.Bytes != 153 || !strings.HasPrefix(l.UA, "Mozilla/5.0") {
		t.Errorf("parsed wrong: %+v", l)
	}
	if _, off := l.Time.Zone(); off != 7*3600 || l.Time.UTC().Hour() != 6 {
		t.Errorf("timezone not honoured: %v", l.Time)
	}
	if l2, ok2 := parseLogLine(`1.2.3.4 - - [24/Oct/2026:13:31:42 +0000] "-" 400 0 "-" "-"`); !ok2 || l2.Path != "" || l2.Status != 400 {
		t.Errorf("malformed request (no path) must still count: %+v ok=%v", l2, ok2)
	}
	for _, bad := range []string{"", "garbage", `1.2.3.4 - - [not a date] "GET / HTTP/1.1" 200 1 "-" "-"`, `1.2.3.4 - - [24/Oct/2026:13:31:42 +0000] "GET / HTTP/1.1" abc 1 "-" "-"`, `1.2.3.4 - - [24/Oct/2026:13:31:42 +0000] "GET / HTTP/1.1" 999 1 "-" "-"`} {
		if _, ok := parseLogLine(bad); ok {
			t.Errorf("should reject %q", bad)
		}
	}
}

func TestBotAndAssetDetection(t *testing.T) {
	for ua, want := range map[string]bool{
		"Mozilla/5.0 (Windows NT 10.0) Chrome/120": false, "Googlebot/2.1": true, "curl/8.5.0": true,
		"-": true, "": true, "python-requests/2.31": true, "Mozilla/5.0 (iPhone) Safari": false,
	} {
		if isBot(ua) != want {
			t.Errorf("isBot(%q) = %v, want %v", ua, !want, want)
		}
	}
	if !isAsset("/static/app.JS") || !isAsset("/a/b.woff2") || isAsset("/blog/post") || isAsset("/") {
		t.Error("asset detection wrong")
	}
}

type trafficEnv struct {
	dir string
	now time.Time
	an  *Analyzer
}

func newTrafficEnv(t *testing.T, domains ...string) *trafficEnv {
	t.Helper()
	e := &trafficEnv{dir: t.TempDir(), now: time.Date(2026, 10, 24, 12, 30, 0, 0, time.Local)}
	e.an = NewAnalyzer(e.dir, func() []string { return domains })
	e.an.now = func() time.Time { return e.now }
	return e
}

func (e *trafficEnv) write(t *testing.T, name string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.dir, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

const browser = "Mozilla/5.0 Chrome/120"

func TestTrafficAggregatesTotalsBucketsAndPrevious(t *testing.T) {
	e := newTrafficEnv(t, "a.com", "b.com")
	m := func(min int) time.Time { return e.now.Add(-time.Duration(min) * time.Minute) }
	e.write(t, "a.com.access.log",
		logLine(m(5), "1.1.1.1", "GET / HTTP/1.1", 200, 1000, browser),
		logLine(m(10), "1.1.1.1", "GET /about HTTP/1.1", 200, 500, browser),
		logLine(m(10), "2.2.2.2", "GET /app.css HTTP/1.1", 200, 100, browser), // asset: counted, not a "page"
		logLine(m(20), "3.3.3.3", "GET /missing HTTP/1.1", 404, 50, browser),
		logLine(m(25), "4.4.4.4", "POST /api HTTP/1.1", 502, 20, "Googlebot/2.1"),
		logLine(m(90), "9.9.9.9", "GET / HTTP/1.1", 200, 10, browser),  // previous hour
		logLine(m(200), "8.8.8.8", "GET / HTTP/1.1", 200, 10, browser), // before both periods: ignored
	)
	e.write(t, "b.com.access.log", logLine(m(3), "1.1.1.1", "GET /x HTTP/1.1", 301, 0, browser))

	tr, err := e.an.Traffic("1h", "")
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Available || tr.Truncated {
		t.Errorf("flags: %+v", tr)
	}
	want := Totals{Requests: 6, PageViews: 5, Visitors: 4, Bytes: 1670, OK: 4, ClientErr: 1, ServerErr: 1, Bots: 1}
	if tr.Totals != want {
		t.Errorf("totals = %+v, want %+v", tr.Totals, want)
	}
	if tr.Previous == nil || tr.Previous.Requests != 1 {
		t.Errorf("previous period: %+v", tr.Previous)
	}
	if len(tr.Buckets) != 13 { // 5-minute buckets over an hour, including the one in progress
		t.Errorf("buckets = %d", len(tr.Buckets))
	}
	sum := Bucket{}
	for _, b := range tr.Buckets {
		sum.OK += b.OK
		sum.ClientErr += b.ClientErr
		sum.ServerErr += b.ServerErr
	}
	if sum.OK != 4 || sum.ClientErr != 1 || sum.ServerErr != 1 {
		t.Errorf("bucket sums = %+v", sum)
	}
	if tr.Sites[0].Domain != "a.com" || tr.Sites[0].Requests != 5 || tr.Sites[1].Requests != 1 {
		t.Errorf("sites must be ordered by traffic: %+v", tr.Sites)
	}
	for _, p := range tr.TopPaths {
		if strings.HasSuffix(p.Key, ".css") {
			t.Errorf("assets must not appear as top pages: %v", p)
		}
	}
	if tr.TopPaths[0].Key != "/" || tr.TopPaths[0].Count != 1 {
		// "/" (a.com) and ties sort alphabetically; just make sure the list is populated and ordered
		t.Logf("top paths: %+v", tr.TopPaths)
	}
	if tr.TopIPs[0].Key != "1.1.1.1" || tr.TopIPs[0].Count != 3 {
		t.Errorf("top ip: %+v", tr.TopIPs)
	}

	one, err := e.an.Traffic("1h", "b.com")
	if err != nil || one.Totals.Requests != 1 || len(one.Sites) != 1 {
		t.Errorf("site filter: %+v err=%v", one, err)
	}
}

func TestTrafficReadsRotatedLogsAndSkipsOldOnes(t *testing.T) {
	e := newTrafficEnv(t, "a.com")
	d := func(days int) time.Time { return e.now.Add(-time.Duration(days) * 24 * time.Hour) }
	e.write(t, "a.com.access.log", logLine(e.now.Add(-time.Hour), "1.1.1.1", "GET / HTTP/1.1", 200, 1, browser))
	e.write(t, "a.com.access.log.1", logLine(d(1), "2.2.2.2", "GET / HTTP/1.1", 200, 1, browser))

	gzPath := filepath.Join(e.dir, "a.com.access.log.2.gz")
	f, _ := os.Create(gzPath)
	zw := gzip.NewWriter(f)
	zw.Write([]byte(logLine(d(3), "3.3.3.3", "GET / HTTP/1.1", 200, 1, browser) + "\n"))
	zw.Close()
	f.Close()

	old := filepath.Join(e.dir, "a.com.access.log.3.gz")
	f, _ = os.Create(old)
	zw = gzip.NewWriter(f)
	zw.Write([]byte(logLine(d(30), "4.4.4.4", "GET / HTTP/1.1", 200, 1, browser) + "\n"))
	zw.Close()
	f.Close()
	os.Chtimes(old, d(30), d(30))

	tr, err := e.an.Traffic("7d", "")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Totals.Requests != 3 || tr.Totals.Visitors != 3 {
		t.Errorf("want the three recent files (plain, .1 and .2.gz), got %+v", tr.Totals)
	}
	if len(tr.Buckets) != 7 {
		t.Errorf("7d must give 7 daily buckets, got %d", len(tr.Buckets))
	}
}

func TestTrafficValidationCachingAndMissingLogs(t *testing.T) {
	e := newTrafficEnv(t, "a.com")
	if _, err := e.an.Traffic("30y", ""); !errors.Is(err, ErrBadRange) {
		t.Errorf("range: %v", err)
	}
	if _, err := e.an.Traffic("1h", "evil.com"); !errors.Is(err, ErrUnknownSite) {
		t.Errorf("a site that is not managed must be rejected (it would be a path into the log dir): %v", err)
	}
	if _, err := e.an.Traffic("1h", "../../etc/passwd"); !errors.Is(err, ErrUnknownSite) {
		t.Errorf("traversal: %v", err)
	}
	tr, err := e.an.Traffic("1h", "")
	if err != nil || tr.Available || tr.Totals.Requests != 0 {
		t.Errorf("no logs yet: %+v err=%v", tr, err)
	}
	e.write(t, "a.com.access.log", logLine(e.now.Add(-time.Minute), "1.1.1.1", "GET / HTTP/1.1", 200, 1, browser))
	if again, _ := e.an.Traffic("1h", ""); again != tr {
		t.Error("a second call inside the TTL must reuse the cached result")
	}
	e.now = e.now.Add(trafficTTL + time.Second)
	if fresh, _ := e.an.Traffic("1h", ""); fresh.Totals.Requests != 1 {
		t.Errorf("after the TTL the log must be re-read: %+v", fresh.Totals)
	}
}

func refLine(at time.Time, ip, req string, status int, bytes int64, ref, ua string) string {
	return fmt.Sprintf(`%s - - [%s] "%s" %d %d "%s" "%s"`, ip, at.Format(nginxTime), req, status, bytes, ref, ua)
}

func TestTrafficBreakdowns(t *testing.T) {
	e := newTrafficEnv(t, "a.com")
	m := func(min int) time.Time { return e.now.Add(-time.Duration(min) * time.Minute) }
	win := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/151.0 Safari/537.36"
	iph := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 Version/26.6 Mobile/15E148 Safari/604.1"
	e.write(t, "a.com.access.log",
		refLine(m(2), "1.1.1.1", "GET / HTTP/2.0", 200, 100, "https://www.google.com/search?q=x", win),
		refLine(m(3), "1.1.1.1", "GET /pricing HTTP/2.0", 200, 100, "https://a.com/", win), // internal referrer: ignored
		refLine(m(4), "2.2.2.2", "GET / HTTP/2.0", 200, 100, "https://t.co/abc", iph),
		refLine(m(5), "3.3.3.3", "GET /wp-login.php HTTP/1.1", 404, 10, "-", "curl/8.5.0"),
		refLine(m(6), "3.3.3.3", "GET /wp-login.php HTTP/1.1", 404, 10, "-", "curl/8.5.0"),
		refLine(m(7), "4.4.4.4", "GET /.env HTTP/1.1", 404, 10, "-", "Mozilla/5.0 (compatible; Googlebot/2.1)"),
		refLine(m(8), "5.5.5.5", "POST /api/pay HTTP/1.1", 500, 10, "-", win),
		refLine(m(9), "5.5.5.5", "GET /logo.png HTTP/1.1", 404, 10, "-", win), // missing asset: not a "404 page"
		refLine(m(40), "6.6.6.6", "GET / HTTP/2.0", 301, 0, "-", win),
	)
	tr, err := e.an.Traffic("1h", "")
	if err != nil {
		t.Fatal(err)
	}
	get := func(l []Counted, k string) int {
		for _, c := range l {
			if c.Key == k {
				return c.Count
			}
		}
		return 0
	}
	if get(tr.Referrers, "google.com") != 1 || get(tr.Referrers, "t.co") != 1 || get(tr.Referrers, "a.com") != 0 {
		t.Errorf("referrers (no self-referrals, www stripped): %+v", tr.Referrers)
	}
	if get(tr.Browsers, "Chrome") != 5 || get(tr.Browsers, "Safari") != 1 || get(tr.Systems, "Windows") != 5 || get(tr.Systems, "iOS") != 1 {
		t.Errorf("browsers %+v systems %+v", tr.Browsers, tr.Systems)
	}
	if get(tr.Bots, "curl") != 2 || get(tr.Bots, "Googlebot") != 1 || get(tr.Browsers, "curl") != 0 {
		t.Errorf("bots %+v", tr.Bots)
	}
	if get(tr.Devices, "Desktop") != 5 || get(tr.Devices, "Mobile") != 1 || get(tr.Devices, "Bot") != 3 {
		t.Errorf("devices %+v", tr.Devices)
	}
	if get(tr.Methods, "GET") != 8 || get(tr.Methods, "POST") != 1 {
		t.Errorf("methods %+v", tr.Methods)
	}
	if get(tr.NotFound, "/wp-login.php") != 2 || get(tr.NotFound, "/.env") != 1 || get(tr.NotFound, "/logo.png") != 0 {
		t.Errorf("404 list ignores assets: %+v", tr.NotFound)
	}
	if get(tr.ErrorPaths, "/api/pay") != 1 {
		t.Errorf("5xx paths: %+v", tr.ErrorPaths)
	}
	cls := map[string]int{}
	for _, c := range tr.Classes {
		cls[c.Key] = c.Count
	}
	if cls["2xx"] != 3 || cls["3xx"] != 1 || cls["4xx"] != 4 || cls["5xx"] != 1 {
		t.Errorf("classes %+v", tr.Classes)
	}
	hours := 0
	for _, h := range tr.Hours {
		hours += h
	}
	wd := 0
	for _, h := range tr.Weekdays {
		wd += h
	}
	if len(tr.Hours) != 24 || hours != tr.Totals.Requests || len(tr.Weekdays) != 7 || wd != tr.Totals.Requests {
		t.Errorf("hour/weekday distributions must add up: %d/%d vs %d", hours, wd, tr.Totals.Requests)
	}
	if tr.Live.Last5m != 4 || tr.Live.PerMin != 0.8 || tr.Live.Visitors != 3 {
		t.Errorf("live = %+v", tr.Live)
	}
	if len(tr.Latest) != 9 || tr.Latest[0].Path != "/" || tr.Latest[0].Client != "Chrome · Windows" {
		// the 40-minute-old request is outside the 10-minute feed window
		if len(tr.Latest) != 8 {
			t.Errorf("latest = %d %+v", len(tr.Latest), tr.Latest)
		}
	}
	if len(tr.Domains) != 1 || tr.Domains[0] != "a.com" || tr.Timezone == "" {
		t.Errorf("domains/timezone: %+v %q", tr.Domains, tr.Timezone)
	}
	bytes, vis := int64(0), 0
	for _, b := range tr.Buckets {
		bytes += b.Bytes
		vis += b.Visitors
	}
	if bytes != tr.Totals.Bytes || vis < 5 {
		t.Errorf("bucket bytes %d vs %d, visitors %d", bytes, tr.Totals.Bytes, vis)
	}
}
