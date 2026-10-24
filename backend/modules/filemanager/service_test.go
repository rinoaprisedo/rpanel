package filemanager

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, p, c string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProtectsRoots(t *testing.T) {
	root := t.TempDir()
	a := Access{Roots: []string{root}}
	if err := Delete(a, []string{filepath.Clean(root)}); !errors.Is(err, ErrProtected) {
		t.Fatalf("want ErrProtected, got %v", err)
	}
	if err := Delete(Access{Unrestricted: true}, []string{"/"}); !errors.Is(err, ErrProtected) {
		t.Fatalf("want ErrProtected for /, got %v", err)
	}
	f := filepath.Join(root, "x.txt")
	write(t, f, "hi")
	if err := Delete(a, []string{f}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("file not deleted")
	}
}

func TestMoveCopyRules(t *testing.T) {
	root := t.TempDir()
	a := Access{Roots: []string{root}}
	write(t, filepath.Join(root, "d/a.txt"), "A")
	os.Mkdir(filepath.Join(root, "dst"), 0o755)

	if err := Copy([]string{filepath.Join(root, "d")}, filepath.Join(root, "dst")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "dst/d/a.txt")); string(b) != "A" {
		t.Fatal("copy content wrong")
	}
	if err := Copy([]string{filepath.Join(root, "d")}, filepath.Join(root, "dst")); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
	if err := Move(a, []string{filepath.Join(root, "d")}, filepath.Join(root, "d", "sub")); err == nil {
		t.Fatal("moving dir into itself must fail")
	}
	if err := Move(a, []string{filepath.Join(root, "d")}, filepath.Join(root, "dst")); !errors.Is(err, ErrExists) {
		t.Fatalf("move onto existing: got %v", err)
	}
}

func TestRenameValidation(t *testing.T) {
	root := t.TempDir()
	a := Access{Roots: []string{root}}
	f := filepath.Join(root, "a.txt")
	write(t, f, "x")
	for _, n := range []string{"", "..", "a/b", "../x", "a\\b"} {
		if err := Rename(a, f, n); !errors.Is(err, ErrBadName) {
			t.Errorf("%q: want ErrBadName, got %v", n, err)
		}
	}
	if err := Rename(a, f, "b.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestReadWriteText(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "a.html")
	write(t, f, "<b>old</b>")
	if err := WriteText(f, "<script>1</script>"); err != nil {
		t.Fatal(err)
	}
	if c, _ := ReadText(f); c != "<script>1</script>" {
		t.Fatalf("content %q", c)
	}
	bin := filepath.Join(root, "b.bin")
	os.WriteFile(bin, []byte{1, 0, 2}, 0o644)
	if _, err := ReadText(bin); !errors.Is(err, ErrBinary) {
		t.Fatalf("want ErrBinary, got %v", err)
	}
}

func TestChmodValidation(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a")
	write(t, f, "x")
	for _, m := range []string{"", "999", "rwx", "07777", "64"} {
		if err := Chmod(f, m, false); !errors.Is(err, ErrBadMode) {
			t.Errorf("%q: want ErrBadMode, got %v", m, err)
		}
	}
	if err := Chmod(f, "600", false); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(f); info.Mode().Perm() != 0o600 {
		t.Fatal("mode not applied")
	}
}

func TestZipRoundTripAndZipSlip(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src/a.txt"), "A")
	write(t, filepath.Join(root, "src/sub/b.txt"), "B")
	z := filepath.Join(root, "out.zip")
	if err := Zip([]string{filepath.Join(root, "src")}, z); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "dst")
	os.Mkdir(dst, 0o755)
	if err := Unzip(z, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "src/sub/b.txt")); string(b) != "B" {
		t.Fatal("round trip failed")
	}

	evil := filepath.Join(root, "evil.zip")
	f, _ := os.Create(evil)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../escaped.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	dst2 := filepath.Join(root, "dst2")
	os.Mkdir(dst2, 0o755)
	if err := Unzip(evil, dst2); err == nil {
		t.Fatal("zip-slip must be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped.txt")); err == nil {
		t.Fatal("file escaped extraction dir")
	}
}

func TestUnzipRejectsSymlinkDirEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dst := filepath.Join(root, "dst")
	os.Mkdir(dst, 0o755)
	os.Symlink(outside, filepath.Join(dst, "link"))

	evil := filepath.Join(root, "e.zip")
	f, _ := os.Create(evil)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("link/pwn.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	if err := Unzip(evil, dst); err == nil {
		t.Fatal("must reject write through symlinked dir")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwn.txt")); err == nil {
		t.Fatal("file written outside")
	}
}

func TestSearchFindsNamesBelowDirAndStaysInside(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"site/public", "site/Reports-2026", "other"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "site/public/Report.PHP"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "other/notes.txt"), []byte("x"), 0o644)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "report-secret.txt"), []byte("x"), 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "other/link")); err != nil {
		t.Fatal(err)
	}

	got, truncated, err := Search(context.Background(), root, "  REPORT ", 50)
	if err != nil || truncated {
		t.Fatalf("err=%v truncated=%v", err, truncated)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Name)
	}
	want := "Reports-2026,Report.PHP" // folders first, then files; case-insensitive
	if strings.Join(names, ",") != want {
		t.Errorf("got %v, want %s", names, want)
	}
	for _, e := range got {
		if strings.HasPrefix(e.Path, outside) {
			t.Errorf("search followed a symlink out of the directory: %s", e.Path)
		}
	}
	if got[1].Path != filepath.Join(root, "site/public/Report.PHP") {
		t.Errorf("entries must carry their real parent directory: %s", got[1].Path)
	}
	if _, _, err := Search(context.Background(), root, "   ", 5); err == nil {
		t.Error("an empty query must be rejected")
	}
}

func TestSearchStopsAtTheResultLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 12; i++ {
		os.WriteFile(filepath.Join(root, fmt.Sprintf("log-%02d.txt", i)), nil, 0o644)
	}
	got, truncated, err := Search(context.Background(), root, "log", 5)
	if err != nil || len(got) != 5 || !truncated {
		t.Errorf("len=%d truncated=%v err=%v", len(got), truncated, err)
	}
}
