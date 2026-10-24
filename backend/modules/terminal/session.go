// Package terminal serves an interactive PTY shell over a WebSocket.
//
// Wire protocol: binary frames carry raw terminal bytes in both directions;
// text frames from the client carry JSON control messages, currently
// {"type":"resize","cols":N,"rows":N}.
package terminal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const (
	maxFrame    = 64 << 10
	pingEvery   = 30 * time.Second
	writeWait   = 10 * time.Second
	pongTimeout = 75 * time.Second
)

// Options configure one terminal session.
type Options struct {
	Shell       string
	Args        []string
	Env         []string
	Dir         string
	IdleTimeout time.Duration
}

type controlMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// Serve runs a shell attached to conn until either side ends or the session
// is idle for too long. It always reaps the child process. The returned string
// is the reason the session ended.
func Serve(ctx context.Context, conn *websocket.Conn, o Options) (string, error) {
	conn.SetReadLimit(maxFrame)

	cmd := exec.Command(o.Shell, o.Args...)
	cmd.Env = o.Env
	cmd.Dir = o.Dir
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24}) // sets Setsid
	if err != nil {
		return "start failed", err
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			ptmx.Close()
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			cmd.Wait()
		})
	}
	defer stop()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wmu sync.Mutex // gorilla permits one concurrent writer
	write := func(typ int, b []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		return conn.WriteMessage(typ, b)
	}

	reason := make(chan string, 3)
	idle := time.NewTimer(o.IdleTimeout)
	defer idle.Stop()

	// PTY -> client
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := write(websocket.BinaryMessage, buf[:n]); werr != nil {
					reason <- "client write failed"
					return
				}
			}
			if err != nil {
				reason <- "shell exited"
				return
			}
		}
	}()

	// client -> PTY
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})
	go func() {
		for {
			typ, data, err := conn.ReadMessage()
			if err != nil {
				reason <- "client disconnected"
				return
			}
			// User input (not pings) keeps the session alive.
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(o.IdleTimeout)
			switch typ {
			case websocket.BinaryMessage:
				if _, err := ptmx.Write(data); err != nil {
					reason <- "shell exited"
					return
				}
			case websocket.TextMessage:
				var m controlMsg
				if json.Unmarshal(data, &m) == nil && m.Type == "resize" && m.Cols > 0 && m.Rows > 0 && m.Cols < 1000 && m.Rows < 1000 {
					_ = pty.Setsize(ptmx, &pty.Winsize{Cols: m.Cols, Rows: m.Rows})
				}
			}
		}
	}()

	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	var why string
loop:
	for {
		select {
		case why = <-reason:
			break loop
		case <-idle.C:
			why = "idle timeout"
			_ = write(websocket.TextMessage, []byte("\r\n\x1b[33m[session closed: idle timeout]\x1b[0m\r\n"))
			break loop
		case <-ping.C:
			if err := write(websocket.PingMessage, nil); err != nil {
				why = "client write failed"
				break loop
			}
		case <-ctx.Done():
			why = "server shutdown"
			break loop
		}
	}
	stop()
	wmu.Lock()
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, why))
	wmu.Unlock()
	conn.Close()
	return why, nil
}

// DefaultEnv builds a minimal, predictable environment for the shell.
func DefaultEnv(home string) []string {
	return []string{
		"TERM=xterm-256color",
		"LANG=C.UTF-8",
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"SHELL=/bin/bash",
	}
}
