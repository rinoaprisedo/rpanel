package cronjobs

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const module = "cron"

// APIPrefix is exempt from the global tag-stripping JSON sanitizer: shell
// scripts legitimately contain < > & characters.
const APIPrefix = "/api/v1/cron"

type Handler struct{ svc *Service }

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	return &Handler{svc: NewService(db, cfg.CronDir, cfg.CronDataDir)}
}

// Service exposes the service for the CLI entry point and startup sync.
func (h *Handler) Service() *Service { return h.svc }

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		utils.Error(c, 404, err.Error())
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

// List godoc
// @Summary		List cron jobs
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron [get]
func (h *Handler) List(c *gin.Context) {
	out, err := h.svc.List()
	if err != nil {
		utils.Error(c, 500, "failed to list jobs")
		return
	}
	utils.Success(c, 200, "ok", out)
}

// Get godoc
// @Summary		Get a cron job
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron/{uuid} [get]
func (h *Handler) Get(c *gin.Context) {
	j, err := h.svc.Get(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", j)
}

// Create godoc
// @Summary		Create a cron job (shell script or URL)
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron [post]
func (h *Handler) Create(c *gin.Context) {
	var in Input
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	in.ActorID = utils.CurrentUserID(c)
	j, err := h.svc.Create(in)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, j.UUID.String(), gin.H{"name": j.Name, "type": j.Type, "schedule": j.Schedule, "command": j.Command})
	utils.Success(c, 201, "job created", j)
}

// Update godoc
// @Summary		Update a cron job
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron/{uuid} [put]
func (h *Handler) Update(c *gin.Context) {
	var in Input
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	in.ActorID = utils.CurrentUserID(c)
	j, err := h.svc.Update(c.Param("uuid"), in)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, j.UUID.String(), gin.H{"name": j.Name, "type": j.Type, "schedule": j.Schedule, "command": j.Command})
	utils.Success(c, 200, "job updated", j)
}

func (h *Handler) toggle(c *gin.Context, enabled bool) {
	j, err := h.svc.SetEnabled(c.Param("uuid"), enabled, utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, j.UUID.String(), gin.H{"name": j.Name, "enabled": enabled})
	utils.Success(c, 200, "job updated", j)
}

// Enable godoc
// @Summary		Enable a job (writes its cron file)
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron/{uuid}/enable [post]
func (h *Handler) Enable(c *gin.Context) { h.toggle(c, true) }

// Disable godoc
// @Summary		Disable a job (removes its cron file)
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron/{uuid}/disable [post]
func (h *Handler) Disable(c *gin.Context) { h.toggle(c, false) }

// RunNow godoc
// @Summary		Run a job immediately in the background (poll /runs for the result)
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron/{uuid}/run [post]
func (h *Handler) RunNow(c *gin.Context) {
	j, err := h.svc.Get(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, j.UUID.String(), gin.H{"name": j.Name, "op": "run_now"})
	go func() {
		// Detached from the request: a manual run may outlive the HTTP call.
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(j.Timeout)*time.Second+time.Minute)
		defer cancel()
		h.svc.Execute(ctx, j, "manual")
	}()
	utils.Success(c, 202, "job started", nil)
}

// Runs godoc
// @Summary		Run history of a job (latest first)
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron/{uuid}/runs [get]
func (h *Handler) Runs(c *gin.Context) {
	n, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	out, err := h.svc.Runs(c.Param("uuid"), n)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

// Delete godoc
// @Summary		Delete a job, its cron file, script and history
// @Tags			cron
// @Security		SessionCookie
// @Router			/cron/{uuid} [delete]
func (h *Handler) Delete(c *gin.Context) {
	j, err := h.svc.Delete(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionDelete, module, j.UUID.String(), gin.H{"name": j.Name})
	utils.Success(c, 200, "job deleted", nil)
}

// SyncOnStart rewrites cron files so they point at the current binary.
func (h *Handler) SyncOnStart() {
	if err := h.svc.SyncAll(); err != nil {
		log.Printf("cron: sync on start: %v", err)
	}
}
