package system

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	domainRe = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))+$`)
	identRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

// ValidDomain reports whether s is a plain hostname (no wildcard, port or path).
func ValidDomain(s string) bool { return len(s) <= 253 && domainRe.MatchString(s) }

// ValidIdent reports whether s is a safe SQL/system identifier.
func ValidIdent(s string) bool { return identRe.MatchString(s) }

var ErrPathDenied = errors.New("path outside allowed roots")

// ResolvePath cleans p and verifies that it lies inside one of roots
// (absolute paths). If unrestricted is true any absolute path is accepted.
// Symlinks are resolved for the longest existing prefix so a link inside an
// allowed root cannot escape it.
func ResolvePath(p string, roots []string, unrestricted bool) (string, error) {
	if p == "" || strings.ContainsRune(p, 0) {
		return "", ErrPathDenied
	}
	if !filepath.IsAbs(p) {
		return "", ErrPathDenied
	}
	clean := filepath.Clean(p)
	real := evalExisting(clean)
	if unrestricted {
		return clean, nil
	}
	for _, r := range roots {
		rr := evalExisting(filepath.Clean(r))
		if within(real, rr) {
			return clean, nil
		}
	}
	return "", ErrPathDenied
}

func within(p, root string) bool {
	if p == root {
		return true
	}
	if root == string(filepath.Separator) {
		return true
	}
	return strings.HasPrefix(p, root+string(filepath.Separator))
}

// evalExisting resolves symlinks of the deepest existing ancestor of p and
// re-appends the remaining (not yet existing) suffix.
func evalExisting(p string) string {
	cur, rest := p, ""
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// RealPath resolves symlinks in the longest existing prefix of p, so it also
// works for a file that is about to be created.
func RealPath(p string) string { return evalExisting(filepath.Clean(p)) }
