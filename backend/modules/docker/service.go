// Package docker manages containers, images and compose projects by driving
// the docker CLI through system.Runner (no shell, argv only).
package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rpanel/backend/system"
)

var (
	ErrBadInput = errors.New("invalid input")
	ErrNotFound = errors.New("not found")

	containerRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`) // names and hex ids
	imageRefRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,254}$`)
	envKeyRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	portRe      = regexp.MustCompile(`^(([0-9]{1,3}\.){3}[0-9]{1,3}:)?[0-9]{1,5}:[0-9]{1,5}(/(tcp|udp))?$`)
	volumeRe    = regexp.MustCompile(`^(/[A-Za-z0-9._/-]*|[A-Za-z0-9][A-Za-z0-9_.-]*):/[A-Za-z0-9._/-]*(:ro|:rw)?$`)
	labelKeyRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	restartRe   = regexp.MustCompile(`^(no|always|unless-stopped|on-failure(:[0-9]{1,3})?)$`)
)

// Docker wraps the CLI.
type Docker struct {
	Run system.Runner
}

func (d *Docker) cli(ctx context.Context, timeout time.Duration, args ...string) (system.Result, error) {
	res, err := d.Run.Run(ctx, timeout, "docker", args...)
	if err != nil {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = err.Error()
		}
		return res, errors.New(lastLines(msg, 6))
	}
	return res, nil
}

func lastLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

// ---- status ----

type Status struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"` // daemon reachable
	Version   string `json:"version"`
	Compose   bool   `json:"compose"`
	Error     string `json:"error,omitempty"`
}

func (d *Docker) Status(ctx context.Context) Status {
	var st Status
	res, err := d.Run.Run(ctx, 10*time.Second, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		// Distinguish "binary missing" from "daemon down".
		if strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "not allowed") {
			return st
		}
		st.Installed = true
		st.Error = lastLines(strings.TrimSpace(res.Stderr), 2)
		return st
	}
	st.Installed, st.Running = true, true
	st.Version = strings.TrimSpace(res.Stdout)
	if _, err := d.Run.Run(ctx, 10*time.Second, "docker", "compose", "version"); err == nil {
		st.Compose = true
	}
	return st
}

// Install installs Docker from the distribution repositories.
func (d *Docker) Install(ctx context.Context) error {
	if res, err := d.Run.Run(ctx, 5*time.Minute, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt-get update failed: %s", lastLines(res.Stderr, 4))
	}
	if res, err := d.Run.Run(ctx, 20*time.Minute, "apt-get", "install", "-y", "docker.io"); err != nil {
		return fmt.Errorf("install failed: %s", lastLines(res.Stderr, 6))
	}
	// Compose v2 plugin: package name differs between distributions; best effort.
	if _, err := d.Run.Run(ctx, 10*time.Minute, "apt-get", "install", "-y", "docker-compose-v2"); err != nil {
		_, _ = d.Run.Run(ctx, 10*time.Minute, "apt-get", "install", "-y", "docker-compose-plugin")
	}
	if res, err := d.Run.Run(ctx, time.Minute, "systemctl", "enable", "--now", "docker"); err != nil {
		return fmt.Errorf("cannot start docker: %s", lastLines(res.Stderr, 4))
	}
	return nil
}

// ---- containers ----

type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
	Created string `json:"created"`
}

