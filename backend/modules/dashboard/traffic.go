package dashboard

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LogLine is one parsed nginx "combined" access-log entry.
type LogLine struct {
	Time    time.Time
	IP      string
	Method  string
	Path    string // without the query string
	Status  int
	Bytes   int64
	Referer string
	UA      string
}

const nginxTime = "02/Jan/2006:15:04:05 -0700"

// parseLogLine reads nginx's default "combined" format:
//
//	1.2.3.4 - - [24/Oct/2026:13:31:42 +0000] "GET /a?b=1 HTTP/1.1" 200 612 "-" "UA"
func parseLogLine(line string) (LogLine, bool) {
	var l LogLine
	sp := strings.IndexByte(line, ' ')
	lb := strings.IndexByte(line, '[')
	rb := strings.IndexByte(line, ']')
	if sp <= 0 || lb < 0 || rb < lb {
		return l, false
	}
	t, err := time.Parse(nginxTime, line[lb+1:rb])
	if err != nil {
		return l, false
	}
	l.Time, l.IP = t, line[:sp]

	rest := line[rb+1:]
	q1 := strings.IndexByte(rest, '"')
	if q1 < 0 {
		return l, false
	}
	q2 := strings.IndexByte(rest[q1+1:], '"')
	if q2 < 0 {
		return l, false
	}
	req := rest[q1+1 : q1+1+q2]
	if f := strings.Fields(req); len(f) >= 2 {
		l.Method = f[0]
		l.Path = f[1]
		if i := strings.IndexByte(l.Path, '?'); i >= 0 {
			l.Path = l.Path[:i]
		}
	}
	tail := strings.TrimSpace(rest[q1+q2+2:])
	parts := strings.SplitN(tail, " ", 3)
	if len(parts) < 2 {
		return l, false
	}
	if l.Status, err = strconv.Atoi(parts[0]); err != nil || l.Status < 100 || l.Status > 599 {
		return l, false
	}
	l.Bytes, _ = strconv.ParseInt(parts[1], 10, 64) // "-" counts as 0
	if len(parts) == 3 {
		if i := strings.LastIndex(parts[2], `" "`); i >= 0 {
			l.UA = strings.TrimSuffix(parts[2][i+3:], `"`)
			l.Referer = strings.TrimPrefix(parts[2][:i], `"`)
		}
	}
	return l, true
}

var botMarkers = []string{"bot", "spider", "crawl", "slurp", "curl/", "wget", "python-requests", "go-http-client", "scrapy", "headless", "facebookexternalhit", "okhttp", "libwww"}

