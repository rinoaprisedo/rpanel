package databases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"rpanel/backend/system"

	"gorm.io/gorm"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrExists    = errors.New("already exists")
	serverNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	backupFileRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_]+\.[0-9]{8}-[0-9]{6}\.sql$`)
	safeDirRe    = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)
)

// OpenFunc connects to a server; replaced in tests.
type OpenFunc func(engine, host string, port int, user, pass string) (Engine, error)

func defaultOpen(engine, host string, port int, user, pass string) (Engine, error) {
	switch engine {
	case EngineMySQL:
		return openMySQL(host, port, user, pass)
	case EnginePostgres:
		return openPostgres(host, port, user, pass)
	}
	return nil, fmt.Errorf("%w: engine", ErrBadInput)
}

type Service struct {
	db        *gorm.DB
	cipher    *Cipher // nil when DB_SECRET_KEY is unset
	run       system.Runner
	backupDir string
	open      OpenFunc
}

func NewService(db *gorm.DB, secret string, run system.Runner, backupDir string) *Service {
	c, _ := NewCipher(secret)
	return &Service{db: db, cipher: c, run: run, backupDir: filepath.Clean(backupDir), open: defaultOpen}
}

func (s *Service) needCipher() error {
	if s.cipher == nil {
		return ErrNoSecret
	}
	return nil
}

// ---- servers ----

func (s *Service) ListServers() ([]Server, error) {
	var out []Server
	return out, s.db.Order("name").Find(&out).Error
}

func (s *Service) server(uuid string) (*Server, error) {
	var sv Server
	if err := s.db.Where("uuid = ?", uuid).First(&sv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sv, nil
}

func (s *Service) password(sv *Server) (string, error) {
	if err := s.needCipher(); err != nil {
		return "", err
	}
	return s.cipher.Decrypt(sv.AdminPassE)
}

// connect opens an engine for the server with a decrypted password.
func (s *Service) connect(sv *Server) (Engine, string, error) {
	pw, err := s.password(sv)
	if err != nil {
		return nil, "", err
	}
	e, err := s.open(sv.Engine, sv.Host, sv.Port, sv.AdminUser, pw)
	return e, pw, err
}

func (s *Service) withEngine(ctx context.Context, uuid string, fn func(Engine, *Server) error) error {
	sv, err := s.server(uuid)
	if err != nil {
		return err
	}
	e, _, err := s.connect(sv)
	if err != nil {
		return err
	}
	defer e.Close()
	return fn(e, sv)
}

type ServerInput struct {
	Name          string  `json:"name"`
	Engine        string  `json:"engine"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	AdminUser     string  `json:"admin_user"`
	AdminPassword string  `json:"admin_password"`
	ActorID       *uint64 `json:"-"`
}

// AddServer verifies the credentials by connecting, then stores them encrypted.
func (s *Service) AddServer(ctx context.Context, in ServerInput) (*Server, error) {
	if err := s.needCipher(); err != nil {
		return nil, err
	}
	if !serverNameRe.MatchString(in.Name) {
		return nil, fmt.Errorf("%w: name", ErrBadInput)
	}
	if in.Engine != EngineMySQL && in.Engine != EnginePostgres {
		return nil, fmt.Errorf("%w: engine", ErrBadInput)
	}
	if !ValidHost(in.Host) || in.Host == "%" || in.Port < 1 || in.Port > 65535 || in.AdminUser == "" || in.AdminPassword == "" {
		return nil, fmt.Errorf("%w: host, port or credentials", ErrBadInput)
	}
	var n int64
	s.db.Model(&Server{}).Where("name = ?", in.Name).Count(&n)
	if n > 0 {
		return nil, ErrExists
	}
	e, err := s.open(in.Engine, in.Host, in.Port, in.AdminUser, in.AdminPassword)
	if err != nil {
		return nil, err
	}
	defer e.Close()
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := e.Ping(pctx); err != nil {
		return nil, fmt.Errorf("cannot connect: %w", err)
	}
	enc, err := s.cipher.Encrypt(in.AdminPassword)
	if err != nil {
		return nil, err
	}
	sv := &Server{Name: in.Name, Engine: in.Engine, Host: in.Host, Port: in.Port, AdminUser: in.AdminUser, AdminPassE: enc}
	sv.CreatedBy = in.ActorID
	return sv, s.db.Create(sv).Error
}

func (s *Service) DeleteServer(uuid string) (*Server, error) {
	sv, err := s.server(uuid)
	if err != nil {
		return nil, err
	}
	return sv, s.db.Unscoped().Delete(sv).Error // only forgets the connection; the server is untouched
}

// ---- databases ----

