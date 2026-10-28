package weblogs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rpanel/backend/modules/website"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setup(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&website.Website{}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a.example.com", "b.example.com"} {
		db.Create(&website.Website{Domain: d, Type: "static", Status: "running"})
	}
	return NewService(db, dir), dir
}

const (
	a1 = `1.1.1.1 - - [28/Oct/2026:04:38:38 +0700] "GET /a HTTP/2.0" 200 10 "-" "curl"`
	a2 = `2.2.2.2 - - [29/Oct/2026:10:00:00 +0700] "GET /login HTTP/2.0" 404 5 "-" "Mozilla"`
	a3 = `3.3.3.3 - - [30/Oct/2026:12:00:00 +0700] "POST /api HTTP/2.0" 500 7 "-" "curl"`
	b1 = `9.9.9.9 - - [28/Oct/2026:23:59:59 +0700] "GET /b HTTP/2.0" 200 1 "-" "x"`
)

func write(t *testing.T, dir, name string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSitesListsSizes(t *testing.T) {
	s, dir := setup(t)
	write(t, dir, "a.example.com.access.log", a1, a2)
	sites, err := s.Sites()
	if err != nil || len(sites) != 2 || sites[0].Domain != "a.example.com" || sites[0].AccessSize == 0 || sites[1].AccessSize != 0 {
		t.Fatalf("%+v %v", sites, err)
	}
}

func TestReadLimitAndKeyword(t *testing.T) {
	s, dir := setup(t)
	write(t, dir, "a.example.com.access.log", a1, a2, a3)
	r, err := s.Read(Query{Domain: "a.example.com", Kind: "access", Lines: 2})
	if err != nil || len(r.Lines) != 2 || r.Lines[1] != a3 || r.Matched != 3 || !r.Truncated {
		t.Fatalf("limit: %+v %v", r, err)
	}
	r, _ = s.Read(Query{Domain: "a.example.com", Kind: "access", Keyword: "LOGIN"})
	if len(r.Lines) != 1 || r.Lines[0] != a2 {
		t.Errorf("keyword is case-insensitive: %+v", r)
	}
	r, _ = s.Read(Query{Domain: "a.example.com", Kind: "access", Lines: 0})
	if len(r.Lines) != 3 || r.Truncated {
		t.Errorf("0 means all: %+v", r)
	}
}

func TestDateRangeIsInclusive(t *testing.T) {
	s, dir := setup(t)
	write(t, dir, "a.example.com.access.log", a1, a2, a3)
	from, _ := ParseDay("2026-10-29")
	to, _ := ParseDay("2026-10-29")
	r, _ := s.Read(Query{Domain: "a.example.com", Kind: "access", From: from, To: to})
	if len(r.Lines) != 1 || r.Lines[0] != a2 {
		t.Fatalf("one day: %+v", r)
	}
	to2, _ := ParseDay("2026-10-30")
	r, _ = s.Read(Query{Domain: "a.example.com", Kind: "access", From: from, To: to2})
	if len(r.Lines) != 2 {
		t.Errorf("29th-30th: %+v", r)
	}
	if _, err := ParseDay("29/10/2026"); !errors.Is(err, ErrBadInput) {
		t.Error("bad date")
	}
}

func TestAllSitesMergesByTimeWithDomainPrefix(t *testing.T) {
	s, dir := setup(t)
	write(t, dir, "a.example.com.access.log", a1, a3)
	write(t, dir, "b.example.com.access.log", b1)
	r, err := s.Read(Query{Domain: AllSites, Kind: "access"})
	if err != nil || len(r.Lines) != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	if !strings.HasPrefix(r.Lines[0], "[a.example.com] 1.1.1.1") || !strings.HasPrefix(r.Lines[1], "[b.example.com] 9.9.9.9") || !strings.HasPrefix(r.Lines[2], "[a.example.com] 3.3.3.3") {
		t.Errorf("order/prefix wrong: %v", r.Lines)
	}
}

func TestErrorLogTimesAreParsed(t *testing.T) {
	s, dir := setup(t)
	write(t, dir, "a.example.com.error.log",
		`2026/10/28 04:38:38 [error] 1#1: *1 open() failed`,
		`2026/10/30 04:38:38 [warn] 1#1: *2 upstream slow`)
	from, _ := ParseDay("2026-10-30")
	r, _ := s.Read(Query{Domain: "a.example.com", Kind: "error", From: from})
	if len(r.Lines) != 1 || !strings.Contains(r.Lines[0], "upstream slow") {
		t.Fatalf("%+v", r)
	}
}

func TestOnlyKnownSitesAreReadable(t *testing.T) {
	s, dir := setup(t)
	write(t, dir, "secret.access.log", a1)
	for _, d := range []string{"secret", "../secret", "a.example.com/../../x", ""} {
		if _, err := s.Read(Query{Domain: d, Kind: "access"}); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q must not be readable: %v", d, err)
		}
	}
	if _, err := s.Read(Query{Domain: "a.example.com", Kind: "../etc/passwd"}); !errors.Is(err, ErrBadInput) {
		t.Error("bad kind")
	}
	if _, err := s.File("../x", "access"); err == nil {
		t.Error("download of an unknown site")
	}
}

func TestFileAndClear(t *testing.T) {
	s, dir := setup(t)
	write(t, dir, "a.example.com.access.log", a1)
	p, err := s.File("a.example.com", "access")
	if err != nil || filepath.Base(p) != "a.example.com.access.log" {
		t.Fatalf("%v %v", p, err)
	}
	if _, err := s.File(AllSites, "access"); !errors.Is(err, ErrBadInput) {
		t.Error("cannot export all")
	}
	if err := s.Clear("a.example.com", "access"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Size() != 0 {
		t.Error("log not emptied")
	}
	if err := s.Clear(AllSites, "access"); !errors.Is(err, ErrBadInput) {
		t.Error("cannot clear all")
	}
	if err := s.Clear("b.example.com", "access"); err != nil {
		t.Errorf("clearing a missing log is fine: %v", err)
	}
}

func TestLargeLogOnlyScansTheTail(t *testing.T) {
	s, dir := setup(t)
	f, _ := os.Create(filepath.Join(dir, "a.example.com.access.log"))
	line := a1 + "\n"
	for n := 0; n < (readWindow+1<<20)/len(line); n++ {
		f.WriteString(line)
	}
	f.WriteString(a3 + "\n")
	f.Close()
	r, err := s.Read(Query{Domain: "a.example.com", Kind: "access", Lines: 1})
	if err != nil || !r.Truncated || len(r.Lines) != 1 || r.Lines[0] != a3 {
		t.Fatalf("%v %v", r, err)
	}
}
