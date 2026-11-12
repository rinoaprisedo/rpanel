package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// The "run commands after upload" API. Vibecoder sends the lines the user typed (docker load, compose up, a migration or
// seed executed inside a container); RPanel runs them in order and stops at the first failure.
//
// Nothing here goes through a shell: a line is split into arguments and handed to the docker binary directly, so
// `;`, `&&`, `|`, `$()` and redirections are refused instead of interpreted. Only a fixed set of docker subcommands is
// accepted, and every host path has to stay inside the deploy folders. `run` is not allowed (it can mount any host path
// or start a privileged container); migrations and seeds go through `exec` on a container that `compose up` started.

const (
	maxCommands   = 20
	maxLineLen    = 2000
	maxOutputKeep = 64 << 10
)

var (
	errCmd = errors.New("command not allowed")

	// docker <sub>
	dockerSubs = map[string]bool{
		"load": true, "tag": true, "pull": true, "images": true, "ps": true, "logs": true,
		"start": true, "stop": true, "restart": true, "exec": true, "compose": true,
	}
	// docker compose <sub>
	composeSubs = map[string]bool{
		"up": true, "down": true, "ps": true, "logs": true, "pull": true, "build": true,
		"start": true, "stop": true, "restart": true, "exec": true, "config": true,
	}
	// Options of `docker compose` (before the subcommand) that take a value.
	composeValueFlags = map[string]bool{
		"-p": true, "--project-name": true, "-f": true, "--file": true, "--project-directory": true,
		"--env-file": true, "--profile": true, "--ansi": true, "--progress": true, "--parallel": true,
	}
	// Options of `exec` (docker and compose) that take a value, so the container name is found correctly.
	execValueFlags = map[string]bool{
		"-e": true, "--env": true, "-u": true, "--user": true, "-w": true, "--workdir": true,
		"--env-file": true, "--detach-keys": true, "--index": true,
	}
	pathFlags = map[string]bool{"-f": true, "--file": true, "--project-directory": true, "--env-file": true}
)

// splitCommand cuts a line into arguments the way a shell would for plain words and quotes, but never expands anything.
// Unquoted shell operators are an error; inside quotes they are ordinary text (e.g. sh -c 'a && b' runs in the container).
func splitCommand(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	have := false
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, have = r, true
		case r == ' ' || r == '\t':
			if have {
				args, have = append(args, cur.String()), false
				cur.Reset()
			}
		case strings.ContainsRune(";&|<>$`()\\\n\r", r):
			return nil, fmt.Errorf("%w: %q is not supported (no shell here; put it inside quotes if it is meant for the container)", errCmd, string(r))
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("%w: unclosed quote", errCmd)
	}
	if have {
		args = append(args, cur.String())
	}
	return args, nil
}

// leading splits args into the options before the first positional argument and the rest.
func leading(args []string, valueFlags map[string]bool) (opts, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			return args[:i], args[i:]
		}
		if valueFlags[a] && !strings.Contains(a, "=") {
			i++
		}
	}
	return args, nil
}

func flagName(a string) string {
	if i := strings.Index(a, "="); i > 0 && strings.HasPrefix(a, "--") {
		return a[:i]
	}
	return a
}

// vet checks one command and returns the arguments to pass to the docker binary. dir (already resolved, may be empty) is
// the project folder used for compose commands that do not name one.
func (h *Deploy) vet(line, dir string) ([]string, error) {
	args, err := splitCommand(strings.TrimSpace(line))
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: empty command", errCmd)
	}
	if args[0] != "docker" {
		return nil, fmt.Errorf("%w: only docker commands run here", errCmd)
	}
	args = args[1:]
	if len(args) == 0 || !dockerSubs[args[0]] {
		return nil, fmt.Errorf("%w: allowed: docker %s", errCmd, joinKeys(dockerSubs))
	}
	sub, rest := args[0], args[1:]

	switch sub {
	case "load":
		if err := h.checkPaths(rest, map[string]bool{"-i": true, "--input": true}); err != nil {
			return nil, err
		}
	case "exec":
		if err := noPrivileged(rest); err != nil {
			return nil, err
		}
	case "compose":
		opts, after := leading(rest, composeValueFlags)
		if len(after) == 0 || !composeSubs[after[0]] {
			return nil, fmt.Errorf("%w: allowed: docker compose %s", errCmd, joinKeys(composeSubs))
		}
		if err := h.checkPaths(opts, pathFlags); err != nil {
			return nil, err
		}
		if after[0] == "exec" {
			if err := noPrivileged(after[1:]); err != nil {
				return nil, err
			}
		}
		if dir != "" && !hasAny(opts, "-f", "--file", "--project-directory") {
			rest = append([]string{"--project-directory", dir}, rest...)
		}
	}
	return append([]string{sub}, rest...), nil
}

