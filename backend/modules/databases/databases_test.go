package databases

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rpanel/backend/system"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type fakeEngine struct {
	calls   []string
	failDB  bool
	pingErr error
}

func (f *fakeEngine) rec(s string)               { f.calls = append(f.calls, s) }
func (f *fakeEngine) Close() error               { return nil }
func (f *fakeEngine) Ping(context.Context) error { return f.pingErr }
func (f *fakeEngine) ListDatabases(context.Context) ([]DatabaseInfo, error) {
	return []DatabaseInfo{{Name: "app", Size: 1}}, nil
}
func (f *fakeEngine) CreateDatabase(_ context.Context, n, o string) error {
	f.rec("createdb " + n + " owner=" + o)
	if f.failDB {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeEngine) DropDatabase(_ context.Context, n string) error {
	f.rec("dropdb " + n)
	return nil
}
func (f *fakeEngine) ListUsers(context.Context) ([]UserInfo, error) { return nil, nil }
func (f *fakeEngine) CreateUser(_ context.Context, u, h, p string) error {
	f.rec("createuser " + u + "@" + h)
	return nil
}
func (f *fakeEngine) DropUser(_ context.Context, u, h string) error {
	f.rec("dropuser " + u + "@" + h)
	return nil
}
func (f *fakeEngine) SetPassword(_ context.Context, u, h, p string) error {
	f.rec("setpw " + u)
	return nil
}
func (f *fakeEngine) Grant(_ context.Context, d, u, h string) error {
	f.rec("grant " + d + " " + u)
	return nil
}

type runCall struct {
	name string
	args []string
	env  []string
}

type fakeRunner struct {
	mu    sync.Mutex
	calls []runCall
	write string // if set, create this file when a dump command runs (simulates --result-file / -f)
	fail  bool
}

func (f *fakeRunner) Run(ctx context.Context, d time.Duration, name string, args ...string) (system.Result, error) {
	return f.RunEnv(ctx, d, nil, name, args...)
}

func (f *fakeRunner) RunEnv(_ context.Context, _ time.Duration, env []string, name string, args ...string) (system.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, runCall{name, args, env})
	if f.fail {
		return system.Result{Stderr: "access denied"}, errors.New("exit 1")
	}
	if name == "mysqldump" || name == "pg_dump" {
		for _, a := range args {
			if p, ok := strings.CutPrefix(a, "--result-file="); ok {
				os.WriteFile(p, []byte("-- dump"), 0o600)
			}
		}
		for i, a := range args {
			if a == "-f" {
				os.WriteFile(args[i+1], []byte("-- dump"), 0o600)
			}
		}
	}
	return system.Result{}, nil
}

type env struct {
	svc *Service
	eng *fakeEngine
	run *fakeRunner
	dir string
}

func setup(t *testing.T, secret string) *env {
	t.Helper()
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&Server{})
	e := &env{eng: &fakeEngine{}, run: &fakeRunner{}, dir: filepath.Join(dir, "backups")}
	e.svc = NewService(db, secret, e.run, e.dir)
	e.svc.open = func(engine, host string, port int, user, pass string) (Engine, error) { return e.eng, nil }
	return e
}

func (e *env) addServer(t *testing.T, engine string) *Server {
	t.Helper()
	sv, err := e.svc.AddServer(context.Background(), ServerInput{Name: "main", Engine: engine, Host: "127.0.0.1", Port: 3306, AdminUser: "rpanel", AdminPassword: "s3cret-pass"})
	if err != nil {
		t.Fatal(err)
	}
	return sv
}

func TestCipherRoundTripAndTamper(t *testing.T) {
	c, _ := NewCipher("k")
	enc, _ := c.Encrypt("hunter22")
	if enc == "hunter22" || strings.Contains(enc, "hunter") {
		t.Fatal("not encrypted")
	}
	if got, err := c.Decrypt(enc); err != nil || got != "hunter22" {
		t.Fatalf("roundtrip: %q %v", got, err)
	}
	enc2, _ := c.Encrypt("hunter22")
	if enc == enc2 {
		t.Fatal("nonce must differ per call")
	}
	other, _ := NewCipher("other")
	if _, err := other.Decrypt(enc); err == nil {
		t.Fatal("wrong key must fail")
	}
	if _, err := c.Decrypt(enc[:len(enc)-4] + "AAAA"); err == nil {
		t.Fatal("tampered data must fail")
	}
	if _, err := NewCipher(""); !errors.Is(err, ErrNoSecret) {
		t.Fatal("empty secret must be refused")
	}
}