func jsonLines(out string, each func(raw []byte) error) error {
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if err := each([]byte(line)); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (d *Docker) ListContainers(ctx context.Context) ([]Container, error) {
	res, err := d.cli(ctx, 30*time.Second, "ps", "-a", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	out := []Container{}
	err = jsonLines(res.Stdout, func(raw []byte) error {
		var r struct{ ID, Names, Image, State, Status, Ports, CreatedAt string }
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		out = append(out, Container{ID: r.ID, Name: r.Names, Image: r.Image, State: r.State, Status: r.Status, Ports: r.Ports, Created: r.CreatedAt})
		return nil
	})
	return out, err
}

// ValidContainer reports whether s is a safe container name or id.
func ValidContainer(s string) bool { return containerRe.MatchString(s) }

func (d *Docker) Action(ctx context.Context, id, action string) error {
	if !ValidContainer(id) {
		return fmt.Errorf("%w: container", ErrBadInput)
	}
	var args []string
	switch action {
	case "start", "stop", "restart", "pause", "unpause", "kill":
		args = []string{action, id}
	case "remove":
		args = []string{"rm", "-f", id}
	default:
		return fmt.Errorf("%w: action", ErrBadInput)
	}
	_, err := d.cli(ctx, 2*time.Minute, args...)
	return err
}

func (d *Docker) Logs(ctx context.Context, id string, tail int) (string, error) {
	if !ValidContainer(id) {
		return "", fmt.Errorf("%w: container", ErrBadInput)
	}
	if tail < 1 || tail > 2000 {
		tail = 200
	}
	res, err := d.cli(ctx, 30*time.Second, "logs", "--tail", strconv.Itoa(tail), "--timestamps", id)
	if err != nil {
		return "", err
	}
	// docker writes container stderr to our stderr: merge for display.
	return res.Stdout + res.Stderr, nil
}

type RunInput struct {
	Image   string   `json:"image"`
	Name    string   `json:"name"`
	Ports   []string `json:"ports"`   // host:container[/proto] or ip:host:container
	Env     []string `json:"env"`     // KEY=VALUE
	Volumes []string `json:"volumes"` // /host/path:/container/path[:ro] or named:/path
	Restart string   `json:"restart"`

	ExposeAll   bool     `json:"expose_all"`  // -P: publish every EXPOSEd port to a random host port
	Networks    []string `json:"networks"`    // --network, first one only is honoured by docker run, the rest are connected after
	Command     string   `json:"command"`     // arguments after the image, split shell-style (never run through a shell)
	Entrypoint  string   `json:"entrypoint"`  // --entrypoint (executable only)
	AutoRemove  bool     `json:"auto_remove"` // --rm
	TTY         bool     `json:"tty"`         // -t
	Interactive bool     `json:"interactive"` // -i
	Privileged  bool     `json:"privileged"`  // --privileged
	MinMemoryMB int      `json:"min_memory"`  // --memory-reservation, 0 = unset
	CPUs        float64  `json:"cpus"`        // --cpus, 0 = unset
	MemoryMB    int      `json:"memory"`      // --memory, 0 = unset
	Labels      []string `json:"labels"`      // KEY=VALUE
	Remark      string   `json:"remark"`      // stored as label panel.remark
	Owner       string   `json:"owner"`       // uid:gid to chown host-path mounts to before starting, "" = leave alone
}

var ownerRe = regexp.MustCompile(`^([0-9]{1,10}):([0-9]{1,10})$`)

// protectedRoots are never created into or chowned, whatever the user mounts.
var protectedRoots = map[string]bool{
	"bin": true, "boot": true, "dev": true, "etc": true, "lib": true, "lib32": true, "lib64": true, "proc": true,
	"root": true, "run": true, "sbin": true, "sys": true, "usr": true, "home": true,
}

// protectedTrees are paths whose ownership the panel or docker itself relies on.
var protectedTrees = []string{"/opt/rpanel", "/var/lib/docker", "/var/lib/containerd", "/var/lib/rpanel", "/var/lib/postgresql", "/var/spool", "/var/log"}

func underAny(p string, roots []string) bool {
	for _, r := range roots {
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// prepareMounts creates missing host directories of bind mounts and gives them
// to in.Owner (recursively), so images that run as an unprivileged user
// (mysql, postgres, ...) can write to them.
func prepareMounts(in RunInput) error {
	m := ownerRe.FindStringSubmatch(in.Owner)
	if m == nil {
		return fmt.Errorf("%w: owner must look like 999:999", ErrBadInput)
	}
	uid, _ := strconv.Atoi(m[1])
	gid, _ := strconv.Atoi(m[2])
	for _, v := range in.Volumes {
		src, _, _ := strings.Cut(v, ":")
		if !strings.HasPrefix(src, "/") {
			continue // named volume: docker sets its permissions
		}
		src = filepath.Clean(src)
		parts := strings.Split(strings.Trim(src, "/"), "/")
		if len(parts) < 2 || protectedRoots[parts[0]] || underAny(src, protectedTrees) {
			return fmt.Errorf("%w: refusing to change owner of %s", ErrBadInput, src)
		}
		if err := os.MkdirAll(src, 0o755); err != nil {
			return err
		}
		err := filepath.WalkDir(src, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Lchown(path, uid, gid)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// splitArgs splits s into argv words honouring single and double quotes and
// backslash escapes. Nothing is expanded or executed.
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	has := false
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && quote != '\'':
			esc = true
			has = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			has = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("%w: unclosed quote in command", ErrBadInput)
	}
	if has {
		out = append(out, cur.String())
	}
	return out, nil
}

// BuildRunArgs validates the input and produces argv for `docker run -d`.
// Every user value is its own argv element, and the image follows `--`-free
// positional position only after validation that it cannot start with '-'.
func BuildRunArgs(in RunInput) ([]string, error) {
	if !imageRefRe.MatchString(in.Image) {
		return nil, fmt.Errorf("%w: image", ErrBadInput)
	}
	args := []string{"run", "-d"}
	if in.Name != "" {
		if !containerRe.MatchString(in.Name) {
			return nil, fmt.Errorf("%w: name", ErrBadInput)
		}
		args = append(args, "--name", in.Name)
	}
	if in.AutoRemove {
		args = append(args, "--rm") // docker refuses --rm together with a restart policy
	} else if in.Restart != "" {
		if !restartRe.MatchString(in.Restart) {
			return nil, fmt.Errorf("%w: restart policy", ErrBadInput)
		}
		args = append(args, "--restart", in.Restart)
	}
	for _, p := range in.Ports {
		if !portRe.MatchString(p) {
			return nil, fmt.Errorf("%w: port mapping %q", ErrBadInput, p)
		}
		args = append(args, "-p", p)
	}
	for _, e := range in.Env {
		k, v, ok := strings.Cut(e, "=")
		if !ok || !envKeyRe.MatchString(k) || strings.ContainsAny(v, "\x00\r\n") {
			return nil, fmt.Errorf("%w: environment variable", ErrBadInput)
		}
		args = append(args, "-e", e)
	}
	for _, v := range in.Volumes {
		if !volumeRe.MatchString(v) || strings.Contains(v, "..") {
			return nil, fmt.Errorf("%w: volume %q", ErrBadInput, v)
		}
		args = append(args, "-v", v)
	}
	if in.ExposeAll {
		args = append(args, "-P")
	}
	for _, n := range in.Networks {
		if !containerRe.MatchString(n) {
			return nil, fmt.Errorf("%w: network %q", ErrBadInput, n)
		}
		args = append(args, "--network", n)
		break // docker run takes a single network
	}
	for _, l := range in.Labels {
		k, v, ok := strings.Cut(l, "=")
		if !ok || !labelKeyRe.MatchString(k) || strings.ContainsAny(v, "\x00\r\n") {
			return nil, fmt.Errorf("%w: label", ErrBadInput)
		}
		args = append(args, "--label", l)
	}
	if r := strings.TrimSpace(in.Remark); r != "" {
		if len(r) > 500 || strings.ContainsAny(r, "\x00\r\n") {
			return nil, fmt.Errorf("%w: remark", ErrBadInput)
		}
		args = append(args, "--label", "panel.remark="+r)
	}
	if in.TTY {
		args = append(args, "-t")
	}
	if in.Interactive {
		args = append(args, "-i")
	}
	if in.Privileged {
		args = append(args, "--privileged")
	}
	if in.MinMemoryMB < 0 || in.MemoryMB < 0 || in.CPUs < 0 || in.MinMemoryMB > 1<<22 || in.MemoryMB > 1<<22 || in.CPUs > 1024 {
		return nil, fmt.Errorf("%w: resource limit", ErrBadInput)
	}
	if in.MinMemoryMB > 0 {
		args = append(args, "--memory-reservation", fmt.Sprintf("%dm", in.MinMemoryMB))
	}
	if in.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", in.MemoryMB))
	}
	if in.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(in.CPUs, 'f', -1, 64))
	}
	if ep := strings.TrimSpace(in.Entrypoint); ep != "" {
		if strings.ContainsAny(ep, "\x00\r\n") {
			return nil, fmt.Errorf("%w: entrypoint", ErrBadInput)
		}
		args = append(args, "--entrypoint", ep)
	}
	args = append(args, in.Image)
	if strings.TrimSpace(in.Command) != "" {
		words, err := splitArgs(in.Command)
		if err != nil {
			return nil, err
		}
		for _, w := range words {
			if strings.ContainsRune(w, 0) {
				return nil, fmt.Errorf("%w: command", ErrBadInput)
			}
		}
		args = append(args, words...)
	}
	return args, nil
}

func (d *Docker) RunContainer(ctx context.Context, in RunInput) (string, error) {
	args, err := BuildRunArgs(in)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Owner) != "" {
		if err := prepareMounts(in); err != nil {
			return "", err
		}
	}
	res, err := d.cli(ctx, 15*time.Minute, args...) // may pull the image first
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(lastLines(res.Stdout, 1)), nil
}

// ---- images ----

type Image struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}

