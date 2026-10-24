package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsUnlisted(t *testing.T) {
	for _, n := range []string{"sh", "bash", "rm", "/bin/ls", "nginx; id"} {
		if _, err := Run(context.Background(), 0, n); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%q: want ErrNotAllowed, got %v", n, err)
		}
	}
}

func TestRunRejectsNUL(t *testing.T) {
	if _, err := Run(context.Background(), 0, "id", "a\x00b"); err == nil {
		t.Fatal("expected NUL error")
	}
}

func TestRunNoShellInterpretation(t *testing.T) {
	// "id" with a hostile arg must be passed literally, not via a shell.
	res, err := Run(context.Background(), 0, "id", "-u", "; echo pwned")
	if err == nil && strings.Contains(res.Stdout, "pwned") {
		t.Fatal("shell metacharacters were interpreted")
	}
}

func TestValidDomain(t *testing.T) {
	ok := []string{"example.com", "a-b.example.co.id", "sub.example.com"}
	bad := []string{"", "localhost", "-a.com", "a..com", "exa mple.com", "a.com;rm", "*.a.com", "a.com/x", "a.com:80"}
	for _, s := range ok {
		if !ValidDomain(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range bad {
		if ValidDomain(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestValidIdent(t *testing.T) {
	if !ValidIdent("my_db1") || ValidIdent("1db") || ValidIdent("a-b") || ValidIdent("a;b") || ValidIdent("") {
		t.Fatal("ident validation wrong")
	}
}

func TestResolvePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.Symlink(outside, filepath.Join(root, "escape"))
	roots := []string{root}

	cases := []struct {
		p  string
		ok bool
	}{
		{root, true},
		{filepath.Join(root, "sub"), true},
		{filepath.Join(root, "sub", "new.txt"), true},
		{filepath.Join(root, "sub", "..", "..", "etc"), false},
		{root + "-evil", false},
		{filepath.Join(root, "escape"), false},
		{filepath.Join(root, "escape", "x"), false},
		{"relative/path", false},
		{"", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		_, err := ResolvePath(c.p, roots, false)
		if (err == nil) != c.ok {
			t.Errorf("%q: ok=%v err=%v", c.p, c.ok, err)
		}
	}
	if _, err := ResolvePath("/etc/passwd", roots, true); err != nil {
		t.Errorf("unrestricted should allow: %v", err)
	}
	if _, err := ResolvePath("rel", roots, true); err == nil {
		t.Error("relative must be rejected even when unrestricted")
	}
}
