package databases

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxPageSize = 500
	maxQueryRow = 1000
	maxCellSize = 64 << 10
)

type TableInfo struct {
	Name string `json:"name"`
	Type string `json:"type"` // "BASE TABLE" | "VIEW"
	Rows int64  `json:"rows"` // estimate
	Size int64  `json:"size"`
}

type Column struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Primary  bool    `json:"primary"`
	Default  *string `json:"default"`
	Extra    string  `json:"extra"`
}

type SelectOpts struct {
	Limit, Offset int
	Order         string
	Desc          bool
	Search        string
}

type Page struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Total   int64    `json:"total"`
}

type QueryResult struct {
	IsQuery   bool     `json:"is_query"`
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Affected  int64    `json:"affected"`
	Truncated bool     `json:"truncated"`
	ElapsedMS int64    `json:"elapsed_ms"`
}

// Browser works inside one database of a server: table structure, row editing
// and free-form SQL. Table and column names always come from the catalog, never
// straight from the request.
type Browser interface {
	Close() error
	Tables(ctx context.Context) ([]TableInfo, error)
	Columns(ctx context.Context, table string) ([]Column, error)
	Select(ctx context.Context, table string, o SelectOpts) (*Page, error)
	Insert(ctx context.Context, table string, vals map[string]*string) error
	Update(ctx context.Context, table string, pk map[string]string, vals map[string]*string) (int64, error)
	Delete(ctx context.Context, table string, pk map[string]string) (int64, error)
	Truncate(ctx context.Context, table string) error
	Drop(ctx context.Context, table string) error
	Exec(ctx context.Context, stmt string) (*QueryResult, error)
}

// Browsable is implemented by engines that can open a Browser on one database.
type Browsable interface {
	Browse(ctx context.Context, db string) (Browser, error)
}

type dialect struct {
	quote      func(string) string
	qualify    func(db, table string) string
	ph         func(i int) string // 1-based placeholder
	castText   func(col string) string
	like       string
	tablesSQL  string
	tableArgs  func(db string) []any
	columnsSQL string
	columnArgs func(db, table string) []any
	use        func(db string) string // statement run on the console connection first
}

type sqlBrowser struct {
	db   *sql.DB
	d    dialect
	name string
	own  bool // close the pool with the browser
}

func (b *sqlBrowser) Close() error {
	if b.own {
		return b.db.Close()
	}
	return nil
}

