package fail2ban

import (
	"errors"
	"strconv"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "fail2ban"

type Handler struct{ svc *Service }

func NewHandler() *Handler { return &Handler{svc: NewService(system.Default, DefaultPaths())} }

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrNotInstalled):
		utils.Error(c, 409, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

func audit(c *gin.Context, a activity_logs.ActivityAction, detail gin.H) {
	activity_logs.Audit(c, a, module, "", detail)
}

// Installed godoc
// @Summary		Whether Fail2ban is installed (drives the sidebar entry)
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/installed [get]
func (h *Handler) Installed(c *gin.Context) {
	utils.Success(c, 200, "ok", gin.H{"installed": h.svc.Installed()})
}

// Status godoc
// @Summary		Fail2ban status with jails and banned IPs
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/status [get]
func (h *Handler) Status(c *gin.Context) {
	utils.Success(c, 200, "ok", h.svc.Status(c.Request.Context(), c.ClientIP()))
}

// GetSettings godoc
// @Summary		Fail2ban settings managed by the panel
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/settings [get]
func (h *Handler) GetSettings(c *gin.Context) {
	s, err := h.svc.Settings(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", s)
}

// SaveSettings godoc
// @Summary		Save Fail2ban settings and reload the daemon
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/settings [put]
func (h *Handler) SaveSettings(c *gin.Context) {
	var in Settings
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid settings")
		return
	}
	s, err := h.svc.Save(c.Request.Context(), in)
	if err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, gin.H{"change": "settings", "ban_time": s.BanTime, "find_time": s.FindTime, "max_retry": s.MaxRetry})
	utils.Success(c, 200, "settings saved", s)
}

type ipInput struct {
	IP string `json:"ip"`
}

// Ban godoc
// @Summary		Ban an IP address in a jail
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/jails/{jail}/ban [post]
func (h *Handler) Ban(c *gin.Context) {
	var in ipInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "ip is required")
		return
	}
	if err := h.svc.Ban(c.Request.Context(), c.Param("jail"), in.IP, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, gin.H{"ban": in.IP, "jail": c.Param("jail")})
	utils.Success(c, 200, "banned", nil)
}

// Unban godoc
// @Summary		Unban an IP address
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/jails/{jail}/unban [post]
func (h *Handler) Unban(c *gin.Context) {
	var in ipInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "ip is required")
		return
	}
	if err := h.svc.Unban(c.Request.Context(), c.Param("jail"), in.IP); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, gin.H{"unban": in.IP, "jail": c.Param("jail")})
	utils.Success(c, 200, "unbanned", nil)
}

// Control godoc
// @Summary		Start, stop, restart or reload the Fail2ban service
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/service/{action} [post]
func (h *Handler) Control(c *gin.Context) {
	if err := h.svc.Control(c.Request.Context(), c.Param("action")); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, gin.H{"service": c.Param("action")})
	utils.Success(c, 200, "done", nil)
}

// Log godoc
// @Summary		Tail of the Fail2ban log
// @Tags			fail2ban
// @Security		SessionCookie
// @Router			/fail2ban/log [get]
func (h *Handler) Log(c *gin.Context) {
	n, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	lines, err := h.svc.Log(n)
	if err != nil {
		utils.Error(c, 500, "cannot read the log")
		return
	}
	utils.Success(c, 200, "ok", gin.H{"lines": lines})
}
