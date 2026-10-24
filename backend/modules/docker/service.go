// Package docker manages containers, images and compose projects by driving
// the docker CLI through system.Runner (no shell, argv only).
package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if in.Restart != "" {
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
	return append(args, in.Image), nil
}

func (d *Docker) RunContainer(ctx context.Context, in RunInput) (string, error) {
	args, err := BuildRunArgs(in)
	if err != nil {
		return "", err
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
