package firewall

import (
	"errors"
	"strconv"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "firewall"

type Handler struct{ svc *Service }

func NewHandler(cfg *config.Config) *Handler {
	return &Handler{svc: NewService(system.Default, cfg.SSHDConfig, cfg.AppPort, cfg.FirewallProtectedPorts)}
}

// fail maps errors; lockout warnings carry code "lockout" so the UI can ask for
// confirmation and retry with confirm=true.
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrLockout):
		c.JSON(409, gin.H{"success": false, "code": "lockout", "message": err.Error()})
	case errors.Is(err, ErrStale):
		c.JSON(409, gin.H{"success": false, "code": "stale", "message": err.Error()})
	default:
		utils.Error(c, 422, err.Error())
	}
}

func audit(c *gin.Context, a activity_logs.ActivityAction, detail gin.H) {
	activity_logs.Audit(c, a, module, "", detail)
}

// Status godoc
// @Summary		UFW status, default policy, rules and protected ports
// @Tags			firewall
// @Security		SessionCookie
// @Router			/firewall/status [get]
func (h *Handler) Status(c *gin.Context) {
	utils.Success(c, 200, "ok", h.svc.Status(c.Request.Context()))
}

// Install godoc
// @Summary		Install ufw via apt (does not enable it)
// @Tags			firewall
// @Security		SessionCookie
// @Router			/firewall/install [post]
func (h *Handler) Install(c *gin.Context) {
	if err := h.svc.Install(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionCreate, gin.H{"op": "install"})
	utils.Success(c, 200, "ufw installed", nil)
}

// Enable godoc
// @Summary		Enable the firewall (protected ports are allowed first)
// @Tags			firewall
// @Security		SessionCookie
// @Router			/firewall/enable [post]
func (h *Handler) Enable(c *gin.Context) {
	if err := h.svc.Enable(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, gin.H{"op": "enable"})
	utils.Success(c, 200, "firewall enabled", nil)
}

// Disable godoc
// @Summary		Disable the firewall
// @Tags			firewall
// @Security		SessionCookie
// @Router			/firewall/disable [post]
func (h *Handler) Disable(c *gin.Context) {
	if err := h.svc.Disable(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, gin.H{"op": "disable"})
	utils.Success(c, 200, "firewall disabled", nil)
}

type policyReq struct {
	Policy string `json:"policy" binding:"required"`
}

// SetDefault godoc
// @Summary		Set the default incoming policy (allow | deny | reject)
// @Tags			firewall
// @Security		SessionCookie
// @Router			/firewall/default [post]
func (h *Handler) SetDefault(c *gin.Context) {
	var in policyReq
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	if err := h.svc.SetDefaultIncoming(c.Request.Context(), in.Policy); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, gin.H{"op": "default_incoming", "policy": in.Policy})
	utils.Success(c, 200, "default policy updated", nil)
}

// AddRule godoc
// @Summary		Add an inbound rule
// @Tags			firewall
// @Security		SessionCookie
// @Router			/firewall/rules [post]
func (h *Handler) AddRule(c *gin.Context) {
	var in RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	if err := h.svc.AddRule(c.Request.Context(), in, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionCreate, gin.H{"op": "add_rule", "action": in.Action, "port": in.Port, "proto": in.Proto, "source": in.Source, "confirmed": in.Confirm})
	utils.Success(c, 201, "rule added", nil)
}

// DeleteRule godoc
// @Summary		Delete rule N (?to=&action= must match the current rule; confirm=1 overrides lockout warnings)
// @Tags			firewall
// @Security		SessionCookie
// @Router			/firewall/rules/{number} [delete]
func (h *Handler) DeleteRule(c *gin.Context) {
	n, err := strconv.Atoi(c.Param("number"))
	if err != nil {
		utils.Error(c, 400, "invalid rule number")
		return
	}
	to, action, confirm := c.Query("to"), c.Query("action"), c.Query("confirm") == "1"
	if err := h.svc.DeleteRule(c.Request.Context(), n, to, action, confirm); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionDelete, gin.H{"op": "delete_rule", "number": n, "to": to, "action": action, "confirmed": confirm})
	utils.Success(c, 200, "rule deleted", nil)
}
