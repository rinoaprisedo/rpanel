package databases

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"strconv"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx"
)

type pgEngine struct{ db *sql.DB }

func openPostgres(host string, port int, user, pass string) (Engine, error) {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, pass),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/postgres",
		RawQuery: "sslmode=prefer&connect_timeout=5",
	}
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	return &pgEngine{db: db}, nil
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