// checkPaths makes sure every path-valued option stays inside the deploy folders.
func (h *Deploy) checkPaths(args []string, flags map[string]bool) error {
	for i := 0; i < len(args); i++ {
		name := flagName(args[i])
		if !flags[name] {
			continue
		}
		var v string
		if name != args[i] {
			v = args[i][len(name)+1:]
		} else if i+1 < len(args) {
			i++
			v = args[i]
		}
		if v == "" {
			return fmt.Errorf("%w: %s needs a path", errCmd, name)
		}
		if _, err := h.access.Resolve(v); err != nil {
			return fmt.Errorf("%w: %s is outside the deploy folders", errCmd, v)
		}
	}
	return nil
}

// noPrivileged refuses --privileged among the exec options (everything after the container name belongs to the
// container's own command and is left alone).
func noPrivileged(args []string) error {
	opts, _ := leading(args, execValueFlags)
	for _, a := range opts {
		if flagName(a) == "--privileged" {
			return fmt.Errorf("%w: --privileged is not allowed", errCmd)
		}
	}
	return nil
}

func hasAny(args []string, names ...string) bool {
	for _, a := range args {
		for _, n := range names {
			if flagName(a) == n {
				return true
			}
		}
	}
	return false
}

func joinKeys(m map[string]bool) string {
	var k []string
	for _, s := range []string{"load", "tag", "pull", "images", "ps", "logs", "start", "stop", "restart", "exec", "compose", "up", "down", "build", "config"} {
		if m[s] {
			k = append(k, s)
		}
	}
	return strings.Join(k, ", ")
}

func timeoutFor(args []string) time.Duration {
	for _, a := range args {
		switch a {
		case "load", "pull", "up", "build":
			return 45 * time.Minute
		}
	}
	return 10 * time.Minute
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxOutputKeep {
		s = "…" + s[len(s)-maxOutputKeep:]
	}
	return s
}

type runIn struct {
	Commands []string `json:"commands" binding:"required"`
	Dir      string   `json:"dir"`
}

type runOut struct {
	Command  string `json:"command"`
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
}

// RunCommands runs the commands in order and stops at the first one that fails or is refused. Every command is checked
// before the first one runs, so a typo in line 5 does not leave the deploy half done. The response lists what ran;
// `failed_at` is the 0-based index of the command that stopped the run (-1 when all succeeded). The caller can fix the
// line and send the commands again.
func (h *Deploy) RunCommands(c *gin.Context) {
	var in runIn
	if c.ShouldBindJSON(&in) != nil || len(in.Commands) == 0 {
		utils.Error(c, 400, "invalid request")
		return
	}
	if len(in.Commands) > maxCommands {
		utils.Error(c, 400, fmt.Sprintf("at most %d commands per request", maxCommands))
		return
	}
	dir := ""
	if strings.TrimSpace(in.Dir) != "" {
		var ok bool
		if dir, ok = h.dir(c, in.Dir); !ok {
			return
		}
	}
	plans := make([][]string, len(in.Commands))
	for i, line := range in.Commands {
		if len(line) > maxLineLen {
			utils.Error(c, 400, fmt.Sprintf("command %d is too long", i+1))
			return
		}
		args, err := h.vet(line, dir)
		if err != nil {
			utils.Error(c, 400, fmt.Sprintf("command %d: %s", i+1, strings.TrimPrefix(err.Error(), errCmd.Error()+": ")))
			return
		}
		plans[i] = args
	}

	ctx := context.WithoutCancel(c.Request.Context()) // a dropped connection must not kill a half-loaded deploy
	results := []runOut{}
	failed := -1
	for i, args := range plans {
		res, err := h.d.Run.Run(ctx, timeoutFor(args), "docker", args...)
		out := runOut{Command: strings.TrimSpace(in.Commands[i]), OK: err == nil, ExitCode: res.ExitCode, Output: tail(res.Stdout + res.Stderr)}
		if err != nil {
			out.Error = err.Error()
			failed = i
		}
		results = append(results, out)
		if failed >= 0 {
			break
		}
	}
	audit(c, activity_logs.ActionCreate, "docker", gin.H{"op": "run", "commands": in.Commands, "failed_at": failed})
	utils.Success(c, 200, "done", gin.H{"results": results, "failed_at": failed})
}
