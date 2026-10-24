package databases

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx"
)

type pgEngine struct {
	db         *sql.DB
	host       string
	port       int
	user, pass string
}

func openPostgres(host string, port int, user, pass string) (Engine, error) {
	db, err := openPostgresDB(host, port, user, pass, "postgres")
	if err != nil {
		return nil, err
	}
	return &pgEngine{db: db, host: host, port: port, user: user, pass: pass}, nil
}

func openPostgresDB(host string, port int, user, pass, dbname string) (*sql.DB, error) {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, pass),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + dbname,
		RawQuery: "sslmode=prefer&connect_timeout=5",
	}
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	return db, nil
}

func (p *pgEngine) Close() error                   { return p.db.Close() }
func (p *pgEngine) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

// exec builds a statement server-side with format(%I/%L) so identifiers and
// literals are quoted by PostgreSQL itself, then runs the resulting text.
func (p *pgEngine) exec(ctx context.Context, format string, args ...any) error {
	var stmt string
	q := "SELECT format($1" + placeholders(len(args)) + ")"
	all := append([]any{format}, args...)
	if err := p.db.QueryRowContext(ctx, q, all...).Scan(&stmt); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx, stmt)
	return err
}

func placeholders(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += ", $" + strconv.Itoa(i+2) + "::text"
	}
	return s
}

func (p *pgEngine) ListDatabases(ctx context.Context) ([]DatabaseInfo, error) {
	rows, err := p.db.QueryContext(ctx, "SELECT datname, pg_database_size(datname) FROM pg_database WHERE NOT datistemplate ORDER BY datname")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseInfo{}
	for rows.Next() {
		var d DatabaseInfo
		if err := rows.Scan(&d.Name, &d.Size); err != nil {
			return nil, err
		}
		if !IsSystemDB(d.Name) {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

func (p *pgEngine) CreateDatabase(ctx context.Context, name, owner string) error {
	if owner != "" {
		return p.exec(ctx, "CREATE DATABASE %I OWNER %I ENCODING 'UTF8' TEMPLATE template0", name, owner)
	}
	return p.exec(ctx, "CREATE DATABASE %I ENCODING 'UTF8' TEMPLATE template0", name)
}

func (p *pgEngine) DropDatabase(ctx context.Context, name string) error {
	return p.exec(ctx, "DROP DATABASE %I", name)
}

func (p *pgEngine) ListUsers(ctx context.Context) ([]UserInfo, error) {
	rows, err := p.db.QueryContext(ctx, "SELECT rolname FROM pg_roles WHERE rolname !~ '^pg_' ORDER BY rolname")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserInfo{}
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.Name); err != nil {
			return nil, err
		}
		if !IsSystemUser(u.Name) {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func (p *pgEngine) CreateUser(ctx context.Context, user, _, password string) error {
	return p.exec(ctx, "CREATE ROLE %I LOGIN PASSWORD %L", user, password)
}

func (p *pgEngine) DropUser(ctx context.Context, user, _ string) error {
	return p.exec(ctx, "DROP ROLE %I", user)
}

func (p *pgEngine) SetPassword(ctx context.Context, user, _, password string) error {
	return p.exec(ctx, "ALTER ROLE %I PASSWORD %L", user, password)
}

func (p *pgEngine) Grant(ctx context.Context, db, user, _ string) error {
	return p.exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE %I TO %I", db, user)
}

func pgQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

var pgDialect = dialect{
	quote:    pgQuote,
	qualify:  func(_, t string) string { return `"public".` + pgQuote(t) },
	ph:       func(i int) string { return "$" + strconv.Itoa(i) },
	castText: func(c string) string { return c + "::text" },
	like:     "ILIKE",
	tablesSQL: `SELECT c.relname, CASE WHEN c.relkind IN ('v','m') THEN 'VIEW' ELSE 'BASE TABLE' END,
		GREATEST(c.reltuples, 0)::bigint, CASE WHEN c.relkind IN ('r','p','m') THEN pg_total_relation_size(c.oid) ELSE 0 END
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r','p','v','m') ORDER BY c.relname`,
	tableArgs: func(string) []any { return nil },
	columnsSQL: `SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull,
		EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = a.attrelid AND i.indisprimary AND a.attnum = ANY(i.indkey)),
		pg_get_expr(d.adbin, d.adrelid), COALESCE(NULLIF(a.attidentity::text, ''), '')
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE n.nspname = 'public' AND c.relname = $1 AND c.relkind IN ('r','p','v','m') AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`,
	columnArgs: func(_, t string) []any { return []any{t} },
}

// Browse opens a dedicated pool: PostgreSQL connections are bound to one database.
func (p *pgEngine) Browse(_ context.Context, db string) (Browser, error) {
	conn, err := openPostgresDB(p.host, p.port, p.user, p.pass, db)
	if err != nil {
		return nil, err
	}
	return &sqlBrowser{db: conn, d: pgDialect, name: db, own: true}, nil
}
