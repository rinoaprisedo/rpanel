package filemanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

func testRouter(root string, super bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(utils.CtxIsSuperadmin, super) })
	allow := func(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
	NewHandler([]string{root}).RegisterRoutes(r.Group("/api/v1"), allow)
	return r
}

func do(r *gin.Engine, method, target string, body []byte, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUploadStreamingAndTraversal(t *testing.T) {
	root := t.TempDir()
	r := testRouter(root, false)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "../../evil.txt") // client-supplied path must be reduced to base name
	fw.Write([]byte("hello"))
	mw.Close()

	w := do(r, "POST", "/api/v1/file-manager/upload?dir="+url.QueryEscape(root), buf.Bytes(), mw.FormDataContentType())
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	if b, err := os.ReadFile(filepath.Join(root, "evil.txt")); err != nil || string(b) != "hello" {
		t.Fatalf("file not saved in dir: %v", err)
	}
	// Second upload without overwrite must conflict.
	w = do(r, "POST", "/api/v1/file-manager/upload?dir="+url.QueryEscape(root), buf.Bytes(), mw.FormDataContentType())
	if w.Code != 409 {
		t.Fatalf("want 409, got %d", w.Code)
	}
	// Upload outside roots is denied.
	w = do(r, "POST", "/api/v1/file-manager/upload?dir=/etc", buf.Bytes(), mw.FormDataContentType())
	if w.Code != 403 {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

func TestListDeniedOutsideRootAndAllowedForSuperadmin(t *testing.T) {
	root := t.TempDir()
	if w := do(testRouter(root, false), "GET", "/api/v1/file-manager/list?path=/etc", nil, ""); w.Code != 403 {
		t.Fatalf("non-super /etc: %d", w.Code)
	}
	if w := do(testRouter(root, true), "GET", "/api/v1/file-manager/list?path="+url.QueryEscape(os.TempDir()), nil, ""); w.Code != 200 {
		t.Fatalf("super: %d", w.Code)
	}
}

func TestWriteKeepsHTMLVerbatim(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "index.html")
	os.WriteFile(f, []byte("x"), 0o644)
	body, _ := json.Marshal(map[string]string{"path": f, "content": "<script>a()</script>"})
	if w := do(testRouter(root, false), "PUT", "/api/v1/file-manager/write", body, "application/json"); w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body)
	}
	if b, _ := os.ReadFile(f); string(b) != "<script>a()</script>" {
		t.Fatalf("content altered: %q", b)
	}
}

func TestChownRequiresSuperadmin(t *testing.T) {
	root := t.TempDir()
	body, _ := json.Marshal(map[string]string{"path": root, "owner": "root"})
	if w := do(testRouter(root, false), "POST", "/api/v1/file-manager/chown", body, "application/json"); w.Code != 403 {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

var _ = http.StatusOK

func TestRestrictedUsersCannotReachSensitiveDirs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"cron", "site/.ssh", "site/public"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.Symlink(filepath.Join(root, "cron"), filepath.Join(root, "site/public/link"))
	a := Access{Roots: []string{root}, Denied: []string{filepath.Join(root, "cron")}}
	for _, p := range []string{"cron", "cron/job.sh", "site/.ssh/authorized_keys", "site/public/link/job.sh"} {
		if _, err := a.Resolve(filepath.Join(root, p)); !errors.Is(err, system.ErrPathDenied) {
			t.Errorf("%s: want denied, got %v", p, err)
		}
	}
	if _, err := a.Resolve(filepath.Join(root, "site/public/index.php")); err != nil {
		t.Errorf("normal file denied: %v", err)
	}
	a.Unrestricted = true
	if _, err := a.Resolve(filepath.Join(root, "cron/job.sh")); err != nil {
		t.Errorf("superadmin denied: %v", err)
	}
}