func (s *Service) ListDatabases(ctx context.Context, uuid string) (out []DatabaseInfo, err error) {
	err = s.withEngine(ctx, uuid, func(e Engine, _ *Server) (err error) {
		out, err = e.ListDatabases(ctx)
		return
	})
	return
}

type CreateDBInput struct {
	Name     string `json:"name"`
	User     string `json:"user"`     // optional: also create/grant this user
	Host     string `json:"host"`     // MySQL only, default localhost
	Password string `json:"password"` // required when user is set
}

func (s *Service) CreateDatabase(ctx context.Context, uuid string, in CreateDBInput) error {
	if !ValidName(in.Name) || IsSystemDB(in.Name) {
		return fmt.Errorf("%w: database name", ErrBadInput)
	}
	if in.Host == "" {
		in.Host = "localhost"
	}
	if in.User != "" {
		if !ValidUser(in.User) || IsSystemUser(in.User) || !ValidHost(in.Host) {
			return fmt.Errorf("%w: user or host", ErrBadInput)
		}
		if err := ValidPassword(in.Password); err != nil {
			return err
		}
	}
	return s.withEngine(ctx, uuid, func(e Engine, sv *Server) error {
		owner := ""
		if in.User != "" {
			if err := e.CreateUser(ctx, in.User, in.Host, in.Password); err != nil {
				return fmt.Errorf("create user: %w", err)
			}
			owner = in.User
		}
		if err := e.CreateDatabase(ctx, in.Name, owner); err != nil {
			if in.User != "" { // do not leave a half-created user behind
				_ = e.DropUser(ctx, in.User, in.Host)
			}
			return err
		}
		if in.User != "" && sv.Engine == EngineMySQL {
			if err := e.Grant(ctx, in.Name, in.User, in.Host); err != nil {
				return fmt.Errorf("grant: %w", err)
			}
		}
		return nil
	})
}

func (s *Service) DropDatabase(ctx context.Context, uuid, name string) error {
	if !ValidName(name) {
		return fmt.Errorf("%w: database name", ErrBadInput)
	}
	if IsSystemDB(name) {
		return ErrProtected
	}
	return s.withEngine(ctx, uuid, func(e Engine, _ *Server) error { return e.DropDatabase(ctx, name) })
}

// ---- users ----

func (s *Service) ListUsers(ctx context.Context, uuid string) (out []UserInfo, err error) {
	err = s.withEngine(ctx, uuid, func(e Engine, _ *Server) (err error) {
		out, err = e.ListUsers(ctx)
		return
	})
	return
}

type UserInput struct {
	User     string `json:"user"`
	Host     string `json:"host"`
	Password string `json:"password"`
	Database string `json:"database"` // for grant
}

func (u *UserInput) normalize() error {
	if u.Host == "" {
		u.Host = "localhost"
	}
	if !ValidUser(u.User) || !ValidHost(u.Host) {
		return fmt.Errorf("%w: user or host", ErrBadInput)
	}
	return nil
}

func (s *Service) CreateUser(ctx context.Context, uuid string, in UserInput) error {
	if err := in.normalize(); err != nil {
		return err
	}
	if IsSystemUser(in.User) {
		return ErrProtected
	}
	if err := ValidPassword(in.Password); err != nil {
		return err
	}
	return s.withEngine(ctx, uuid, func(e Engine, _ *Server) error { return e.CreateUser(ctx, in.User, in.Host, in.Password) })
}

func (s *Service) DropUser(ctx context.Context, uuid string, in UserInput) error {
	if err := in.normalize(); err != nil {
		return err
	}
	return s.withEngine(ctx, uuid, func(e Engine, sv *Server) error {
		if IsSystemUser(in.User) || strings.EqualFold(in.User, sv.AdminUser) {
			return ErrProtected
		}
		return e.DropUser(ctx, in.User, in.Host)
	})
}

func (s *Service) SetPassword(ctx context.Context, uuid string, in UserInput) error {
	if err := in.normalize(); err != nil {
		return err
	}
	if err := ValidPassword(in.Password); err != nil {
		return err
	}
	return s.withEngine(ctx, uuid, func(e Engine, sv *Server) error {
		if IsSystemUser(in.User) {
			return ErrProtected
		}
		if err := e.SetPassword(ctx, in.User, in.Host, in.Password); err != nil {
			return err
		}
		return nil
	})
}

func (s *Service) Grant(ctx context.Context, uuid string, in UserInput) error {
	if err := in.normalize(); err != nil {
		return err
	}
	if !ValidName(in.Database) || IsSystemDB(in.Database) {
		return fmt.Errorf("%w: database name", ErrBadInput)
	}
	return s.withEngine(ctx, uuid, func(e Engine, _ *Server) error { return e.Grant(ctx, in.Database, in.User, in.Host) })
}

