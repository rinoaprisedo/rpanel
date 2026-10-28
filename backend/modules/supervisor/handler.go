package supervisor

import (
	"errors"
	"strconv"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "supervisor"

type Handler struct{ svc *Service }

func NewHandler() *Handler { return &Handler{svc: NewService(system.Default, DefaultPaths())} }

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrNotFound):
		utils.Error(c, 404, err.Error())
	case errors.Is(err, ErrNotInstalled):
		utils.Error(c, 409, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

func audit(c *gin.Context, a activity_logs.ActivityAction, target string, detail gin.H) {
	activity_logs.Audit(c, a, module, target, detail)
}

// Installed godoc
// @Summary		Whether Supervisor is installed (drives the sidebar entry)
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/installed [get]
func (h *Handler) Installed(c *gin.Context) {
	utils.Success(c, 200, "ok", gin.H{"installed": h.svc.Installed()})
}

// Status godoc
// @Summary		Supervisor state and every process
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/status [get]
func (h *Handler) Status(c *gin.Context) {
	utils.Success(c, 200, "ok", h.svc.Status(c.Request.Context()))
}

// Action godoc
// @Summary		start | stop | restart a program
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/programs/{name}/{action} [post]
func (h *Handler) Action(c *gin.Context) {
	out, err := h.svc.Action(c.Request.Context(), c.Param("action"), c.Param("name"))
	if err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, c.Param("name"), gin.H{"action": c.Param("action")})
	utils.Success(c, 200, "done", gin.H{"output": out})
}

// Log godoc
// @Summary		Tail of a program's stdout or stderr
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/programs/{name}/log [get]
func (h *Handler) Log(c *gin.Context) {
	n, _ := strconv.Atoi(c.DefaultQuery("bytes", "20000"))
	out, err := h.svc.Log(c.Request.Context(), c.Param("name"), c.DefaultQuery("stream", "stdout"), n)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"log": out})
}

// Create godoc
// @Summary		Add a program managed by the panel
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/programs [post]
func (h *Handler) Create(c *gin.Context) {
	var in Config
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	cfg, err := h.svc.Save(c.Request.Context(), in, true)
	if err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionCreate, cfg.Name, gin.H{"command": cfg.Command, "user": cfg.User})
	utils.Success(c, 201, "program added", cfg)
}

// Update godoc
// @Summary		Change a program managed by the panel
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/programs/{name} [put]
func (h *Handler) Update(c *gin.Context) {
	var in Config
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	in.Name = c.Param("name")
	cfg, err := h.svc.Save(c.Request.Context(), in, false)
	if err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, cfg.Name, gin.H{"command": cfg.Command, "user": cfg.User})
	utils.Success(c, 200, "program updated", cfg)
}

// Delete godoc
// @Summary		Remove a program managed by the panel
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/programs/{name} [delete]
func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("name")); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionDelete, c.Param("name"), nil)
	utils.Success(c, 200, "program removed", nil)
}

// Daemon godoc
// @Summary		Start, stop or restart supervisord
// @Tags			supervisor
// @Security		SessionCookie
// @Router			/supervisor/service/{action} [post]
func (h *Handler) Daemon(c *gin.Context) {
	if err := h.svc.Daemon(c.Request.Context(), c.Param("action")); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, "", gin.H{"service": c.Param("action")})
	utils.Success(c, 200, "done", nil)
}
