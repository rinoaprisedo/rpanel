package terminal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func server(t *testing.T, idle time.Duration, max int) (*Handler, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewHandler(context.Background(), "/bin/sh", 1, max, "http://allowed.example")
	h.shellArgs = nil
	h.idle = idle
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(utils.CtxUserID, uint64(7)); c.Next() })
	r.GET("/ws", h.Connect)
	s := httptest.NewServer(r)
	t.Cleanup(s.Close)
	return h, s
}

func dial(s *httptest.Server, origin string) (*websocket.Conn, *http.Response, error) {
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	return websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http")+"/ws", hdr)
}

func readUntil(t *testing.T, c *websocket.Conn, want string, d time.Duration) string {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(d))
	var got strings.Builder
	for !strings.Contains(got.String(), want) {
		_, b, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %q, got %q: %v", want, got.String(), err)
		}
		got.Write(b)
	}
	return got.String()
}

func TestOriginPolicy(t *testing.T) {
	_, s := server(t, time.Minute, 3)
	if _, resp, err := dial(s, ""); err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatalf("missing origin must be 403, err=%v resp=%v", err, resp)
	}
	if _, resp, err := dial(s, "http://evil.example"); err == nil || resp.StatusCode != 403 {
		t.Fatalf("foreign origin must be 403")
	}
	c, _, err := dial(s, "http://allowed.example")
	if err != nil {
		t.Fatalf("configured origin should connect: %v", err)
	}
	c.Close()
	c2, _, err := dial(s, s.URL) // same-origin
	if err != nil {
		t.Fatalf("same origin should connect: %v", err)
	}
	c2.Close()
}

func TestShellEchoAndResize(t *testing.T) {
	_, s := server(t, time.Minute, 3)
	c, _, err := dial(s, "http://allowed.example")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":100,"rows":30}`))
	c.WriteMessage(websocket.BinaryMessage, []byte("echo MARK$((20+22))\n"))
	readUntil(t, c, "MARK42", 5*time.Second)
	c.WriteMessage(websocket.BinaryMessage, []byte("exit\n"))
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("expected normal close after exit, got %v", err)
			}
			return
		}
	}
}

func TestIdleTimeoutAndSessionLimit(t *testing.T) {
	h, s := server(t, 400*time.Millisecond, 1)
	c, _, err := dial(s, "http://allowed.example")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// second concurrent session for same user is refused
	if _, resp, err := dial(s, "http://allowed.example"); err == nil || resp.StatusCode != 429 {
		t.Fatalf("want 429, err=%v", err)
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("expected close on idle, got %v", err)
			}
			break
		}
	}
	time.Sleep(200 * time.Millisecond)
	h.mu.Lock()
	n := len(h.active)
	h.mu.Unlock()
	if n != 0 {
		t.Fatalf("session slot leaked: %d", n)
	}
}
