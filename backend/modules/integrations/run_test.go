package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"rpanel/backend/modules/docker"
	"rpanel/backend/modules/filemanager"
	"rpanel/backend/system"

	"github.com/gin-gonic/gin"
)

type fakeRun struct {
	calls [][]string
	failN int // 1-based call that fails, 0 = none
}

func (f *fakeRun) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.failN == len(f.calls) {
		return system.Result{Stderr: "boom", ExitCode: 1}, errors.New("exit status 1")
	}
	return system.Result{Stdout: "ok"}, nil
}

func runDeploy(t *testing.T, f *fakeRun) (*Deploy, string) {
	root := t.TempDir()
	return &Deploy{roots: []string{root}, access: filemanager.Access{Roots: []string{root}}, d: &docker.Docker{Run: f}}, root
}

func TestSplitCommand(t *testing.T) {
	got, err := splitCommand(`docker compose exec -T app sh -c 'php artisan migrate && php artisan db:seed'`)
	want := []string{"docker", "compose", "exec", "-T", "app", "sh", "-c", "php artisan migrate && php artisan db:seed"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"docker ps; rm -rf /", "docker ps && ls", "docker ps | cat", "docker ps > /etc/x", "docker $(id)", "docker `id`", `docker ps \`, "docker ps 'open"} {
		if _, err := splitCommand(bad); err == nil {
			t.Errorf("should reject %q", bad)
		}
	}
}

func TestVet(t *testing.T) {
	h, root := runDeploy(t, &fakeRun{})
	in := filepath.Join(root, "images.tar.gz")
	dir := filepath.Join(root, "app")
	ok := map[string][]string{
		"docker load -i " + in:                                   {"load", "-i", in},
		"docker compose up -d":                                   {"compose", "--project-directory", dir, "up", "-d"},
		"docker compose -p toi up -d --remove-orphans":           {"compose", "--project-directory", dir, "-p", "toi", "up", "-d", "--remove-orphans"},
		"docker compose -f " + dir + "/c.yml up -d":              {"compose", "-f", dir + "/c.yml", "up", "-d"},
		"docker compose exec -T app php artisan migrate --force": {"compose", "--project-directory", dir, "exec", "-T", "app", "php", "artisan", "migrate", "--force"},
		"docker exec app sh -c 'php artisan db:seed --force'":    {"exec", "app", "sh", "-c", "php artisan db:seed --force"},
		"docker exec -u root app ls --privileged":                {"exec", "-u", "root", "app", "ls", "--privileged"}, // after the container name: the app's own flag
	}
	for line, want := range ok {
		got, err := h.vet(line, dir)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q, %v want %q", line, got, err, want)
		}
	}
	bad := []string{
		"ls", "sudo docker ps", "docker", "docker run -v /:/host alpine", "docker rm -f x", "docker cp a b", "docker system prune -af",
		"docker compose run app sh", "docker compose", "docker compose rm -f", "docker compose -f /etc/passwd up",
		"docker compose --project-directory=/etc up", "docker load -i /etc/passwd", "docker load -i", "docker load --input=../x",
		"docker exec --privileged app sh", "docker compose exec --privileged app sh", "", "docker ps; id",
	}
	for _, line := range bad {
		if got, err := h.vet(line, dir); err == nil {
			t.Errorf("should reject %q, got %q", line, got)
		}
	}
}

func post(r *gin.Engine, body any) (int, map[string]any) {
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestRunStopsAtFirstFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := &fakeRun{failN: 2}
	h, root := runDeploy(t, f)
	r := gin.New()
	r.POST("/run", h.RunCommands)
	in := filepath.Join(root, "i.tar.gz")
	code, out := post(r, gin.H{"commands": []string{"docker load -i " + in, "docker compose -f " + root + "/c.yml up -d", "docker ps"}})
	if code != 200 {
		t.Fatalf("status %d %v", code, out)
	}
	data := out["data"].(map[string]any)
	if data["failed_at"].(float64) != 1 || len(data["results"].([]any)) != 2 || len(f.calls) != 2 {
		t.Fatalf("expected a stop after the 2nd command: %v calls=%d", data, len(f.calls))
	}
}

func TestRunRefusesEverythingIfOneLineIsBad(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := &fakeRun{}
	h, root := runDeploy(t, f)
	r := gin.New()
	r.POST("/run", h.RunCommands)
	code, out := post(r, gin.H{"commands": []string{"docker load -i " + filepath.Join(root, "i.tar.gz"), "docker ps; reboot"}})
	if code != 400 || len(f.calls) != 0 {
		t.Fatalf("status %d, calls %d, %v", code, len(f.calls), out)
	}
	if msg, _ := out["message"].(string); !strings.Contains(msg, "command 2") {
		t.Errorf("message should name the line: %q", msg)
	}
	if code, _ := post(r, gin.H{"commands": []string{"docker ps"}, "dir": "/etc"}); code != 403 {
		t.Errorf("dir outside the deploy folders must be refused, got %d", code)
	}
}
