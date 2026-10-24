package databases

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type mysqlEngine struct{ db *sql.DB }

func openMySQL(host string, port int, user, pass string) (Engine, error) {
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = user, pass
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	cfg.InterpolateParams = true // client-side escaping: CREATE USER/GRANT cannot be prepared server-side
	cfg.Timeout = 5e9
	cfg.ReadTimeout = 60e9
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	return &mysqlEngine{db: db}, nil
}

func (m *mysqlEngine) Close() error                   { return m.db.Close() }
func (m *mysqlEngine) Ping(ctx context.Context) error { return m.db.PingContext(ctx) }

func qi(s string) string { return "`" + s + "`" } // s is validated: no backticks possible

func (m *mysqlEngine) ListDatabases(ctx context.Context) ([]DatabaseInfo, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT s.schema_name, CAST(COALESCE(SUM(t.data_length + t.index_length), 0) AS SIGNED)
		FROM information_schema.schemata s
		LEFT JOIN information_schema.tables t ON t.table_schema = s.schema_name
		GROUP BY s.schema_name ORDER BY s.schema_name`)
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

func (m *mysqlEngine) CreateDatabase(ctx context.Context, name, _ string) error {
	_, err := m.db.ExecContext(ctx, "CREATE DATABASE "+qi(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func (m *mysqlEngine) DropDatabase(ctx context.Context, name string) error {
	_, err := m.db.ExecContext(ctx, "DROP DATABASE "+qi(name))
	return err
}

func (m *mysqlEngine) ListUsers(ctx context.Context) ([]UserInfo, error) {
	rows, err := m.db.QueryContext(ctx, "SELECT User, Host FROM mysql.user WHERE User <> '' ORDER BY User, Host")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserInfo{}
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.Name, &u.Host); err != nil {
			return nil, err
		}
		if !IsSystemUser(u.Name) {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func (m *mysqlEngine) CreateUser(ctx context.Context, user, host, password string) error {
	_, err := m.db.ExecContext(ctx, "CREATE USER ?@? IDENTIFIED BY ?", user, host, password)
	return err
}

func (m *mysqlEngine) DropUser(ctx context.Context, user, host string) error {
	_, err := m.db.ExecContext(ctx, "DROP USER ?@?", user, host)
	return err
}

func (m *mysqlEngine) SetPassword(ctx context.Context, user, host, password string) error {
	_, err := m.db.ExecContext(ctx, "ALTER USER ?@? IDENTIFIED BY ?", user, host, password)
	return err
}

func (m *mysqlEngine) Grant(ctx context.Context, db, user, host string) error {
	_, err := m.db.ExecContext(ctx, fmt.Sprintf("GRANT ALL PRIVILEGES ON %s.* TO ?@?", qi(db)), user, host)
	return err
}

var mysqlDialect = dialect{
	quote: func(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" },
	qualify: func(db, t string) string {
		return qi(strings.ReplaceAll(db, "`", "``")) + "." + qi(strings.ReplaceAll(t, "`", "``"))
	},
	ph:       func(int) string { return "?" },
	castText: func(c string) string { return "CAST(" + c + " AS CHAR)" },
	like:     "LIKE",
	tablesSQL: `SELECT table_name, CASE WHEN table_type = 'VIEW' THEN 'VIEW' ELSE 'BASE TABLE' END,
		CAST(COALESCE(table_rows, 0) AS SIGNED), CAST(COALESCE(data_length + index_length, 0) AS SIGNED)
		FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name`,
	tableArgs: func(db string) []any { return []any{db} },
	columnsSQL: `SELECT column_name, column_type, is_nullable = 'YES', column_key = 'PRI', column_default, extra
		FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`,
	columnArgs: func(db, t string) []any { return []any{db, t} },
	use:        func(db string) string { return "USE " + qi(strings.ReplaceAll(db, "`", "``")) },
}

func (m *mysqlEngine) Browse(_ context.Context, db string) (Browser, error) {
	return &sqlBrowser{db: m.db, d: mysqlDialect, name: db}, nil
}
