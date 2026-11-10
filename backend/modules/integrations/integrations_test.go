package integrations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rpanel/backend/modules/filemanager"

	"github.com/gin-gonic/gin"
)

func TestValidScopes(t *testing.T) {
	if s, err := validScopes([]string{ScopeUpload, ScopeUpload, ScopeDocker}); err != nil || s != "deploy:upload,deploy:docker" {
		t.Fatalf("got %q %v", s, err)
	}
	for _, bad := range [][]string{{}, {"admin"}, {ScopeUpload, "x"}} {
		if _, err := validScopes(bad); err == nil {
			t.Fatalf("%v should be rejected", bad)
		}
	}
}

func TestPublicKeyFormat(t *testing.T) {
	blob := strings.Repeat("A", 68)
	ok := []string{"ssh-ed25519 " + blob, "ssh-ed25519 " + blob + " vibecoder@mac"}
	bad := []string{`command="rm -rf /" ssh-ed25519 ` + blob, "ssh-ed25519 " + blob + "\nssh-rsa " + blob, "ssh-ed25519 short", ""}
	for _, k := range ok {
		if !pubKeyRe.MatchString(k) {
			t.Errorf("should accept %q", k)
		}
	}
	for _, k := range bad {
		if pubKeyRe.MatchString(k) {
			t.Errorf("should reject %q", k)
		}
	}
}

func deployFor(t *testing.T) (*Deploy, *gin.Engine, string) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	h := &Deploy{roots: []string{root}, access: filemanager.Access{Roots: []string{root}}}
	r := gin.New()
	r.GET("/status", h.UploadStatus)
	r.POST("/chunk", h.Chunk)
	return h, r, root
}

func send(r *gin.Engine, method, url string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestChunkedUpload(t *testing.T) {
	_, r, root := deployFor(t)
	data := bytes.Repeat([]byte("0123456789"), 100)
	sum := sha256.Sum256(data)
	q := func(off int) string {
		return fmt.Sprintf("/chunk?dir=%s&name=img.tar&offset=%d&total=%d&sha256=%s", root, off, len(data), hex.EncodeToString(sum[:]))
	}
	if w := send(r, "POST", q(0), data[:400]); w.Code != 200 || !strings.Contains(w.Body.String(), `"done":false`) {
		t.Fatalf("first chunk: %d %s", w.Code, w.Body)
	}
	if w := send(r, "POST", q(100), data[100:200]); w.Code != 409 {
		t.Fatalf("misaligned chunk must conflict, got %d", w.Code)
	}
	if w := send(r, "GET", fmt.Sprintf("/status?dir=%s&name=img.tar", root), nil); !strings.Contains(w.Body.String(), `"size":400`) {
		t.Fatalf("status: %s", w.Body)
	}
	if w := send(r, "POST", q(400), data[400:]); w.Code != 200 || !strings.Contains(w.Body.String(), `"done":true`) {
		t.Fatalf("last chunk: %d %s", w.Code, w.Body)
	}
	got, _ := os.ReadFile(filepath.Join(root, "img.tar"))
	if !bytes.Equal(got, data) {
		t.Fatal("content differs")
	}
	if w := send(r, "POST", q(0), data[:10]); w.Code != 409 {
		t.Fatalf("existing file must not be overwritten, got %d", w.Code)
	}
}

func TestUploadRejectsEscapesAndBadChecksum(t *testing.T) {
	_, r, root := deployFor(t)
	outside := t.TempDir()
	for _, u := range []string{
		fmt.Sprintf("/chunk?dir=%s&name=a.tar&offset=0&total=1", outside),
		fmt.Sprintf("/chunk?dir=%s&name=../a.tar&offset=0&total=1", root),
		fmt.Sprintf("/chunk?dir=%s&name=a.tar.part&offset=0&total=1", root),
	} {
		if w := send(r, "POST", u, []byte("x")); w.Code == 200 {
			t.Errorf("%s should be rejected", u)
		}
	}
	u := fmt.Sprintf("/chunk?dir=%s&name=b.tar&offset=0&total=1&sha256=%s", root, strings.Repeat("0", 64))
	if w := send(r, "POST", u, []byte("x")); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad checksum: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "b.tar")); err == nil {
		t.Fatal("file with bad checksum must not be kept")
	}
}

func TestSymlinkedPartIsRefused(t *testing.T) {
	_, r, root := deployFor(t)
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("keep"), 0o600)
	os.Symlink(victim, filepath.Join(root, "c.tar.part"))
	u := fmt.Sprintf("/chunk?dir=%s&name=c.tar&offset=0&total=2", root)
	if w := send(r, "POST", u, []byte("zz")); w.Code == 200 {
		t.Fatal("symlinked .part must be refused")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatal("symlink target was written")
	}
}

func TestOverwriteKeepsOneBackup(t *testing.T) {
	_, r, root := deployFor(t)
	put := func(content string) *httptest.ResponseRecorder {
		u := fmt.Sprintf("/chunk?dir=%s&name=images.tar.gz&offset=0&total=%d&overwrite=1", root, len(content))
		return send(r, "POST", u, []byte(content))
	}
	read := func(n string) string { b, _ := os.ReadFile(filepath.Join(root, n)); return string(b) }
	exists := func(n string) bool { _, err := os.Stat(filepath.Join(root, n)); return err == nil }

	if w := put("v1"); w.Code != 200 || exists("bak-images.tar.gz") {
		t.Fatalf("first upload: %d, no backup expected", w.Code)
	}
	if w := put("v2"); w.Code != 200 || read("images.tar.gz") != "v2" || read("bak-images.tar.gz") != "v1" {
		t.Fatalf("second upload: %d images=%q bak=%q", w.Code, read("images.tar.gz"), read("bak-images.tar.gz"))
	}
	if w := put("v3"); w.Code != 200 || read("images.tar.gz") != "v3" || read("bak-images.tar.gz") != "v2" {
		t.Fatalf("third upload must replace the old backup: images=%q bak=%q", read("images.tar.gz"), read("bak-images.tar.gz"))
	}
	if exists("images.tar.gz.part") {
		t.Fatal("part file left behind")
	}
}

func TestFailedUploadLeavesCurrentFileAlone(t *testing.T) {
	_, r, root := deployFor(t)
	os.WriteFile(filepath.Join(root, "images.tar.gz"), []byte("current"), 0o640)
	os.WriteFile(filepath.Join(root, "bak-images.tar.gz"), []byte("older"), 0o640)
	u := fmt.Sprintf("/chunk?dir=%s&name=images.tar.gz&offset=0&total=3&overwrite=1&sha256=%s", root, strings.Repeat("0", 64))
	if w := send(r, "POST", u, []byte("new")); w.Code != 422 {
		t.Fatalf("bad checksum: %d", w.Code)
	}
	for n, want := range map[string]string{"images.tar.gz": "current", "bak-images.tar.gz": "older"} {
		if b, _ := os.ReadFile(filepath.Join(root, n)); string(b) != want {
			t.Fatalf("%s changed to %q", n, b)
		}
	}
}
