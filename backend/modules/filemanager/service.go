// Package filemanager implements the web file manager. Every path coming from
// a request must pass through Access.Resolve before touching the filesystem.
package filemanager

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"rpanel/backend/system"
)

const (
	MaxEditSize     = 2 << 20 // largest file the text editor will open
	maxUnzipBytes   = 4 << 30
	maxUnzipEntries = 100000
)

var (
	ErrExists    = errors.New("destination already exists")
	ErrNotDir    = errors.New("not a directory")
	ErrIsDir     = errors.New("is a directory")
	ErrTooLarge  = errors.New("file too large to edit")
	ErrBinary    = errors.New("binary file cannot be edited")
	ErrProtected = errors.New("refusing to modify a protected path")
	ErrBadName   = errors.New("invalid file name")
	ErrBadMode   = errors.New("invalid mode")
	ErrBadOwner  = errors.New("invalid owner or group")
	ownerRe      = regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}$`)
	modeRe       = regexp.MustCompile(`^[0-7]{3,4}$`)
)

// Access describes what the current user may touch.
type Access struct {
	Roots        []string
	Denied       []string
	Unrestricted bool
}

// Resolve validates p against the access rules. Restricted users additionally
// never reach a denied directory or any .ssh directory (authorized_keys = login).
func (a Access) Resolve(p string) (string, error) {
	clean, err := system.ResolvePath(p, a.Roots, a.Unrestricted)
	if err != nil || a.Unrestricted {
		return clean, err
	}
	real := system.RealPath(clean)
	for _, cand := range []string{clean, real} {
		for _, part := range strings.Split(cand, string(filepath.Separator)) {
			if part == ".ssh" || part == ".gnupg" {
				return "", system.ErrPathDenied
			}
		}
		for _, d := range a.Denied {
			if d == "" {
				continue
			}
			for _, dd := range []string{filepath.Clean(d), system.RealPath(d)} {
				if cand == dd || strings.HasPrefix(cand, dd+string(filepath.Separator)) {
					return "", system.ErrPathDenied
				}
			}
		}
	}
	return clean, nil
}

// protected reports paths that must never be deleted/renamed/moved: the
// filesystem root and the configured roots themselves.
func (a Access) protected(clean string) bool {
	if clean == string(filepath.Separator) {
		return true
	}
	for _, r := range a.Roots {
		if clean == filepath.Clean(r) {
			return true
		}
	}
	return false
}

// ValidName reports whether n is a single safe path element.
func ValidName(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, "/\\\x00") && len(n) <= 255
}

type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	IsLink  bool      `json:"is_link"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	Owner   string    `json:"owner"`
	Group   string    `json:"group"`
	ModTime time.Time `json:"mod_time"`
}

func ownerNames(info fs.FileInfo) (string, string) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ""
	}
	u, g := strconv.Itoa(int(st.Uid)), strconv.Itoa(int(st.Gid))
	if usr, err := user.LookupId(u); err == nil {
		u = usr.Username
	}
	if grp, err := user.LookupGroupId(g); err == nil {
		g = grp.Name
	}
	return u, g
}

func toEntry(dir string, info fs.FileInfo) Entry {
	e := Entry{
		Name: info.Name(), Path: filepath.Join(dir, info.Name()),
		IsDir: info.IsDir(), IsLink: info.Mode()&os.ModeSymlink != 0,
		Size: info.Size(), Mode: fmt.Sprintf("%04o", info.Mode().Perm()), ModTime: info.ModTime(),
	}
	e.Owner, e.Group = ownerNames(info)
	if e.IsLink { // show whether the link points at a directory
		if t, err := os.Stat(e.Path); err == nil {
			e.IsDir = t.IsDir()
		}
	}
	return e
}