func (d *Docker) ListImages(ctx context.Context) ([]Image, error) {
	res, err := d.cli(ctx, 30*time.Second, "images", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	out := []Image{}
	err = jsonLines(res.Stdout, func(raw []byte) error {
		var r struct{ ID, Repository, Tag, Size, CreatedSince string }
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		out = append(out, Image{ID: r.ID, Repository: r.Repository, Tag: r.Tag, Size: r.Size, Created: r.CreatedSince})
		return nil
	})
	return out, err
}

func (d *Docker) Pull(ctx context.Context, ref string) error {
	if !imageRefRe.MatchString(ref) {
		return fmt.Errorf("%w: image reference", ErrBadInput)
	}
	_, err := d.cli(ctx, 20*time.Minute, "pull", ref)
	return err
}

var imageIDRe = regexp.MustCompile(`^(sha256:)?[a-f0-9]{12,64}$`)

func (d *Docker) RemoveImage(ctx context.Context, id string) error {
	if !imageIDRe.MatchString(id) && !imageRefRe.MatchString(id) {
		return fmt.Errorf("%w: image", ErrBadInput)
	}
	_, err := d.cli(ctx, 2*time.Minute, "rmi", id)
	return err
}

// PruneImages removes dangling images; with all it also removes every image no container uses.
func (d *Docker) PruneImages(ctx context.Context, all bool) (string, error) {
	args := []string{"image", "prune", "-f"}
	if all {
		args = append(args, "-a")
	}
	res, err := d.cli(ctx, 10*time.Minute, args...)
	if err != nil {
		return "", err
	}
	return reclaimed(res.Stdout), nil
}

func reclaimed(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if r, ok := strings.CutPrefix(strings.TrimSpace(l), "Total reclaimed space: "); ok {
			return r
		}
	}
	return "0B"
}

