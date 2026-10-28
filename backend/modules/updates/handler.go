package updates

import (
	"errors"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "updates"

type Handler struct{ svc *Service }

func NewHandler() *Handler { return &Handler{svc: NewService(system.Default, DefaultPaths())} }

// Service exposes the service so other modules can register callbacks.
func (h *Handler) Service() *Service { return h.svc }

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrBusy):
		utils.Error(c, 409, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

// Status godoc
// @Summary		Pending package updates, reboot flag and automatic-update setting
// @Tags			updates
// @Security		SessionCookie
// @Router			/updates [get]
func (h *Handler) Status(c *gin.Context) {
	utils.Success(c, 200, "ok", h.svc.Status(c.Request.Context()))
}

// Progress godoc
// @Summary		Progress of the latest update job
// @Tags			updates
// @Security		SessionCookie
// @Router			/updates/progress [get]
func (h *Handler) Progress(c *gin.Context) {
	p := h.svc.Progress()
	if p == nil {
		utils.Error(c, 404, "no update job yet")
		return
	}
	utils.Success(c, 200, "ok", p)
}

// Start godoc
// @Summary		Start a job: check | security | all
// @Tags			updates
// @Security		SessionCookie
// @Router			/updates/run [post]
func (h *Handler) Start(c *gin.Context) {
	var in struct {
		Kind string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "kind is required")
		return
	}
	p, err := h.svc.Start(in.Kind)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, "", gin.H{"run": in.Kind})
	utils.Success(c, 202, "started", p)
}

// Auto godoc
// @Summary		Enable or disable automatic security updates
// @Tags			updates
// @Security		SessionCookie
// @Router			/updates/auto [put]
func (h *Handler) Auto(c *gin.Context) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "enabled is required")
		return
	}
	if err := h.svc.SetAutoSecurity(c.Request.Context(), in.Enabled); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, "", gin.H{"auto_security": in.Enabled})
	utils.Success(c, 200, "saved", h.svc.Status(c.Request.Context()))
}

// Reboot godoc
// @Summary		Restart the server
// @Tags			updates
// @Security		SessionCookie
// @Router			/updates/reboot [post]
func (h *Handler) Reboot(c *gin.Context) {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&in)
	if err := h.svc.Reboot(in.Confirm); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, "", gin.H{"reboot": true})
	utils.Success(c, 202, "the server is restarting", nil)
}
