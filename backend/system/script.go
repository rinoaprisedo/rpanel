package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"time"
)

var scriptPathRe = regexp.MustCompile(`^/[A-Za-z0-9._/-]+\.sh$`)

// RunScript executes an existing script file with /bin/bash. This is the one
// deliberate place where user-authored shell is executed (cron jobs); the
// script lives in a panel-owned directory and the process runs in its own
// process group so a timeout kills the whole tree.
func RunScript(ctx context.Context, timeout time.Duration, path string) (Result, error) {
	if !scriptPathRe.MatchString(path) {
		return Result{ExitCode: -1}, errors.New("invalid script path")
	}
	if timeout <= 0 {
		timeout = time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/bash", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = "/"
	cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8")
	var out, errb capBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	if err == nil {
		return res, nil
	}
	res.ExitCode = -1
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return res, fmt.Errorf("timed out after %s", timeout)
	}
	return res, err
}