func isBot(ua string) bool {
	u := strings.ToLower(ua)
	if u == "" || u == "-" {
		return true
	}
	for _, m := range botMarkers {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

var assetExt = map[string]bool{
	".css": true, ".js": true, ".map": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".ico": true, ".webp": true, ".avif": true, ".woff": true, ".woff2": true, ".ttf": true, ".eot": true, ".otf": true,
}

func isAsset(path string) bool { return assetExt[strings.ToLower(filepath.Ext(path))] }

// ---- result types ----

type Totals struct {
	Requests  int   `json:"requests"`
	PageViews int   `json:"page_views"` // requests for pages, without images, scripts and fonts
	Visitors  int   `json:"visitors"`
	Bytes     int64 `json:"bytes"`
	OK        int   `json:"ok"` // 1xx-3xx
	ClientErr int   `json:"client_err"`
	ServerErr int   `json:"server_err"`
	Bots      int   `json:"bots"`
}

type Bucket struct {
	T         int64 `json:"t"`
	OK        int   `json:"ok"`
	ClientErr int   `json:"client_err"`
	ServerErr int   `json:"server_err"`
	Bytes     int64 `json:"bytes"`
	Visitors  int   `json:"visitors"`
}

// Recent is one of the latest requests, for the live feed.
type Recent struct {
	T      int64  `json:"t"`
	Site   string `json:"site"`
	IP     string `json:"ip"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Bytes  int64  `json:"bytes"`
	Client string `json:"client"` // "Chrome · Windows", a bot name, ...
}

// Live describes the last few minutes.
type Live struct {
	Last5m   int     `json:"last_5m"`
	PerMin   float64 `json:"per_min"`
	Visitors int     `json:"visitors"` // distinct clients in the last 5 minutes
}

type SiteTraffic struct {
	Domain string `json:"domain"`
	Totals
}

type Counted struct {
	Key    string `json:"key"`
	Count  int    `json:"count"`
	Errors int    `json:"errors,omitempty"`
}

type Traffic struct {
	Range     string        `json:"range"`
	Since     int64         `json:"since"`
	Until     int64         `json:"until"`
	BucketSec int64         `json:"bucket_sec"`
	Available bool          `json:"available"` // at least one log file was readable
	Truncated bool          `json:"truncated"` // a size limit stopped the scan early
	Totals    Totals        `json:"totals"`
	Previous  *Totals       `json:"previous"` // the period of equal length just before
	Buckets   []Bucket      `json:"buckets"`
	Sites     []SiteTraffic `json:"sites"`
	TopPaths  []Counted     `json:"top_paths"`
	TopIPs    []Counted     `json:"top_ips"`
	Statuses  []Counted     `json:"statuses"`

	Methods    []Counted `json:"methods"`
	Referrers  []Counted `json:"referrers"`
	Browsers   []Counted `json:"browsers"`
	Systems    []Counted `json:"systems"`
	Devices    []Counted `json:"devices"`
	Bots       []Counted `json:"bots"`        // recognised crawlers and scripts
	NotFound   []Counted `json:"not_found"`   // most requested missing URLs
	ErrorPaths []Counted `json:"error_paths"` // URLs behind most 5xx answers
	Hours      []int     `json:"hours"`       // requests by hour of day (server time)
	Weekdays   []int     `json:"weekdays"`    // requests by weekday, Sunday first
	Classes    []Counted `json:"classes"`     // 2xx / 3xx / 4xx / 5xx
	Latest     []Recent  `json:"latest"`
	Live       Live      `json:"live"`
	Domains    []string  `json:"domains"` // every managed site, for the filter
	Timezone   string    `json:"timezone"`
}

// ---- windows ----

type trafficSpec struct {
	window time.Duration
	bucket time.Duration
	daily  bool // buckets are local calendar days
}

var trafficRanges = map[string]trafficSpec{
	"1h":  {window: time.Hour, bucket: 5 * time.Minute},
	"6h":  {window: 6 * time.Hour, bucket: 15 * time.Minute},
	"24h": {window: 24 * time.Hour, bucket: time.Hour},
	"7d":  {window: 7 * 24 * time.Hour, daily: true},
}

func localMidnight(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// window returns the current period [since, until] and the equally long one before it.
func (s trafficSpec) window2(now time.Time) (since, prevSince time.Time) {
	if s.daily {
		since = localMidnight(now).AddDate(0, 0, -6)
		return since, since.AddDate(0, 0, -7)
	}
	since = now.Add(-s.window)
	return since, since.Add(-s.window)
}

func (s trafficSpec) bucketStart(t time.Time) int64 {
	if s.daily {
		return localMidnight(t).Unix()
	}
	b := int64(s.bucket / time.Second)
	return t.Unix() / b * b
}

func (s trafficSpec) bucketSec() int64 {
	if s.daily {
		return 86400
	}
	return int64(s.bucket / time.Second)
}

// ---- reading logs ----

const (
	maxBytesPerSite = 96 << 20 // uncompressed bytes scanned per site and request
	maxTrackedKeys  = 20000    // distinct paths/IPs counted before new ones are ignored
)

// logFiles lists a site's access logs newest first, skipping files that only hold data older than since.
func logFiles(dir, domain string, since time.Time) []string {
	base := filepath.Join(dir, domain+".access.log")
	cands := []string{base, base + ".1"}
	for i := 2; i <= 15; i++ {
		cands = append(cands, fmt.Sprintf("%s.%d.gz", base, i))
	}
	var out []string
	for i, f := range cands {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if i > 0 && st.ModTime().Before(since) {
			break // rotated before the window began: this and every older file is out of range
		}
		out = append(out, f)
	}
	return out
}

// scanFile feeds every parsable line of one log file to fn. budget is decreased by the bytes read.
func scanFile(path string, budget *int64, fn func(LogLine)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	} else if st, err := f.Stat(); err == nil && st.Size() > *budget {
		// Only the tail fits the budget; the first line after the seek is partial.
		if _, err := f.Seek(st.Size()-*budget, io.SeekStart); err != nil {
			return err
		}
		br := bufio.NewReader(f)
		_, _ = br.ReadString('\n')
		r = br
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		*budget -= int64(len(line)) + 1
		if l, ok := parseLogLine(line); ok {
			fn(l)
		}
		if *budget <= 0 {
			return errBudget
		}
	}
	return sc.Err()
}

var errBudget = errors.New("scan budget exhausted")

// ---- aggregation ----

type siteAgg struct {
	domain   string
	cur, prv Totals
	curIPs   map[string]struct{}
	prvIPs   map[string]struct{}
}

type aggregator struct {
	spec     trafficSpec
	since    time.Time // start of the current period
	prevFrom time.Time // start of the previous period
	until    time.Time
	buckets  map[int64]*Bucket
	paths    map[string]*Counted
	ips      map[string]int
	statuses map[int]int
	allIPs   map[string]struct{}
	prevIPs  map[string]struct{}

	bucketIPs  map[int64]map[string]struct{}
	methods    map[string]int
	referrers  map[string]int
	browsers   map[string]int
	systems    map[string]int
	devices    map[string]int
	bots       map[string]int
	notFound   map[string]int
	errorPaths map[string]int
	hours      [24]int
	weekdays   [7]int
	classes    [4]int // 2xx 3xx 4xx 5xx (1xx counts as 2xx)
	recent     []Recent
	live5      int
	live5IPs   map[string]struct{}
	clientMemo map[string]Client
}

func newAggregator(spec trafficSpec, now time.Time) *aggregator {
	since, prev := spec.window2(now)
	a := &aggregator{
		spec: spec, since: since, prevFrom: prev, until: now,
		buckets: map[int64]*Bucket{}, paths: map[string]*Counted{}, ips: map[string]int{},
		statuses: map[int]int{}, allIPs: map[string]struct{}{}, prevIPs: map[string]struct{}{},
		bucketIPs: map[int64]map[string]struct{}{}, methods: map[string]int{}, referrers: map[string]int{}, browsers: map[string]int{},
		systems: map[string]int{}, devices: map[string]int{}, bots: map[string]int{}, notFound: map[string]int{}, errorPaths: map[string]int{},
		live5IPs: map[string]struct{}{}, clientMemo: map[string]Client{},
	}
	for t := spec.bucketStart(since); t <= now.Unix(); {
		a.buckets[t] = &Bucket{T: t}
		if spec.daily {
			t = time.Unix(t, 0).In(time.Local).AddDate(0, 0, 1).Unix()
		} else {
			t += spec.bucketSec()
		}
	}
	return a
}

func tally(t *Totals, l LogLine) {
	t.Requests++
	t.Bytes += l.Bytes
	switch {
	case l.Status >= 500:
		t.ServerErr++
	case l.Status >= 400:
		t.ClientErr++
	default:
		t.OK++
	}
	if isBot(l.UA) {
		t.Bots++
	}
}

func (a *aggregator) add(s *siteAgg, l LogLine) {
	switch {
	case !l.Time.Before(a.since):
		tally(&s.cur, l)
		s.curIPs[l.IP] = struct{}{}
		if len(a.allIPs) < maxTrackedKeys*5 {
			a.allIPs[l.IP] = struct{}{}
		}
		bs := a.spec.bucketStart(l.Time)
		if b := a.buckets[bs]; b != nil {
			switch {
			case l.Status >= 500:
				b.ServerErr++
			case l.Status >= 400:
				b.ClientErr++
			default:
				b.OK++
			}
			b.Bytes += l.Bytes
			set := a.bucketIPs[bs]
			if set == nil {
				set = map[string]struct{}{}
				a.bucketIPs[bs] = set
			}
			if len(set) < maxTrackedKeys {
				set[l.IP] = struct{}{}
			}
		}
		a.detail(s, l)
		a.statuses[l.Status]++
		if len(a.ips) < maxTrackedKeys || a.ips[l.IP] > 0 {
			a.ips[l.IP]++
		}
		if !isAsset(l.Path) && l.Path != "" {
			c := a.paths[l.Path]
			if c == nil && len(a.paths) < maxTrackedKeys {
				c = &Counted{Key: l.Path}
				a.paths[l.Path] = c
			}
			if c != nil {
				c.Count++
				if l.Status >= 400 {
					c.Errors++
				}
			}
		}
	case !l.Time.Before(a.prevFrom):
		tally(&s.prv, l)
		s.prvIPs[l.IP] = struct{}{}
		if len(a.prevIPs) < maxTrackedKeys*5 {
			a.prevIPs[l.IP] = struct{}{}
		}
	}
}

const recentWindow = 10 * time.Minute

func bump(m map[string]int, key string) {
	if _, ok := m[key]; ok || len(m) < maxTrackedKeys {
		m[key]++
	}
}

// client classifies a User-Agent once per distinct string.
func (a *aggregator) client(ua string) Client {
	if c, ok := a.clientMemo[ua]; ok {
		return c
	}
	c := classifyUA(ua)
	if len(a.clientMemo) < 5000 {
		a.clientMemo[ua] = c
	}
	return c
}

// detail records the dimensions beyond totals for a request inside the current period.
func (a *aggregator) detail(s *siteAgg, l LogLine) {
	asset := isAsset(l.Path)
	if !asset && l.Path != "" {
		s.cur.PageViews++
	}
	c := a.client(l.UA)
	if l.Method != "" {
		bump(a.methods, l.Method)
	}
	if c.Bot != "" {
		bump(a.bots, c.Bot)
	} else {
		bump(a.browsers, c.Browser)
		bump(a.systems, c.OS)
	}
	bump(a.devices, c.Device)
	if h := refererHost(l.Referer); h != "" && h != s.domain && !strings.HasSuffix(h, "."+s.domain) {
		bump(a.referrers, h)
	}
	lt := l.Time.In(time.Local)
	a.hours[lt.Hour()]++
	a.weekdays[lt.Weekday()]++
	switch {
	case l.Status >= 500:
		a.classes[3]++
		if l.Path != "" {
			bump(a.errorPaths, l.Path)
		}
	case l.Status >= 400:
		a.classes[2]++
		if l.Status == 404 && !asset && l.Path != "" {
			bump(a.notFound, l.Path)
		}
	case l.Status >= 300:
		a.classes[1]++
	default:
		a.classes[0]++
	}
	if age := a.until.Sub(l.Time); age >= 0 && age <= recentWindow {
		if len(a.recent) < 3000 {
			who := c.Bot
			if who == "" {
				who = c.Browser + " · " + c.OS
			}
			a.recent = append(a.recent, Recent{T: l.Time.Unix(), Site: s.domain, IP: l.IP, Method: l.Method, Path: l.Path, Status: l.Status, Bytes: l.Bytes, Client: who})
		}
		if age <= 5*time.Minute {
			a.live5++
			a.live5IPs[l.IP] = struct{}{}
		}
	}
}

func topMap(m map[string]int, n int) []Counted {
	out := make([]Counted, 0, len(m))
	for k, v := range m {
		out = append(out, Counted{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func topCounted(m map[string]*Counted, n int) []Counted {
	out := make([]Counted, 0, len(m))
	for _, c := range m {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (a *aggregator) result(rng string, sites []*siteAgg, available, truncated bool) *Traffic {
	t := &Traffic{
		Range: rng, Since: a.since.Unix(), Until: a.until.Unix(), BucketSec: a.spec.bucketSec(),
		Available: available, Truncated: truncated, Buckets: []Bucket{}, Sites: []SiteTraffic{},
		TopPaths: topCounted(a.paths, 8), TopIPs: []Counted{}, Statuses: []Counted{},
		Methods: topMap(a.methods, 8), Referrers: topMap(a.referrers, 8), Browsers: topMap(a.browsers, 8), Systems: topMap(a.systems, 8),
		Devices: topMap(a.devices, 4), Bots: topMap(a.bots, 8), NotFound: topMap(a.notFound, 8), ErrorPaths: topMap(a.errorPaths, 8),
		Hours: append([]int(nil), a.hours[:]...), Weekdays: append([]int(nil), a.weekdays[:]...), Latest: []Recent{},
		Live: Live{Last5m: a.live5, PerMin: float64(a.live5) / 5, Visitors: len(a.live5IPs)},
	}
	t.Timezone = a.until.In(time.Local).Format("MST -07:00")
	for i, name := range []string{"2xx", "3xx", "4xx", "5xx"} {
		t.Classes = append(t.Classes, Counted{Key: name, Count: a.classes[i]})
	}
	sort.SliceStable(a.recent, func(i, j int) bool { return a.recent[i].T > a.recent[j].T })
	if len(a.recent) > 15 {
		a.recent = a.recent[:15]
	}
	t.Latest = append(t.Latest, a.recent...)
	for _, s := range sites {
		st := SiteTraffic{Domain: s.domain, Totals: s.cur}
		st.Visitors = len(s.curIPs)
		t.Sites = append(t.Sites, st)
		addTotals(&t.Totals, s.cur)
		if t.Previous == nil {
			t.Previous = &Totals{}
		}
		addTotals(t.Previous, s.prv)
	}
	t.Totals.Visitors = len(a.allIPs)
	if t.Previous != nil {
		t.Previous.Visitors = len(a.prevIPs)
		if t.Previous.Requests == 0 {
			t.Previous = nil // nothing to compare with
		}
	}
	sort.Slice(t.Sites, func(i, j int) bool {
		if t.Sites[i].Requests != t.Sites[j].Requests {
			return t.Sites[i].Requests > t.Sites[j].Requests
		}
		return t.Sites[i].Domain < t.Sites[j].Domain
	})
	for k, b := range a.buckets {
		b.Visitors = len(a.bucketIPs[k])
		t.Buckets = append(t.Buckets, *b)
	}
	sort.Slice(t.Buckets, func(i, j int) bool { return t.Buckets[i].T < t.Buckets[j].T })
	ipc := make([]Counted, 0, len(a.ips))
	for ip, n := range a.ips {
		ipc = append(ipc, Counted{Key: ip, Count: n})
	}
	sort.Slice(ipc, func(i, j int) bool {
		if ipc[i].Count != ipc[j].Count {
			return ipc[i].Count > ipc[j].Count
		}
		return ipc[i].Key < ipc[j].Key
	})
	if len(ipc) > 8 {
		ipc = ipc[:8]
	}
	t.TopIPs = ipc
	for code, n := range a.statuses {
		t.Statuses = append(t.Statuses, Counted{Key: strconv.Itoa(code), Count: n})
	}
	sort.Slice(t.Statuses, func(i, j int) bool {
		if t.Statuses[i].Count != t.Statuses[j].Count {
			return t.Statuses[i].Count > t.Statuses[j].Count
		}
		return t.Statuses[i].Key < t.Statuses[j].Key
	})
	if len(t.Statuses) > 8 {
		t.Statuses = t.Statuses[:8]
	}
	return t
}

func addTotals(dst *Totals, s Totals) {
	dst.Requests += s.Requests
	dst.PageViews += s.PageViews
	dst.Bytes += s.Bytes
	dst.OK += s.OK
	dst.ClientErr += s.ClientErr
	dst.ServerErr += s.ServerErr
	dst.Bots += s.Bots
}

// ---- analyzer ----

// Analyzer turns the nginx logs of the managed websites into traffic statistics.
// Results are cached briefly: parsing is the expensive part and the dashboard polls.
type Analyzer struct {
	logDir  string
	domains func() []string
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]cachedTraffic
}

type cachedTraffic struct {
	at  time.Time
	val *Traffic
}

const trafficTTL = 45 * time.Second

func NewAnalyzer(logDir string, domains func() []string) *Analyzer {
	return &Analyzer{logDir: logDir, domains: domains, now: time.Now, cache: map[string]cachedTraffic{}}
}

var ErrBadRange = errors.New("unknown range")
var ErrUnknownSite = errors.New("unknown site")

// Traffic aggregates one range for all sites, or for a single site if site is set.
func (a *Analyzer) Traffic(rng, site string) (*Traffic, error) {
	spec, ok := trafficRanges[rng]
	if !ok {
		return nil, ErrBadRange
	}
	domains := a.domains()
	if site != "" {
		found := false
		for _, d := range domains {
			found = found || d == site
		}
		if !found {
			return nil, ErrUnknownSite
		}
		domains = []string{site}
	}
	key := rng + "|" + site
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.cache[key]; ok && a.now().Sub(c.at) < trafficTTL {
		return c.val, nil
	}
	now := a.now()
	agg := newAggregator(spec, now)
	var sites []*siteAgg
	available, truncated := false, false
	for _, d := range domains {
		s := &siteAgg{domain: d, curIPs: map[string]struct{}{}, prvIPs: map[string]struct{}{}}
		sites = append(sites, s)
		budget := int64(maxBytesPerSite)
		for _, f := range logFiles(a.logDir, d, agg.prevFrom) {
			available = true
			if err := scanFile(f, &budget, func(l LogLine) { agg.add(s, l) }); errors.Is(err, errBudget) {
				truncated = true
				break
			}
		}
	}
	res := agg.result(rng, sites, available, truncated)
	res.Domains = append([]string{}, a.domains()...)
	sort.Strings(res.Domains)
	a.cache[key] = cachedTraffic{at: now, val: res}
	return res, nil
}
