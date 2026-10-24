package cronjobs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type env struct {
	svc     *Service
	cronDir string
	dataDir string
}

func setup(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Job{}, &Run{}); err != nil {
		t.Fatal(err)
	}
	e := &env{cronDir: filepath.Join(dir, "cron.d"), dataDir: filepath.Join(dir, "data")}
	e.svc = NewService(db, e.cronDir, e.dataDir)
	e.svc.exe, e.svc.wd = "/opt/rpanel/rpanel", "/opt/rpanel"
	return e
}

func TestNormalizeSchedule(t *testing.T) {
	ok := map[string]string{
		"* * * * *": "* * * * *", "  */5  0-6  1,15 * 1-5 ": "*/5 0-6 1,15 * 1-5", "0 3 * * 0": "0 3 * * 0",
	}
	for in, want := range ok {
		if got, err := NormalizeSchedule(in); err != nil || got != want {
			t.Errorf("%q => %q, %v", in, got, err)
		}
	}
	bad := []string{
		"", "* * * *", "* * * * * *", "@daily", "* * * * mon", "61 * * * *", "* 25 * * *",
		"* * * * *\nroot rm -rf /", "* * * * * root id", "$(id) * * * *", "*;ls * * * *", "* * * * * ; id",
	}
	for _, in := range bad {
		if _, err := NormalizeSchedule(in); !errors.Is(err, ErrBadInput) {
			t.Errorf("%q must be rejected, got %v", in, err)
		}
	}
}

func TestCreateWritesCronFileAndScript(t *testing.T) {
	e := setup(t)
	j, err := e.svc.Create(Input{Name: "backup", Type: TypeShell, Schedule: "30 2 * * *", Command: "echo hi\r\necho there"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(e.cronDir, "rpanel-"+j.UUID.String()))
	if err != nil {
		t.Fatal(err)
	}
	want := "30 2 * * * root cd /opt/rpanel && /opt/rpanel/rpanel cron-run " + j.UUID.String() + " >/dev/null 2>&1\n"
	if !strings.Contains(string(b), want) {
		t.Fatalf("cron file:\n%s", b)
	}
	if st, _ := os.Stat(filepath.Join(e.cronDir, "rpanel-"+j.UUID.String())); st.Mode().Perm() != 0o644 {
		t.Fatalf("cron file mode %v (cron rejects writable-by-others)", st.Mode().Perm())
	}
	if strings.ContainsRune(filepath.Base(e.svc.cronFile(j)), '.') {
		t.Fatal("cron.d file names must not contain dots (they are ignored)")
	}
	sp := filepath.Join(e.dataDir, "scripts", j.UUID.String()+".sh")
	if c, _ := os.ReadFile(sp); string(c) != "echo hi\necho there\n" {
		t.Fatalf("script %q", c)
	}
	if st, _ := os.Stat(sp); st.Mode().Perm() != 0o700 {
		t.Fatal("script must be 0700")
	}
}

func TestDisableAndDeleteRemoveFiles(t *testing.T) {
	e := setup(t)
	j, _ := e.svc.Create(Input{Name: "a", Type: TypeShell, Schedule: "* * * * *", Command: "true"})
	cf := e.svc.cronFile(j)
	if _, err := e.svc.SetEnabled(j.UUID.String(), false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cf); err == nil {
		t.Fatal("disabled job must have no cron file")
	}
	if _, err := e.svc.SetEnabled(j.UUID.String(), true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cf); err != nil {
		t.Fatal("enabled job must have cron file")
	}
	if _, err := e.svc.Delete(j.UUID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cf); err == nil {
		t.Fatal("cron file must be removed")
	}
	if _, err := os.Stat(e.svc.scriptPath(j)); err == nil {
		t.Fatal("script must be removed")
	}
	if _, err := e.svc.Get(j.UUID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatal("row must be gone")
	}
}

func TestValidateInput(t *testing.T) {
	e := setup(t)
	bad := []Input{
		{Name: "", Type: TypeShell, Schedule: "* * * * *", Command: "x"},
		{Name: "n", Type: "weird", Schedule: "* * * * *", Command: "x"},
		{Name: "n", Type: TypeShell, Schedule: "* * * * *", Command: "   "},
		{Name: "n", Type: TypeShell, Schedule: "* * * * *", Command: "x", Timeout: 999999},
		{Name: "n", Type: TypeURL, Schedule: "* * * * *", Command: "ftp://x"},
		{Name: "n", Type: TypeURL, Schedule: "* * * * *", Command: "http://a.com/x y"},
		{Name: "n\nx", Type: TypeShell, Schedule: "* * * * *", Command: "x"},
	}
	for i, in := range bad {
		if _, err := e.svc.Create(in); !errors.Is(err, ErrBadInput) {
			t.Errorf("case %d must be rejected, got %v", i, err)
		}
	}
	if list, _ := e.svc.List(); len(list) != 0 {
		t.Fatal("nothing must be stored")
	}
	if entries, _ := os.ReadDir(e.cronDir); len(entries) != 0 {
		t.Fatal("no cron file must be written")
	}
}

func TestCreateFailsWhenPanelPathUnsafe(t *testing.T) {
	e := setup(t)
	e.svc.wd = "/opt/my panel"
	if _, err := e.svc.Create(Input{Name: "a", Type: TypeShell, Schedule: "* * * * *", Command: "true"}); err == nil {
		t.Fatal("unsafe path must be refused")
	}
	if list, _ := e.svc.List(); len(list) != 0 {
		t.Fatal("row must be rolled back")
	}
}

func TestExecuteShellCapturesOutputAndExit(t *testing.T) {
	e := setup(t)
	j, _ := e.svc.Create(Input{Name: "a", Type: TypeShell, Schedule: "* * * * *", Command: "echo out; echo err 1>&2; exit 3"})
	r := e.svc.Execute(context.Background(), j, "manual")
	if r.Status != RunFailed || r.ExitCode != 3 || !strings.Contains(r.Output, "out") || !strings.Contains(r.Output, "err") || r.FinishedAt == nil {
		t.Fatalf("run %+v", r)
	}
	ok, _ := e.svc.Create(Input{Name: "b", Type: TypeShell, Schedule: "* * * * *", Command: "echo fine"})
	r2 := e.svc.Execute(context.Background(), ok, "schedule")
	if r2.Status != RunSuccess || r2.ExitCode != 0 || r2.Trigger != "schedule" {
		t.Fatalf("run %+v", r2)
	}
	got, _ := e.svc.Get(ok.UUID.String())
	if got.LastStatus != RunSuccess || got.LastRunAt == nil {
		t.Fatalf("job status not updated: %+v", got)
	}
}

func TestExecuteTimeoutKillsProcessTree(t *testing.T) {
	e := setup(t)
	j, _ := e.svc.Create(Input{Name: "slow", Type: TypeShell, Schedule: "* * * * *", Command: "sleep 30 & sleep 30", Timeout: 1})
	r := e.svc.Execute(context.Background(), j, "manual")
	if r.Status != RunFailed || !strings.Contains(r.Output, "timed out") {
		t.Fatalf("run %+v", r)
	}
	if d := r.FinishedAt.Sub(r.StartedAt).Seconds(); d > 10 {
		t.Fatalf("timeout not enforced: %.1fs", d)
	}
}

func TestExecuteURL(t *testing.T) {
	e := setup(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			http.Error(w, "nope", 500)
			return
		}
		w.Write([]byte("pong"))
	}))
	defer srv.Close()
	ok, _ := e.svc.Create(Input{Name: "ping", Type: TypeURL, Schedule: "* * * * *", Command: srv.URL + "/ok"})
	r := e.svc.Execute(context.Background(), ok, "manual")
	if r.Status != RunSuccess || r.ExitCode != 200 || !strings.Contains(r.Output, "pong") {
		t.Fatalf("run %+v", r)
	}
	bad, _ := e.svc.Create(Input{Name: "bad", Type: TypeURL, Schedule: "* * * * *", Command: srv.URL + "/fail"})
	r = e.svc.Execute(context.Background(), bad, "manual")
	if r.Status != RunFailed || r.ExitCode != 500 {
		t.Fatalf("run %+v", r)
	}
	if _, err := os.Stat(e.svc.scriptPath(ok)); err == nil {
		t.Fatal("URL jobs need no script file")
	}
}

