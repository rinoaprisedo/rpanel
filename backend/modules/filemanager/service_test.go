package filemanager

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
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