// ---- volumes ----

type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
	Created    string `json:"created"`
	Containers string `json:"containers"`
}

func (d *Docker) ListVolumes(ctx context.Context) ([]Volume, error) {
	res, err := d.cli(ctx, 30*time.Second, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil, err
	}
	out := []Volume{}
	for _, name := range strings.Fields(res.Stdout) {
		if !containerRe.MatchString(name) {
			continue
		}
		v := Volume{Name: name}
		if r, err := d.cli(ctx, 15*time.Second, "volume", "inspect", "--format", "{{json .}}", name); err == nil {
			var m struct{ Driver, Mountpoint, CreatedAt string }
			if json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &m) == nil {
				v.Driver, v.Mountpoint, v.Created = m.Driver, m.Mountpoint, m.CreatedAt
			}
		}
		if r, err := d.cli(ctx, 15*time.Second, "ps", "-a", "--filter", "volume="+name, "--format", "{{.Names}}"); err == nil {
			v.Containers = strings.Join(strings.Fields(r.Stdout), ", ")
		}
		out = append(out, v)
	}
	return out, nil
}

// PruneVolumes removes volumes no container references (anonymous only on newer engines unless -a).
func (d *Docker) PruneVolumes(ctx context.Context) (string, error) {
	res, err := d.cli(ctx, 10*time.Minute, "volume", "prune", "-a", "-f")
	if err != nil {
		// engines before 23 reject -a and prune every unused volume by default
		if res, err = d.cli(ctx, 10*time.Minute, "volume", "prune", "-f"); err != nil {
			return "", err
		}
	}
	return reclaimed(res.Stdout), nil
}

// Load imports an image archive (docker save output, optionally gzipped) and returns the references it provided.
func (d *Docker) Load(ctx context.Context, path string) ([]string, error) {
	res, err := d.cli(ctx, 45*time.Minute, "load", "-i", path)
	if err != nil {
		return nil, err
	}
	refs := []string{}
	for _, l := range strings.Split(res.Stdout, "\n") {
		if r, ok := strings.CutPrefix(strings.TrimSpace(l), "Loaded image: "); ok {
			refs = append(refs, r)
		} else if r, ok := strings.CutPrefix(strings.TrimSpace(l), "Loaded image ID: "); ok {
			refs = append(refs, r)
		}
	}
	return refs, nil
}
