// Package system is the single gateway for running host commands and touching
// host paths. Modules must never call os/exec directly: every command goes
// through Run, which enforces a binary allowlist, argument validation and a
// timeout, and never invokes a shell.
package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout applies when the caller passes a zero timeout.
const DefaultTimeout = 30 * time.Second

// maxOutput caps captured stdout/stderr (each) to protect memory.
const maxOutput = 4 << 20

var ErrNotAllowed = errors.New("command not allowed")

// allowed lists the binaries panel modules may execute.
var allowed = map[string]bool{
	"nginx": true, "certbot": true, "systemd-run": true, "fail2ban-client": true, "supervisorctl": true, "systemctl": true, "ufw": true,
	"apt-get": true, "apt-cache": true, "dpkg": true, "crontab": true,
	"mysql": true, "mysqldump": true, "psql": true, "pg_dump": true, "pg_restore": true,
	"docker": true, "zip": true, "unzip": true, "tar": true,
	"chmod": true, "runuser": true, "chown": true, "ln": true, "id": true,
}

// Result is the outcome of a command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner abstracts execution so services can be unit-tested with a fake.
type Runner interface {
	Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error)
}

// Exec is the real Runner.
type Exec struct{}

type capBuffer struct{ bytes.Buffer }

func (b *capBuffer) Write(p []byte) (int, error) {
	if b.Len() < maxOutput {
		if room := maxOutput - b.Len(); len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil // never fail the child on a full buffer
}

// Run executes an allowlisted binary with the given args (no shell). A non-zero
// exit returns the Result together with a non-nil error.
func (e Exec) Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error) {
	return e.RunEnv(ctx, timeout, nil, name, args...)
}

// EnvRunner is implemented by runners that can pass extra environment
// variables (used for secrets such as MYSQL_PWD, which must not appear in argv).
type EnvRunner interface {
	RunEnv(ctx context.Context, timeout time.Duration, env []string, name string, args ...string) (Result, error)
}

// RunEnv is like Run with additional KEY=VALUE environment entries.
func (Exec) RunEnv(ctx context.Context, timeout time.Duration, env []string, name string, args ...string) (Result, error) {
	if !allowed[name] {
		return Result{ExitCode: -1}, fmt.Errorf("%w: %q", ErrNotAllowed, name)
	}
	for _, a := range args {
		if strings.ContainsRune(a, 0) {
			return Result{ExitCode: -1}, errors.New("argument contains NUL byte")
		}
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C"), env...)
	var out, errb capBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	if err != nil {
		res.ExitCode = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			return res, fmt.Errorf("%s: timed out after %s", name, timeout)
		}
		return res, fmt.Errorf("%s: %w", name, err)
	}
	return res, nil
}

// LineRunner is implemented by runners that can hand over output while the command runs
// (used for long jobs such as apt upgrades, to show live progress).
type LineRunner interface {
	RunLines(ctx context.Context, timeout time.Duration, env []string, onLine func(string), name string, args ...string) (Result, error)
}

// lineWriter splits a byte stream into lines on \n and \r (apt redraws progress with \r).
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	emit func(string)
	all  capBuffer
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.all.Write(p)
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		if line := strings.TrimSpace(string(w.buf[:i])); line != "" {
			w.emit(line)
		}
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 1<<16 {
		w.buf = w.buf[:0] // a runaway line without a terminator
	}
	return len(p), nil
}

// RunLines is like RunEnv but calls onLine for every output line (stdout and stderr merged) as it appears.
func (Exec) RunLines(ctx context.Context, timeout time.Duration, env []string, onLine func(string), name string, args ...string) (Result, error) {
	if !allowed[name] {
		return Result{ExitCode: -1}, fmt.Errorf("%w: %q", ErrNotAllowed, name)
	}
	for _, a := range args {
		if strings.ContainsRune(a, 0) {
			return Result{ExitCode: -1}, errors.New("argument contains NUL byte")
		}
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C"), env...)
	w := &lineWriter{emit: onLine}
	cmd.Stdout, cmd.Stderr = w, w
	err := cmd.Run()
	out := w.all.String()
	res := Result{Stdout: out, Stderr: ""}
	if err != nil {
		res.ExitCode = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			return res, fmt.Errorf("%s: timed out after %s", name, timeout)
		}
		return res, fmt.Errorf("%s: %w", name, err)
	}
	return res, nil
}

// RunLines runs through r with live output when r supports it, otherwise it runs normally and
// replays the output afterwards.
func RunLines(ctx context.Context, r Runner, timeout time.Duration, env []string, onLine func(string), name string, args ...string) (Result, error) {
	if lr, ok := r.(LineRunner); ok {
		return lr.RunLines(ctx, timeout, env, onLine, name, args...)
	}
	var res Result
	var err error
	if er, ok := r.(EnvRunner); ok {
		res, err = er.RunEnv(ctx, timeout, env, name, args...)
	} else {
		res, err = r.Run(ctx, timeout, name, args...)
	}
	for _, l := range strings.Split(res.Stdout+"\n"+res.Stderr, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			onLine(l)
		}
	}
	return res, err
}

// Default is the process-wide Runner; tests may replace it.
var Default Runner = Exec{}

// Run is shorthand for Default.Run.
func Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error) {
	return Default.Run(ctx, timeout, name, args...)
}

// RunEnv runs through r when it supports environment passing.
func RunEnv(ctx context.Context, r Runner, timeout time.Duration, env []string, name string, args ...string) (Result, error) {
	er, ok := r.(EnvRunner)
	if !ok {
		return Result{ExitCode: -1}, errors.New("runner does not support environment variables")
	}
	return er.RunEnv(ctx, timeout, env, name, args...)
}