func TestValidators(t *testing.T) {
	for _, n := range []string{"app", "my_db1", "_x"} {
		if !ValidName(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range []string{"", "1a", "a-b", "a`b", "a;drop", "a b", strings.Repeat("a", 65)} {
		if ValidName(n) {
			t.Errorf("%q should be invalid", n)
		}
	}
	for _, h := range []string{"%", "localhost", "10.0.0.1", "db.example.com", "::1"} {
		if !ValidHost(h) {
			t.Errorf("host %q should be valid", h)
		}
	}
	for _, h := range []string{"", "a'b", "a b", "x;y", "10.0.0.%'"} {
		if ValidHost(h) {
			t.Errorf("host %q should be invalid", h)
		}
	}
	if ValidPassword("short") == nil || ValidPassword("good-enough-pw") != nil || ValidPassword("line\nbreak-123") == nil {
		t.Fatal("password validation wrong")
	}
}

func TestNoSecretRefusesServerOps(t *testing.T) {
	e := setup(t, "")
	_, err := e.svc.AddServer(context.Background(), ServerInput{Name: "main", Engine: EngineMySQL, Host: "127.0.0.1", Port: 3306, AdminUser: "u", AdminPassword: "p"})
	if !errors.Is(err, ErrNoSecret) {
		t.Fatalf("want ErrNoSecret, got %v", err)
	}
}

func TestAddServerStoresEncryptedAndVerifiesConnection(t *testing.T) {
	e := setup(t, "k")
	sv := e.addServer(t, EngineMySQL)
	if strings.Contains(sv.AdminPassE, "s3cret") || sv.AdminPassE == "" {
		t.Fatal("password must be stored encrypted")
	}
	e.eng.pingErr = errors.New("refused")
	_, err := e.svc.AddServer(context.Background(), ServerInput{Name: "other", Engine: EngineMySQL, Host: "10.0.0.5", Port: 3306, AdminUser: "u", AdminPassword: "p"})
	if err == nil {
		t.Fatal("unreachable server must be rejected")
	}
	if _, err := e.svc.AddServer(context.Background(), ServerInput{Name: "main", Engine: EngineMySQL, Host: "127.0.0.1", Port: 1, AdminUser: "u", AdminPassword: "p"}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate name: %v", err)
	}
}

func TestCreateDatabaseWithUser(t *testing.T) {
	e := setup(t, "k")
	sv := e.addServer(t, EngineMySQL)
	err := e.svc.CreateDatabase(context.Background(), sv.UUID.String(), CreateDBInput{Name: "shop", User: "shop_u", Password: "long-password-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"createuser shop_u@localhost", "createdb shop owner=shop_u", "grant shop shop_u"}
	if strings.Join(e.eng.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v", e.eng.calls)
	}
}

func TestCreateDatabaseFailureDropsCreatedUser(t *testing.T) {
	e := setup(t, "k")
	sv := e.addServer(t, EngineMySQL)
	e.eng.failDB = true
	if err := e.svc.CreateDatabase(context.Background(), sv.UUID.String(), CreateDBInput{Name: "shop", User: "shop_u", Password: "long-password-1"}); err == nil {
		t.Fatal("expected error")
	}
	if last := e.eng.calls[len(e.eng.calls)-1]; last != "dropuser shop_u@localhost" {
		t.Fatalf("user must be rolled back, calls=%v", e.eng.calls)
	}
}

func TestProtectedObjects(t *testing.T) {
	e := setup(t, "k")
	sv := e.addServer(t, EngineMySQL)
	ctx := context.Background()
	id := sv.UUID.String()
	if err := e.svc.DropDatabase(ctx, id, "mysql"); !errors.Is(err, ErrProtected) {
		t.Fatalf("drop mysql: %v", err)
	}
	if err := e.svc.CreateDatabase(ctx, id, CreateDBInput{Name: "information_schema"}); !errors.Is(err, ErrBadInput) {
		t.Fatalf("create system db: %v", err)
	}
	if err := e.svc.DropUser(ctx, id, UserInput{User: "root"}); !errors.Is(err, ErrProtected) {
		t.Fatalf("drop root: %v", err)
	}
	if err := e.svc.DropUser(ctx, id, UserInput{User: "rpanel"}); !errors.Is(err, ErrProtected) {
		t.Fatalf("drop admin account: %v", err)
	}
	if err := e.svc.DropDatabase(ctx, id, "x`; DROP DATABASE y"); !errors.Is(err, ErrBadInput) {
		t.Fatalf("injection: %v", err)
	}
	if len(e.eng.calls) != 0 {
		t.Fatalf("no engine call expected, got %v", e.eng.calls)
	}
}

func TestBackupMySQLUsesEnvPasswordNotArgv(t *testing.T) {
	e := setup(t, "k")
	sv := e.addServer(t, EngineMySQL)
	b, err := e.svc.CreateBackup(context.Background(), sv.UUID.String(), "shop")
	if err != nil {
		t.Fatal(err)
	}
	c := e.run.calls[0]
	if c.name != "mysqldump" {
		t.Fatalf("cmd %s", c.name)
	}
	for _, a := range c.args {
		if strings.Contains(a, "s3cret-pass") {
			t.Fatal("password leaked into argv")
		}
	}
	if len(c.env) != 1 || c.env[0] != "MYSQL_PWD=s3cret-pass" {
		t.Fatalf("env = %v", c.env)
	}
	if !backupFileRe.MatchString(b.File) || b.Database != "shop" || b.Size == 0 {
		t.Fatalf("backup = %+v", b)
	}
	if st, _ := os.Stat(filepath.Join(e.dir, b.File)); st.Mode().Perm() != 0o600 {
		t.Fatal("backup must be 0600")
	}
	list, _ := e.svc.ListBackups(sv.UUID.String())
	if len(list) != 1 || list[0].File != b.File {
		t.Fatalf("list = %+v", list)
	}
}

func TestBackupPostgresAndFailureCleanup(t *testing.T) {
	e := setup(t, "k")
	sv := e.addServer(t, EnginePostgres)
	if _, err := e.svc.CreateBackup(context.Background(), sv.UUID.String(), "shop"); err != nil {
		t.Fatal(err)
	}
	c := e.run.calls[0]
	if c.name != "pg_dump" || c.env[0] != "PGPASSWORD=s3cret-pass" {
		t.Fatalf("call = %+v", c)
	}
	e.run.fail = true
	if _, err := e.svc.CreateBackup(context.Background(), sv.UUID.String(), "other"); err == nil {
		t.Fatal("expected failure")
	}
}

func TestRestoreAndBackupFileValidation(t *testing.T) {
	e := setup(t, "k")
	sv := e.addServer(t, EngineMySQL)
	id := sv.UUID.String()
	b, _ := e.svc.CreateBackup(context.Background(), id, "shop")

	for _, f := range []string{"../../etc/passwd", "main.shop.20240101-000000.sql/../../x", "other.shop.20240101-000000.sql", "main.shop.bad.sql", ""} {
		if err := e.svc.Restore(context.Background(), id, f, ""); err == nil {
			t.Errorf("file %q must be rejected", f)
		}
		if err := e.svc.DeleteBackup(id, f); err == nil {
			t.Errorf("delete %q must be rejected", f)
		}
	}
	if err := e.svc.Restore(context.Background(), id, b.File, "mysql"); !errors.Is(err, ErrBadInput) {
		t.Fatalf("restore into system db: %v", err)
	}
	e.run.calls = nil
	if err := e.svc.Restore(context.Background(), id, b.File, "shop_copy"); err != nil {
		t.Fatal(err)
	}
	c := e.run.calls[0]
	if c.name != "mysql" || !strings.Contains(strings.Join(c.args, " "), "source "+filepath.Join(e.dir, b.File)) {
		t.Fatalf("restore call = %+v", c)
	}
	if err := e.svc.DeleteBackup(id, b.File); err != nil {
		t.Fatal(err)
	}
}