// List returns the entries of dir, directories first then by name.
func List(dir string) ([]Entry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		info, err := it.Info()
		if err != nil {
			continue
		}
		out = append(out, toEntry(dir, info))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ReadText returns the file content for the editor.
func ReadText(p string) (string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", ErrIsDir
	}
	if info.Size() > MaxEditSize {
		return "", ErrTooLarge
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	head := b
	if len(head) > 8192 {
		head = head[:8192]
	}
	for _, c := range head {
		if c == 0 {
			return "", ErrBinary
		}
	}
	return string(b), nil
}

// WriteText atomically replaces a file's content, keeping its mode.
func WriteText(p, content string) error {
	if len(content) > MaxEditSize {
		return ErrTooLarge
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(p); err == nil {
		if info.IsDir() {
			return ErrIsDir
		}
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".rpanel-edit-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func Mkdir(p string) error { return os.Mkdir(p, 0o755) }

// Rename renames p within its directory.
func Rename(a Access, p, newName string) error {
	if a.protected(p) {
		return ErrProtected
	}
	if !ValidName(newName) {
		return ErrBadName
	}
	dst := filepath.Join(filepath.Dir(p), newName)
	if _, err := a.Resolve(dst); err != nil {
		return err
	}
	return renameNoClobber(p, dst)
}

func renameNoClobber(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return ErrExists
	}
	return os.Rename(src, dst)
}

// Move moves each src into the directory dstDir.
func Move(a Access, srcs []string, dstDir string) error {
	if err := mustBeDir(dstDir); err != nil {
		return err
	}
	for _, s := range srcs {
		if a.protected(s) {
			return ErrProtected
		}
		if isInside(dstDir, s) {
			return errors.New("cannot move a directory into itself")
		}
		if err := renameNoClobber(s, filepath.Join(dstDir, filepath.Base(s))); err != nil {
			return err
		}
	}
	return nil
}

// Copy copies each src (recursively) into dstDir. Symlinks are skipped.
func Copy(srcs []string, dstDir string) error {
	if err := mustBeDir(dstDir); err != nil {
		return err
	}
	for _, s := range srcs {
		if isInside(dstDir, s) {
			return errors.New("cannot copy a directory into itself")
		}
		dst := filepath.Join(dstDir, filepath.Base(s))
		if _, err := os.Lstat(dst); err == nil {
			return ErrExists
		}
		if err := copyTree(s, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return nil
	case info.IsDir():
		if err := os.Mkdir(dst, info.Mode().Perm()); err != nil {
			return err
		}
		items, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, it := range items {
			if err := copyTree(filepath.Join(src, it.Name()), filepath.Join(dst, it.Name())); err != nil {
				return err
			}
		}
		return nil
	case info.Mode().IsRegular():
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return nil
}

// Delete removes each path (recursively). Protected paths are refused.
func Delete(a Access, paths []string) error {
	for _, p := range paths {
		if a.protected(p) {
			return ErrProtected
		}
	}
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

// Chmod applies an octal mode, optionally recursively.
func Chmod(p, mode string, recursive bool) error {
	if !modeRe.MatchString(mode) {
		return ErrBadMode
	}
	m, _ := strconv.ParseUint(mode, 8, 32)
	apply := func(path string) error { return os.Chmod(path, fs.FileMode(m)) }
	if !recursive {
		return apply(p)
	}
	return filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		return apply(path)
	})
}

// Chown changes owner/group via chown(1) (no shell). Superadmin only.
func Chown(ctx context.Context, p, owner, group string, recursive bool) error {
	if !ownerRe.MatchString(owner) || (group != "" && !ownerRe.MatchString(group)) {
		return ErrBadOwner
	}
	spec := owner
	if group != "" {
		spec += ":" + group
	}
	args := []string{"-h"}
	if recursive {
		args = append(args, "-R")
	}
	args = append(args, spec, "--", p)
	_, err := system.Run(ctx, 0, "chown", args...)
	return err
}

func mustBeDir(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrNotDir
	}
	return nil
}

// isInside reports whether child is the same as or below parent.
func isInside(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Zip archives srcs into the file dstZip.
func Zip(srcs []string, dstZip string) error {
	if _, err := os.Lstat(dstZip); err == nil {
		return ErrExists
	}
	f, err := os.OpenFile(dstZip, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	fail := func(err error) error {
		zw.Close()
		f.Close()
		os.Remove(dstZip)
		return err
	}
	for _, s := range srcs {
		base := filepath.Dir(s)
		err := filepath.WalkDir(s, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == dstZip || d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			rel, _ := filepath.Rel(base, path)
			rel = filepath.ToSlash(rel)
			info, err := d.Info()
			if err != nil {
				return err
			}
			hdr, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			hdr.Name = rel
			if d.IsDir() {
				hdr.Name += "/"
				_, err = zw.CreateHeader(hdr)
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			hdr.Method = zip.Deflate
			w, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			_, err = io.Copy(w, in)
			return err
		})
		if err != nil {
			return fail(err)
		}
	}
	if err := zw.Close(); err != nil {
		return fail(err)
	}
	return f.Close()
}

// Unzip extracts zipPath into dstDir, rejecting path traversal (zip-slip),
// symlinks, and archives exceeding size/entry limits.
func Unzip(zipPath, dstDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if len(zr.File) > maxUnzipEntries {
		return errors.New("archive has too many entries")
	}
	root := filepath.Clean(dstDir)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	var total uint64
	for _, zf := range zr.File {
		total += zf.UncompressedSize64
		if total > maxUnzipBytes {
			return errors.New("archive too large when extracted")
		}
		if zf.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains a symlink: %s", zf.Name)
		}
		target := filepath.Join(root, filepath.FromSlash(zf.Name))
		if !isInside(target, root) || target == root && !zf.FileInfo().IsDir() {
			return fmt.Errorf("illegal path in archive: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		// An existing symlinked directory inside dstDir must not redirect output.
		if real, err := filepath.EvalSymlinks(filepath.Dir(target)); err != nil || !isInside(real, realRoot) {
			return fmt.Errorf("illegal path in archive: %s", zf.Name)
		}
		if err := extractFile(zf, target); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(zf *zip.File, target string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := zf.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	// Bound actual bytes copied to the declared size to defeat lying headers.
	size := int64(zf.UncompressedSize64)
	n, err := io.Copy(out, io.LimitReader(rc, size+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > size {
		err = fmt.Errorf("entry %s larger than declared", zf.Name)
	}
	if err != nil {
		os.Remove(target)
	}
	return err
}

const (
	maxSearchResults = 300
	maxSearchVisited = 100_000
)

// skipInSearch are pseudo file systems that are huge and never worth searching.
var skipInSearch = map[string]bool{"/proc": true, "/sys": true, "/dev": true, "/run": true}

// Search walks dir and returns the entries whose name contains q (case-insensitive).
// Symlinks are not followed, so a link cannot lead the walk outside dir.
// truncated is true when the result or visit limit stopped the search early.
func Search(ctx context.Context, dir, q string, limit int) (out []Entry, truncated bool, err error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil, false, ErrBadName
	}
	if limit <= 0 || limit > maxSearchResults {
		limit = maxSearchResults
	}
	visited := 0
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir // unreadable folder: keep going
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p == dir {
			return nil
		}
		if d.IsDir() && skipInSearch[p] {
			return fs.SkipDir
		}
		if visited++; visited > maxSearchVisited {
			truncated = true
			return fs.SkipAll
		}
		if !strings.Contains(strings.ToLower(d.Name()), q) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		out = append(out, toEntry(filepath.Dir(p), info))
		if len(out) >= limit {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return nil, false, err
	}
	if err != nil {
		truncated = true // timed out: return what was found so far
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	return out, truncated, nil
}