func (b *sqlBrowser) Tables(ctx context.Context) ([]TableInfo, error) {
	rows, err := b.db.QueryContext(ctx, b.d.tablesSQL, b.d.tableArgs(b.name)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TableInfo{}
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.Type, &t.Rows, &t.Size); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (b *sqlBrowser) Columns(ctx context.Context, table string) ([]Column, error) {
	rows, err := b.db.QueryContext(ctx, b.d.columnsSQL, b.d.columnArgs(b.name, table)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Column{}
	for rows.Next() {
		var c Column
		var def sql.NullString
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Primary, &def, &c.Extra); err != nil {
			return nil, err
		}
		if def.Valid {
			c.Default = &def.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// cols returns the catalog columns of an existing table; this is what makes the
// table name safe to quote into a statement.
func (b *sqlBrowser) cols(ctx context.Context, table string) ([]Column, error) {
	cols, err := b.Columns(ctx, table)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, ErrNotFound
	}
	return cols, nil
}

func hasCol(cols []Column, name string) bool {
	for _, c := range cols {
		if c.Name == name {
			return true
		}
	}
	return false
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (b *sqlBrowser) Select(ctx context.Context, table string, o SelectOpts) (*Page, error) {
	cols, err := b.cols(ctx, table)
	if err != nil {
		return nil, err
	}
	d := b.d
	q := d.qualify(b.name, table)
	where, args := "", []any{}
	if s := strings.TrimSpace(o.Search); s != "" {
		parts := make([]string, 0, len(cols))
		for _, c := range cols {
			args = append(args, "%"+escapeLike(s)+"%")
			parts = append(parts, d.castText(d.quote(c.Name))+" "+d.like+" "+d.ph(len(args)))
		}
		where = " WHERE " + strings.Join(parts, " OR ")
	}
	order := ""
	if hasCol(cols, o.Order) {
		order = " ORDER BY " + d.quote(o.Order)
		if o.Desc {
			order += " DESC"
		}
	} else {
		var pk []string
		for _, c := range cols {
			if c.Primary {
				pk = append(pk, d.quote(c.Name))
			}
		}
		if len(pk) > 0 { // stable paging
			order = " ORDER BY " + strings.Join(pk, ", ")
		}
	}
	limit := o.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	p := &Page{}
	if err := b.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+q+where, args...).Scan(&p.Total); err != nil {
		return nil, err
	}
	rows, err := b.db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s%s%s LIMIT %d OFFSET %d", q, where, order, limit, o.Offset), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	p.Columns, p.Rows, _, err = scanRows(rows, limit)
	return p, err
}

func (b *sqlBrowser) Insert(ctx context.Context, table string, vals map[string]*string) error {
	cols, err := b.cols(ctx, table)
	if err != nil {
		return err
	}
	names, phs, args, err := b.assign(cols, vals, 0)
	if err != nil {
		return err
	}
	_, err = b.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", b.d.qualify(b.name, table),
		strings.Join(names, ", "), strings.Join(phs, ", ")), args...)
	return err
}

// assign validates value keys against the catalog and returns quoted names,
// placeholders and args in a stable (column) order.
func (b *sqlBrowser) assign(cols []Column, vals map[string]*string, start int) (names, phs []string, args []any, err error) {
	for k := range vals {
		if !hasCol(cols, k) {
			return nil, nil, nil, fmt.Errorf("%w: unknown column %q", ErrBadInput, k)
		}
	}
	for _, c := range cols {
		v, ok := vals[c.Name]
		if !ok {
			continue
		}
		names = append(names, b.d.quote(c.Name))
		phs = append(phs, b.d.ph(start+len(args)+1))
		if v == nil {
			args = append(args, nil)
		} else {
			args = append(args, *v)
		}
	}
	if len(names) == 0 {
		return nil, nil, nil, fmt.Errorf("%w: no values", ErrBadInput)
	}
	return names, phs, args, nil
}

// pkWhere requires pk to name exactly the primary key columns.
func (b *sqlBrowser) pkWhere(cols []Column, pk map[string]string, start int) (string, []any, error) {
	var parts []string
	var args []any
	n := 0
	for _, c := range cols {
		if !c.Primary {
			continue
		}
		n++
		v, ok := pk[c.Name]
		if !ok {
			return "", nil, fmt.Errorf("%w: missing primary key %q", ErrBadInput, c.Name)
		}
		args = append(args, v)
		parts = append(parts, b.d.quote(c.Name)+" = "+b.d.ph(start+len(args)))
	}
	if n == 0 {
		return "", nil, fmt.Errorf("%w: table has no primary key, use the SQL tab", ErrBadInput)
	}
	if len(pk) != n {
		return "", nil, fmt.Errorf("%w: invalid primary key", ErrBadInput)
	}
	return strings.Join(parts, " AND "), args, nil
}

func (b *sqlBrowser) Update(ctx context.Context, table string, pk map[string]string, vals map[string]*string) (int64, error) {
	cols, err := b.cols(ctx, table)
	if err != nil {
		return 0, err
	}
	names, phs, args, err := b.assign(cols, vals, 0)
	if err != nil {
		return 0, err
	}
	where, wargs, err := b.pkWhere(cols, pk, len(args))
	if err != nil {
		return 0, err
	}
	sets := make([]string, len(names))
	for i := range names {
		sets[i] = names[i] + " = " + phs[i]
	}
	res, err := b.db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s WHERE %s", b.d.qualify(b.name, table), strings.Join(sets, ", "), where), append(args, wargs...)...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (b *sqlBrowser) Delete(ctx context.Context, table string, pk map[string]string) (int64, error) {
	cols, err := b.cols(ctx, table)
	if err != nil {
		return 0, err
	}
	where, args, err := b.pkWhere(cols, pk, 0)
	if err != nil {
		return 0, err
	}
	res, err := b.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", b.d.qualify(b.name, table), where), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (b *sqlBrowser) Truncate(ctx context.Context, table string) error {
	if _, err := b.cols(ctx, table); err != nil {
		return err
	}
	_, err := b.db.ExecContext(ctx, "TRUNCATE TABLE "+b.d.qualify(b.name, table))
	return err
}

func (b *sqlBrowser) Drop(ctx context.Context, table string) error {
	if _, err := b.cols(ctx, table); err != nil {
		return err
	}
	kind := "TABLE"
	tables, err := b.Tables(ctx)
	if err != nil {
		return err
	}
	for _, t := range tables {
		if t.Name == table && t.Type == "VIEW" {
			kind = "VIEW"
		}
	}
	_, err = b.db.ExecContext(ctx, "DROP "+kind+" "+b.d.qualify(b.name, table))
	return err
}

var queryVerbs = map[string]bool{"select": true, "show": true, "describe": true, "desc": true, "explain": true, "with": true, "values": true, "table": true}

// isQuery reports whether the statement returns a result set, skipping leading comments.
func isQuery(stmt string) bool {
	s := strings.TrimSpace(stmt)
	for {
		switch {
		case strings.HasPrefix(s, "--"), strings.HasPrefix(s, "#"):
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				return false
			}
			s = strings.TrimSpace(s[i+1:])
		case strings.HasPrefix(s, "/*"):
			i := strings.Index(s, "*/")
			if i < 0 {
				return false
			}
			s = strings.TrimSpace(s[i+2:])
		default:
			end := strings.IndexFunc(s, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') })
			if end < 0 {
				end = len(s)
			}
			return queryVerbs[strings.ToLower(s[:end])]
		}
	}
}

func (b *sqlBrowser) Exec(ctx context.Context, stmt string) (*QueryResult, error) {
	stmt = strings.TrimSpace(stmt)
	if stmt == "" {
		return nil, fmt.Errorf("%w: empty statement", ErrBadInput)
	}
	conn, err := b.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if b.d.use != nil {
		if _, err := conn.ExecContext(ctx, b.d.use(b.name)); err != nil {
			return nil, err
		}
	}
	start := time.Now()
	res := &QueryResult{IsQuery: isQuery(stmt)}
	if res.IsQuery {
		rows, err := conn.QueryContext(ctx, stmt)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		res.Columns, res.Rows, res.Truncated, err = scanRows(rows, maxQueryRow)
		if err != nil {
			return nil, err
		}
	} else {
		r, err := conn.ExecContext(ctx, stmt)
		if err != nil {
			return nil, err
		}
		res.Affected, _ = r.RowsAffected()
	}
	res.ElapsedMS = time.Since(start).Milliseconds()
	return res, nil
}

// scanRows reads up to max rows; truncated reports that more were available.
func scanRows(rows *sql.Rows, max int) ([]string, [][]any, bool, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, false, err
	}
	out := [][]any{}
	for rows.Next() {
		if len(out) >= max {
			return cols, out, true, nil
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, false, err
		}
		for i := range vals {
			vals[i] = normalize(vals[i])
		}
		out = append(out, vals)
	}
	return cols, out, false, rows.Err()
}

// normalize turns driver values into JSON-safe ones: binary becomes hex, times
// text, and integers beyond 2^53 strings so the browser cannot round them.
func normalize(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		if utf8.Valid(x) {
			return clip(string(x))
		}
		if len(x) > 256 {
			return "0x" + hex.EncodeToString(x[:256]) + "…"
		}
		return "0x" + hex.EncodeToString(x)
	case string:
		return clip(x)
	case time.Time:
		return x.Format("2006-01-02 15:04:05.999999")
	case int64:
		if x > 1<<53 || x < -(1<<53) {
			return fmt.Sprint(x)
		}
		return x
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Sprint(x)
		}
		return x
	case bool:
		return x
	case map[string]any, []any:
		j, _ := json.Marshal(x)
		return clip(string(j))
	default:
		return clip(fmt.Sprint(x))
	}
}

func clip(s string) string {
	if len(s) <= maxCellSize {
		return s
	}
	i := maxCellSize
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i] + "…"
}