func TestOverlapIsSkippedAndHistoryPruned(t *testing.T) {
	e := setup(t)
	j, _ := e.svc.Create(Input{Name: "a", Type: TypeShell, Schedule: "* * * * *", Command: "true"})
	e.svc.db.Create(&Run{JobID: j.ID, Status: RunRunning, StartedAt: j.CreatedAt})
	// a "running" row that is recent enough must block a second start
	e.svc.db.Model(&Run{}).Where("job_id = ?", j.ID).Update("started_at", nowish())
	r := e.svc.Execute(context.Background(), j, "schedule")
	if r.Status != RunSkipped {
		t.Fatalf("want skipped, got %+v", r)
	}
	e.svc.db.Where("job_id = ?", j.ID).Delete(&Run{})
	for i := 0; i < keepRuns+8; i++ {
		e.svc.Execute(context.Background(), j, "manual")
	}
	var n int64
	e.svc.db.Model(&Run{}).Where("job_id = ?", j.ID).Count(&n)
	if n != keepRuns {
		t.Fatalf("history not pruned: %d", n)
	}
}

func TestRunByUUID(t *testing.T) {
	e := setup(t)
	j, _ := e.svc.Create(Input{Name: "a", Type: TypeShell, Schedule: "* * * * *", Command: "echo x"})
	if r, err := e.svc.RunByUUID(context.Background(), j.UUID.String()); err != nil || r.Status != RunSuccess {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := e.svc.RunByUUID(context.Background(), "../../etc"); !errors.Is(err, ErrBadInput) {
		t.Fatalf("bad id: %v", err)
	}
	e.svc.SetEnabled(j.UUID.String(), false, nil)
	if _, err := e.svc.RunByUUID(context.Background(), j.UUID.String()); err == nil {
		t.Fatal("disabled job must not run from cron")
	}
}

func TestSyncAllRewritesFiles(t *testing.T) {
	e := setup(t)
	j, _ := e.svc.Create(Input{Name: "a", Type: TypeShell, Schedule: "* * * * *", Command: "true"})
	os.Remove(e.svc.cronFile(j))
	e.svc.exe = "/usr/local/bin/rpanel-new"
	if err := e.svc.SyncAll(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(e.svc.cronFile(j))
	if !strings.Contains(string(b), "/usr/local/bin/rpanel-new cron-run") {
		t.Fatalf("not rewritten:\n%s", b)
	}
}

func nowish() time.Time { return time.Now() }
