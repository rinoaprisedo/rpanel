package terminal

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Handler is the HTTP layer for the web terminal.
type Handler struct {
	ctx         context.Context
	shell       string
	shellArgs   []string
	idle        time.Duration
	maxSessions int
	origins     []string // allowed browser origins (FRONTEND_URL)

	mu     sync.Mutex
	active map[uint64]int
}

func NewHandler(ctx context.Context, shell string, idleMinutes, maxSessions int, frontendURLs string) *Handler {
	if idleMinutes < 1 {
		idleMinutes = 15
	}
	if maxSessions < 1 {
		maxSessions = 3
	}
	var origins []string
	for _, o := range strings.Split(frontendURLs, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return &Handler{
		ctx: ctx, shell: shell, shellArgs: []string{"-l"}, idle: time.Duration(idleMinutes) * time.Minute,
		maxSessions: maxSessions, origins: origins, active: map[uint64]int{},
	}
}

// originAllowed defends against cross-site WebSocket hijacking: cookies are
// sent on WS handshakes and CSRF tokens are not, so the Origin header is the
// only barrier. Missing Origin is rejected.
func (h *Handler) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	for _, o := range h.origins {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func (h *Handler) acquire(uid uint64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[uid] >= h.maxSessions {
		return false
	}
	h.active[uid]++
	return true
}

func (h *Handler) release(uid uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[uid] <= 1 {
		delete(h.active, uid)
		return
	}
	h.active[uid]--
}

// Connect godoc
// @Summary		Open an interactive shell (WebSocket upgrade)
// @Tags			terminal
// @Security		SessionCookie
// @Router			/terminal/ws [get]
func (h *Handler) Connect(c *gin.Context) {
	if !h.originAllowed(c.Request) {
		utils.Error(c, 403, "origin not allowed")
		return
	}
	uidp := utils.CurrentUserID(c)
	if uidp == nil {
		utils.Error(c, 401, "authentication required")
		return
	}
	uid := *uidp
	if !h.acquire(uid) {
		utils.Error(c, 429, "too many open terminal sessions")
		return
	}
	defer h.release(uid)

	up := websocket.Upgrader{
		ReadBufferSize: 4096, WriteBufferSize: 8192,
		CheckOrigin: h.originAllowed,
	}
	conn, err := up.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already replied
	}

	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	ip, ua := c.ClientIP(), c.Request.UserAgent()
	started := time.Now()
	activity_logs.LogActivity(&uid, activity_logs.ActionLogin, "terminal", "", nil, gin.H{"event": "session_start"}, ip, ua)

	why, _ := Serve(h.ctx, conn, Options{
		Shell: h.shell, Args: h.shellArgs, Env: DefaultEnv(home), Dir: home, IdleTimeout: h.idle,
	})
	activity_logs.LogActivity(&uid, activity_logs.ActionLogout, "terminal", "", nil,
		gin.H{"event": "session_end", "reason": why, "seconds": int(time.Since(started).Seconds())}, ip, ua)
}
