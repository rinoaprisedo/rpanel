package storage

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

// The stored extension follows the sniffed type, never the client's file name.
func TestUploadIgnoresClientExtension(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, _ := w.CreateFormFile("f", "evil.html")
	part.Write(append(png, []byte("<script>alert(1)</script>")...))
	w.Close()
	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	file, hdr, err := req.FormFile("f")
	if err != nil {
		t.Fatal(err)
	}
	s := NewLocalStorage(t.TempDir(), "/uploads", 1<<20)
	rel, err := s.Upload(file, hdr, "avatars")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(rel, ".png") {
		t.Fatalf("stored as %q, want .png", rel)
	}
}
