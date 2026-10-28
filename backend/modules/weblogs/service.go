// Package weblogs reads, filters, exports and clears the nginx access/error logs of the panel's websites.
package weblogs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"rpanel/backend/modules/website"

	"gorm.io/gorm"
)

var (
	ErrBadInput = errors.New("invalid input")
	ErrNotFound = errors.New("website not found")

	accessTimeRe = regexp.MustCompile(`\[(\d{2}/[A-Za-z]{3}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4})\]`)
	errorTimeRe  = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`)
)

const (
	// readWindow is how much of the end of a file is scanned; the newest part is what people look at.
	readWindow = 8 << 20
	maxLines   = 5000
	AllSites   = "all"
)

type Service struct {
	db     *gorm.DB
	logDir string
	now    func() time.Time
}

func NewService(db *gorm.DB, logDir string) *Service {
	return &Service{db: db, logDir: logDir, now: time.Now}
}

type SiteLog struct {
	Domain      string `json:"domain"`
	AccessSize  int64  `json:"access_size"`
	ErrorSize   int64  `json:"error_size"`
	AccessMTime int64  `json:"access_mtime"`
}

func (s *Service) path(domain, kind string) string {
	return filepath.Join(s.logDir, domain+"."+kind+".log")
}

func validKind(k string) bool { return k == "access" || k == "error" }

// Sites lists the websites with the size of their logs.
func (s *Service) Sites() ([]SiteLog, error) {
	var ws []website.Website
	if err := s.db.Order("domain").Find(&ws).Error; err != nil {
		return nil, err
	}
	out := make([]SiteLog, 0, len(ws))
	for _, w := range ws {
		sl := SiteLog{Domain: w.Domain}
		if fi, err := os.Stat(s.path(w.Domain, "access")); err == nil {
			sl.AccessSize, sl.AccessMTime = fi.Size(), fi.ModTime().Unix()
		}
		if fi, err := os.Stat(s.path(w.Domain, "error")); err == nil {
			sl.ErrorSize = fi.Size()
		}
		out = append(out, sl)
	}
	return out, nil
}

// domains resolves the requested site; only websites known to the panel are readable (no path traversal).
func (s *Service) domains(domain string) ([]string, error) {
	if domain == AllSites {
		var ws []website.Website
		if err := s.db.Order("domain").Find(&ws).Error; err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ws))
		for _, w := range ws {
			out = append(out, w.Domain)
		}
		return out, nil
	}
	var n int64
	if err := s.db.Model(&website.Website{}).Where("domain = ?", domain).Count(&n).Error; err != nil || n == 0 {
		return nil, ErrNotFound
	}
	return []string{domain}, nil
}

// Query narrows the lines returned.
type Query struct {
	Domain  string
	Kind    string    // access | error
	Lines   int       // 0 = as many as allowed
	Keyword string    // case-insensitive substring
	From    time.Time // zero = no lower bound
	To      time.Time // zero = no upper bound (inclusive, whole day)
}

type Result struct {
	Lines     []string `json:"lines"`
	Matched   int      `json:"matched"`   // lines that matched before the Lines limit
	Truncated bool     `json:"truncated"` // older lines exist that were not scanned or returned
	Size      int64    `json:"size"`      // total size of the log files read
}

// lineTime extracts the timestamp of a log line (zero when it has none).
func lineTime(l string) time.Time {
	if m := accessTimeRe.FindStringSubmatch(l); m != nil {
		if t, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[1]); err == nil {
			return t
		}
	}
	if m := errorTimeRe.FindStringSubmatch(l); m != nil {
		if t, err := time.ParseInLocation("2006/01/02 15:04:05", m[1], time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

// tailLines returns the lines in the last readWindow bytes of path (oldest first) and whether older data exists.
func tailLines(path string) (lines []string, size int64, more bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, false, err
	}
	size = fi.Size()
	start := int64(0)
	if size > readWindow {
		start, more = size-readWindow, true
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, size, more, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	first := true
	for sc.Scan() {
		if first && start > 0 {
			first = false // the first line is probably cut in half
			continue
		}
		first = false
		lines = append(lines, sc.Text())
	}
	return lines, size, more, nil
}

// Read returns the matching lines, newest last. For "all" the sites are merged by timestamp and every
// line is prefixed with its domain.
func (s *Service) Read(q Query) (*Result, error) {
	if !validKind(q.Kind) {
		return nil, fmt.Errorf("%w: log type", ErrBadInput)
	}
	if q.Lines < 0 || q.Lines > maxLines {
		q.Lines = maxLines
	}
	doms, err := s.domains(q.Domain)
	if err != nil {
		return nil, err
	}
	type entry struct {
		t    time.Time
		text string
		seq  int
	}
	var all []entry
	res := &Result{Lines: []string{}}
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	seq := 0
	for _, d := range doms {
		lines, size, more, err := tailLines(s.path(d, q.Kind))
		if err != nil {
			return nil, err
		}
		res.Size += size
		if more {
			res.Truncated = true
		}
		for _, l := range lines {
			if kw != "" && !strings.Contains(strings.ToLower(l), kw) {
				continue
			}
			t := lineTime(l)
			if !q.From.IsZero() && !t.IsZero() && t.Before(q.From) {
				continue
			}
			if !q.To.IsZero() && !t.IsZero() && !t.Before(q.To.Add(24*time.Hour)) {
				continue
			}
			if (!q.From.IsZero() || !q.To.IsZero()) && t.IsZero() {
				continue
			}
			if q.Domain == AllSites {
				l = "[" + d + "] " + l
			}
			seq++
			all = append(all, entry{t: t, text: l, seq: seq})
		}
	}
	if q.Domain == AllSites {
		sort.SliceStable(all, func(i, j int) bool {
			if all[i].t.Equal(all[j].t) {
				return all[i].seq < all[j].seq
			}
			return all[i].t.Before(all[j].t)
		})
	}
	res.Matched = len(all)
	limit := q.Lines
	if limit == 0 {
		limit = maxLines
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
		res.Truncated = true
	}
	for _, e := range all {
		res.Lines = append(res.Lines, e.text)
	}
	return res, nil
}

// File returns the log file path of one site for download.
func (s *Service) File(domain, kind string) (string, error) {
	if !validKind(kind) {
		return "", fmt.Errorf("%w: log type", ErrBadInput)
	}
	if domain == AllSites {
		return "", fmt.Errorf("%w: choose a website to export", ErrBadInput)
	}
	if _, err := s.domains(domain); err != nil {
		return "", err
	}
	p := s.path(domain, kind)
	if _, err := os.Stat(p); err != nil {
		return "", ErrNotFound
	}
	return p, nil
}

// Clear empties a log in place. nginx keeps its file handle (append mode), so no reload is needed.
func (s *Service) Clear(domain, kind string) error {
	if !validKind(kind) {
		return fmt.Errorf("%w: log type", ErrBadInput)
	}
	if domain == AllSites {
		return fmt.Errorf("%w: choose a website to clear", ErrBadInput)
	}
	if _, err := s.domains(domain); err != nil {
		return err
	}
	if err := os.Truncate(s.path(domain, kind), 0); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ParseDay parses YYYY-MM-DD in the server's time zone.
func ParseDay(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation("2006-01-02", v, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date must look like 2026-10-28", ErrBadInput)
	}
	return t, nil
}
