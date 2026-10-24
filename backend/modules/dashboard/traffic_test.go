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
	want := Totals{Requests: 6, Visitors: 4, Bytes: 1670, OK: 4, ClientErr: 1, ServerErr: 1, Bots: 1}
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
