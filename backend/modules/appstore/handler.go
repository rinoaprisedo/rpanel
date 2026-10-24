package appstore

import (
	"errors"
	"io"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "app-store"

type Handler struct {
	svc *Service
}

func NewHandler() *Handler {
	return &Handler{svc: NewService(system.Default)}
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		utils.Error(c, 404, "app not found")
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrBadState):
		utils.Error(c, 409, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

type versionReq struct {
	Version string `json:"version"`
}

// version reads the optional {"version": "..."} body; apps without versions send none.
func version(c *gin.Context) (string, bool) {
	var req versionReq
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		utils.Error(c, 400, "invalid request")
		return "", false
	}
	return req.Version, true
}

// List godoc
// @Summary		Installable services with their install status
// @Tags			app-store
// @Security		SessionCookie
// @Router			/app-store/apps [get]
func (h *Handler) List(c *gin.Context) {
	utils.Success(c, 200, "ok", h.svc.List(c.Request.Context()))
}

// Install godoc
// @Summary		Install a service (optionally {"version": "8.4"}) in the background; poll the list for progress
// @Tags			app-store
// @Security		SessionCookie
// @Router			/app-store/apps/{id}/install [post]
func (h *Handler) Install(c *gin.Context) {
	v, ok := version(c)
	if !ok {
		return
	}
	a, err := h.svc.Install(c.Param("id"), v)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, a.ID, gin.H{"op": "install", "version": v})
	utils.Success(c, 202, "installing", nil)
}

// Uninstall godoc
// @Summary		Remove a service's packages (configuration and data are kept)
// @Tags			app-store
// @Security		SessionCookie
// @Router			/app-store/apps/{id}/uninstall [post]
func (h *Handler) Uninstall(c *gin.Context) {
	v, ok := version(c)
	if !ok {
		return
	}
	a, err := h.svc.Uninstall(c.Param("id"), v)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionDelete, module, a.ID, gin.H{"op": "uninstall", "version": v})
	utils.Success(c, 202, "removing", nil)
}

// Control godoc
// @Summary		Start, stop or restart an installed service (optionally {"version": "8.4"} for PHP)
// @Tags			app-store
// @Security		SessionCookie
// @Router			/app-store/apps/{id}/service/{action} [post]
func (h *Handler) Control(c *gin.Context) {
	v, ok := version(c)
	if !ok {
		return
	}
	action := c.Param("action")
	a, err := h.svc.Control(c.Request.Context(), c.Param("id"), v, action)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, a.ID, gin.H{"op": action, "version": v})
	utils.Success(c, 200, "ok", nil)
}
