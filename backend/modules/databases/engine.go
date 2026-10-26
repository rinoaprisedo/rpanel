package databases

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"rpanel/backend/system"
)

type DatabaseInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type UserInfo struct {
	Name string `json:"name"`
	Host string `json:"host"` // empty for PostgreSQL roles
}

// Engine is the set of administrative operations against one server.
// Identifiers are validated by the caller (ValidIdent); passwords are always
// passed as values (parameters / server-side quoting), never concatenated.
type Engine interface {
	Close() error
	Ping(ctx context.Context) error
	ListDatabases(ctx context.Context) ([]DatabaseInfo, error)
	CreateDatabase(ctx context.Context, name, owner string) error
	DropDatabase(ctx context.Context, name string) error
	ListUsers(ctx context.Context) ([]UserInfo, error)
	CreateUser(ctx context.Context, user, host, password string) error
	DropUser(ctx context.Context, user, host string) error
	SetPassword(ctx context.Context, user, host, password string) error
	Grant(ctx context.Context, db, user, host string) error
}

var (
	ErrBadInput  = errors.New("invalid input")
	ErrProtected = errors.New("system objects cannot be modified")
	hostRe       = regexp.MustCompile(`^(%|localhost|[A-Za-z0-9.-]{1,253}|[0-9a-fA-F:]{2,45})$`)
)

var (
	systemDBs = map[string]bool{
		"mysql": true, "information_schema": true, "performance_schema": true, "sys": true,
		"postgres": true, "template0": true, "template1": true,
	}
	systemUsers = map[string]bool{
		"root": true, "mysql": true, "mariadb.sys": true, "mysql.sys": true, "mysql.session": true,
		"mysql.infoschema": true, "postgres": true,
	}
)

// ProtectPanelDatabase hides the panel's own database and role from this module
// (listing, browsing, SQL, backups, drop, grants): they hold password hashes,
// 2FA secrets and session data, and dropping them would take the panel down.
// Called once at start-up, before any request is served.
func ProtectPanelDatabase(dbName, dbUser string) {
	if dbName != "" {
		systemDBs[strings.ToLower(dbName)] = true
	}
	if dbUser != "" {
		systemUsers[strings.ToLower(dbUser)] = true
	}
}

func IsSystemDB(n string) bool   { return systemDBs[strings.ToLower(n)] }
func IsSystemUser(n string) bool { return systemUsers[strings.ToLower(n)] }

// ValidName allows letters, digits and underscore (1..64), starting with a letter or underscore.
func ValidName(n string) bool { return system.ValidIdent(n) }

// ValidUserName also allows the dot/dash common in DB user names but keeps quoting-safe characters only.
var userRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,31}$`)

func ValidUser(n string) bool { return userRe.MatchString(n) }

func ValidHost(h string) bool { return hostRe.MatchString(h) }

// ValidPassword enforces a minimal length and forbids NUL/newlines.
func ValidPassword(p string) error {
	if len(p) < 8 || len(p) > 128 {
		return fmt.Errorf("%w: password must be 8-128 characters", ErrBadInput)
	}
	if strings.ContainsAny(p, "\x00\r\n") {
		return fmt.Errorf("%w: password contains illegal characters", ErrBadInput)
	}
	return nil
}