// ---- backups ----

type Backup struct {
	File     string    `json:"file"`
	Database string    `json:"database"`
	Size     int64     `json:"size"`
	Created  time.Time `json:"created"`
}

func (s *Service) prepareBackupDir() error {
	if !safeDirRe.MatchString(s.backupDir) {
		return fmt.Errorf("%w: BACKUP_DIR contains unsafe characters", ErrBadInput)
	}
	return os.MkdirAll(s.backupDir, 0o700)
}

func (s *Service) connArgs(sv *Server) []string {
	port := strconv.Itoa(sv.Port)
	if sv.Engine == EngineMySQL {
		return []string{"--protocol=tcp", "-h", sv.Host, "-P", port, "-u", sv.AdminUser}
	}
	return []string{"-h", sv.Host, "-p", port, "-U", sv.AdminUser, "-w"}
}

func (s *Service) secretEnv(sv *Server, pw string) []string {
	if sv.Engine == EngineMySQL {
		return []string{"MYSQL_PWD=" + pw}
	}
	return []string{"PGPASSWORD=" + pw}
}

func (s *Service) CreateBackup(ctx context.Context, uuid, dbName string) (*Backup, error) {
	if !ValidName(dbName) || IsSystemDB(dbName) {
		return nil, fmt.Errorf("%w: database name", ErrBadInput)
	}
	sv, err := s.server(uuid)
	if err != nil {
		return nil, err
	}
	if err := s.prepareBackupDir(); err != nil {
		return nil, err
	}
	pw, err := s.password(sv)
	if err != nil {
		return nil, err
	}
	file := fmt.Sprintf("%s.%s.%s.sql", sv.Name, dbName, time.Now().Format("20060102-150405"))
	path := filepath.Join(s.backupDir, file)
	args := s.connArgs(sv)
	var cmd string
	if sv.Engine == EngineMySQL {
		cmd = "mysqldump"
		args = append(args, "--single-transaction", "--routines", "--triggers", "--result-file="+path, dbName)
	} else {
		cmd = "pg_dump"
		args = append(args, "-F", "p", "-f", path, dbName)
	}
	res, err := system.RunEnv(ctx, s.run, 30*time.Minute, s.secretEnv(sv, pw), cmd, args...)
	if err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("backup failed: %s", tail(res.Stderr))
	}
	_ = os.Chmod(path, 0o600)
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("backup produced no file")
	}
	return &Backup{File: file, Database: dbName, Size: st.Size(), Created: st.ModTime()}, nil
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = s[len(s)-400:]
	}
	return s
}

