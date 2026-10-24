package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const maxComposeSize = 256 << 10

var projectRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// Compose manages projects stored as <Dir>/<name>/docker-compose.yml.
type Compose struct {
	D   *Docker
	Dir string
}

type Project struct {
	Name string `json:"name"`
}

func (c *Compose) paths(name string) (dir, file string, err error) {
	if !projectRe.MatchString(name) {
		return "", "", fmt.Errorf("%w: project name (lowercase letters, digits, - and _)", ErrBadInput)
	}
	dir = filepath.Join(c.Dir, name)
	return dir, filepath.Join(dir, "docker-compose.yml"), nil
}

func (c *Compose) List() ([]Project, error) {
	items, err := os.ReadDir(c.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}
	out := []Project{}
	for _, it := range items {
		if !it.IsDir() || !projectRe.MatchString(it.Name()) {
			continue
		}
		if _, err := os.Stat(filepath.Join(c.Dir, it.Name(), "docker-compose.yml")); err == nil {
			out = append(out, Project{Name: it.Name()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (c *Compose) Get(name string) (string, error) {
	_, file, err := c.paths(name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return "", ErrNotFound
	}
	return string(b), err
}

// Save creates or replaces a project's compose file.
func (c *Compose) Save(name, content string, mustNotExist bool) error {
	dir, file, err := c.paths(name)
	if err != nil {
		return err
	}
	if len(content) == 0 || len(content) > maxComposeSize {
		return fmt.Errorf("%w: compose file must be 1 byte to %d KB", ErrBadInput, maxComposeSize>>10)
	}
	if _, err := os.Stat(file); err == nil && mustNotExist {
		return errors.New("a project with this name already exists")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(content), 0o600) // may contain secrets
}

func (c *Compose) cmd(ctx context.Context, name string, timeout time.Duration, sub ...string) (string, error) {
	dir, file, err := c.paths(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(file); err != nil {
		return "", ErrNotFound
	}
	args := append([]string{"compose", "-p", name, "--project-directory", dir, "-f", file}, sub...)
	res, err := c.D.cli(ctx, timeout, args...)
	return res.Stdout + res.Stderr, err
}

func (c *Compose) Up(ctx context.Context, name string) (string, error) {
	return c.cmd(ctx, name, 20*time.Minute, "up", "-d", "--remove-orphans")
}

func (c *Compose) Down(ctx context.Context, name string) (string, error) {
	return c.cmd(ctx, name, 5*time.Minute, "down")
}

func (c *Compose) PS(ctx context.Context, name string) (string, error) {
	return c.cmd(ctx, name, 30*time.Second, "ps", "-a")
}

func (c *Compose) Logs(ctx context.Context, name string, tail int) (string, error) {
	if tail < 1 || tail > 2000 {
		tail = 200
	}
	return c.cmd(ctx, name, 30*time.Second, "logs", "--no-color", "--tail", fmt.Sprint(tail))
}

// Delete brings the project down and removes its directory.
func (c *Compose) Delete(ctx context.Context, name string) error {
	dir, _, err := c.paths(name)
	if err != nil {
		return err
	}
	if _, err := c.Down(ctx, name); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return os.RemoveAll(dir)
}
