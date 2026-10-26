package databases

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/glebarez/go-sqlite"
	"testing"
	"time"
)

func TestIsQuery(t *testing.T) {
	for stmt, want := range map[string]bool{
		"SELECT 1": true, "  select * from t": true, "-- c\nSHOW TABLES": true, "/* x */ explain select 1": true,
		"WITH a AS (SELECT 1) SELECT * FROM a": true, "INSERT INTO t VALUES (1)": false, "update t set a=1": false,
		"drop table t": false, "-- only comment": false,
	} {
		if got := isQuery(stmt); got != want {
			t.Errorf("isQuery(%q) = %v, want %v", stmt, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	if v := normalize([]byte{0xff, 0x00}); v != "0xff00" {
		t.Errorf("binary: %v", v)
	}
	if v := normalize(int64(1 << 60)); v != "1152921504606846976" {
		t.Errorf("big int: %v", v)
	}
	if v := normalize(int64(5)); v != int64(5) {
		t.Errorf("small int: %v", v)
	}
	if v := normalize(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)); v != "2024-01-02 03:04:05" {
		t.Errorf("time: %v", v)
	}
	if normalize(nil) != nil {
		t.Error("nil")
	}
}

func TestQuotingAndLike(t *testing.T) {
	if got := mysqlDialect.quote("a`b"); got != "`a``b`" {
		t.Errorf("mysql quote: %s", got)
	}
	if got := pgDialect.quote(`a"b`); got != `"a""b"` {
		t.Errorf("pg quote: %s", got)
	}
	if got := escapeLike(`50%_\`); got != `50\%\_\\` {
		t.Errorf("escapeLike: %s", got)
	}
}

func sqliteBrowser(t *testing.T) *sqlBrowser {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, note TEXT)`); err != nil {
		t.Fatal(err)
	}
	q := func(s string) string { return `"` + s + `"` }
	return &sqlBrowser{db: db, name: "main", d: dialect{
		quote: q, qualify: func(_, t string) string { return q(t) },
		ph: func(int) string { return "?" }, castText: func(c string) string { return "CAST(" + c + " AS TEXT)" }, like: "LIKE",
		tablesSQL: `SELECT name, 'BASE TABLE', 0, 0 FROM sqlite_master WHERE type='table'`, tableArgs: func(string) []any { return nil },
		columnsSQL: `SELECT name, type, "notnull" = 0, pk > 0, dflt_value, '' FROM pragma_table_info(?)`,
		columnArgs: func(_, t string) []any { return []any{t} },
	}}
}

func TestBrowserRowCRUD(t *testing.T) {
	b, ctx := sqliteBrowser(t), context.Background()
	s := func(v string) *string { return &v }
	if err := b.Insert(ctx, "users", map[string]*string{"name": s("ann <b>"), "note": nil}); err != nil {
		t.Fatal(err)
	}
	if err := b.Insert(ctx, "users", map[string]*string{"name": s("bob")}); err != nil {
		t.Fatal(err)
	}
	p, err := b.Select(ctx, "users", SelectOpts{Order: "name", Desc: true})
	if err != nil || p.Total != 2 || p.Rows[0][1] != "bob" {
		t.Fatalf("select: %+v %v", p, err)
	}
	if p, _ = b.Select(ctx, "users", SelectOpts{Search: "ann"}); p.Total != 1 {
		t.Fatalf("search total = %d", p.Total)
	}
	if n, err := b.Update(ctx, "users", map[string]string{"id": "1"}, map[string]*string{"note": s("hi")}); err != nil || n != 1 {
		t.Fatalf("update: %d %v", n, err)
	}
	// unknown columns, partial pk and unknown tables are rejected before any SQL runs
	if _, err := b.Update(ctx, "users", map[string]string{"id": "1"}, map[string]*string{"x; DROP": s("1")}); !errors.Is(err, ErrBadInput) {
		t.Errorf("bad column: %v", err)
	}
	if _, err := b.Delete(ctx, "users", map[string]string{}); !errors.Is(err, ErrBadInput) {
		t.Errorf("empty pk: %v", err)
	}
	if _, err := b.Select(ctx, "nope", SelectOpts{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown table: %v", err)
	}
	if n, err := b.Delete(ctx, "users", map[string]string{"id": "2"}); err != nil || n != 1 {
		t.Fatalf("delete: %d %v", n, err)
	}
	r, err := b.Exec(ctx, "SELECT name FROM users")
	if err != nil || !r.IsQuery || len(r.Rows) != 1 {
		t.Fatalf("exec select: %+v %v", r, err)
	}
	r, err = b.Exec(ctx, "UPDATE users SET note = 'x'")
	if err != nil || r.IsQuery || r.Affected != 1 {
		t.Fatalf("exec update: %+v %v", r, err)
	}
}

func TestPanelDatabaseIsProtected(t *testing.T) {
	ProtectPanelDatabase("RPanel_Test", "rpanel_role")
	if !IsSystemDB("rpanel_test") || !IsSystemUser("RPANEL_ROLE") {
		t.Fatal("panel database/role must be treated as system objects")
	}
	s := &Service{}
	if err := s.withBrowser(context.Background(), "x", "rpanel_test", func(Browser) error { return nil }); !errors.Is(err, ErrProtected) {
		t.Fatalf("browse panel db: %v", err)
	}
}