func (s *Service) ListBackups(uuid string) ([]Backup, error) {
	sv, err := s.server(uuid)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(s.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Backup{}, nil
		}
		return nil, err
	}
	out := []Backup{}
	for _, it := range items {
		n := it.Name()
		if !backupFileRe.MatchString(n) || !strings.HasPrefix(n, sv.Name+".") {
			continue
		}
		parts := strings.Split(n, ".")
		if len(parts) != 4 || parts[0] != sv.Name {
			continue
		}
		info, err := it.Info()
		if err != nil {
			continue
		}
		out = append(out, Backup{File: n, Database: parts[1], Size: info.Size(), Created: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// backupPath validates a file name belongs to the server and returns its path.
func (s *Service) backupPath(sv *Server, file string) (string, string, error) {
	parts := strings.Split(file, ".")
	if !backupFileRe.MatchString(file) || len(parts) != 4 || parts[0] != sv.Name {
		return "", "", fmt.Errorf("%w: backup file", ErrBadInput)
	}
	p := filepath.Join(s.backupDir, file)
	if _, err := os.Stat(p); err != nil {
		return "", "", ErrNotFound
	}
	return p, parts[1], nil
}

// Restore loads a backup into targetDB (default: the database it was taken from).
func (s *Service) Restore(ctx context.Context, uuid, file, targetDB string) error {
	sv, err := s.server(uuid)
	if err != nil {
		return err
	}
	path, srcDB, err := s.backupPath(sv, file)
	if err != nil {
		return err
	}
	if targetDB == "" {
		targetDB = srcDB
	}
	if !ValidName(targetDB) || IsSystemDB(targetDB) {
		return fmt.Errorf("%w: database name", ErrBadInput)
	}
	pw, err := s.password(sv)
	if err != nil {
		return err
	}
	args := s.connArgs(sv)
	var cmd string
	if sv.Engine == EngineMySQL {
		cmd = "mysql"
		args = append(args, "--default-character-set=utf8mb4", targetDB, "-e", "source "+path)
	} else {
		cmd = "psql"
		args = append(args, "-d", targetDB, "-v", "ON_ERROR_STOP=1", "-q", "-f", path)
	}
	res, err := system.RunEnv(ctx, s.run, 30*time.Minute, s.secretEnv(sv, pw), cmd, args...)
	if err != nil {
		return fmt.Errorf("restore failed: %s", tail(res.Stderr))
	}
	return nil
}

func (s *Service) DeleteBackup(uuid, file string) error {
	sv, err := s.server(uuid)
	if err != nil {
		return err
	}
	p, _, err := s.backupPath(sv, file)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// BackupFilePath returns the validated path for download.
func (s *Service) BackupFilePath(uuid, file string) (string, error) {
	sv, err := s.server(uuid)
	if err != nil {
		return "", err
	}
	p, _, err := s.backupPath(sv, file)
	return p, err
}

// ---- local server installation ----

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// InstallLocal installs MariaDB or PostgreSQL via apt, provisions a dedicated
// "rpanel" admin account with a random password, and registers the server.
func (s *Service) InstallLocal(ctx context.Context, engine string) (*Server, error) {
	if err := s.needCipher(); err != nil {
		return nil, err
	}
	pkg, unit, name, port := "", "", "", 0
	switch engine {
	case EngineMySQL:
		pkg, unit, name, port = "mariadb-server", "mariadb", "local-mysql", 3306
	case EnginePostgres:
		pkg, unit, name, port = "postgresql", "postgresql", "local-postgres", 5432
	default:
		return nil, fmt.Errorf("%w: engine", ErrBadInput)
	}
	if res, err := s.run.Run(ctx, 5*time.Minute, "apt-get", "update"); err != nil {
		return nil, fmt.Errorf("apt-get update failed: %s", tail(res.Stderr))
	}
	if res, err := s.run.Run(ctx, 20*time.Minute, "apt-get", "install", "-y", pkg); err != nil {
		return nil, fmt.Errorf("install failed: %s", tail(res.Stderr))
	}
	if res, err := s.run.Run(ctx, time.Minute, "systemctl", "enable", "--now", unit); err != nil {
		return nil, fmt.Errorf("cannot start %s: %s", unit, tail(res.Stderr))
	}
	pw, err := randomHex(24) // hex only: safe to embed in the provisioning statements below
	if err != nil {
		return nil, err
	}
	const admin = "rpanel"
	if engine == EngineMySQL {
		sql := ""
		for _, h := range []string{"localhost", "127.0.0.1"} {
			sql += fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'; ALTER USER '%s'@'%s' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON *.* TO '%s'@'%s' WITH GRANT OPTION; ",
				admin, h, pw, admin, h, pw, admin, h)
		}
		sql += "FLUSH PRIVILEGES;"
		if res, err := s.run.Run(ctx, time.Minute, "mysql", "-e", sql); err != nil {
			return nil, fmt.Errorf("cannot provision admin user: %s", tail(res.Stderr))
		}
	} else {
		create := fmt.Sprintf("CREATE ROLE %s SUPERUSER LOGIN PASSWORD '%s'", admin, pw)
		alter := fmt.Sprintf("ALTER ROLE %s SUPERUSER LOGIN PASSWORD '%s'", admin, pw)
		_, _ = s.run.Run(ctx, time.Minute, "runuser", "-u", "postgres", "--", "psql", "-c", create) // fails harmlessly if it exists
		if res, err := s.run.Run(ctx, time.Minute, "runuser", "-u", "postgres", "--", "psql", "-v", "ON_ERROR_STOP=1", "-c", alter); err != nil {
			return nil, fmt.Errorf("cannot provision admin role: %s", tail(res.Stderr))
		}
	}
	e, err := s.open(engine, "127.0.0.1", port, admin, pw)
	if err != nil {
		return nil, err
	}
	defer e.Close()
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := e.Ping(pctx); err != nil {
		return nil, fmt.Errorf("installed but cannot connect: %w", err)
	}
	enc, err := s.cipher.Encrypt(pw)
	if err != nil {
		return nil, err
	}
	var sv Server
	if err := s.db.Where("name = ?", name).First(&sv).Error; err == nil {
		sv.AdminPassE, sv.AdminUser, sv.Host, sv.Port = enc, admin, "127.0.0.1", port
		return &sv, s.db.Save(&sv).Error
	}
	sv = Server{Name: name, Engine: engine, Host: "127.0.0.1", Port: port, AdminUser: admin, AdminPassE: enc, Local: true}
	return &sv, s.db.Create(&sv).Error
}
