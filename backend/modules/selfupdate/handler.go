package selfupdate

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(cfg *config.Config, current string) *Handler {
	dir := "/opt/rpanel"
	if cfg.PanelSSLDir != "" {
		dir = filepath.Dir(cfg.PanelSSLDir)
	}
	return &Handler{svc: NewService(system.Default, Settings{
		Repo: cfg.ReleaseRepo, Current: current, LogPath: filepath.Join(dir, "update.log"),
	})}
}

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
// @Summary		Newer RPanel release with its GitHub release notes, and the update job state
// @Tags			system
// @Security		SessionCookie
// @Router			/system/update [get]
func (h *Handler) Status(c *gin.Context) {
	ctx, cancel := ctxTimeout(c)
	defer cancel()
	info, err := h.svc.Status(ctx, c.Query("refresh") == "1")
	if err != nil {
		utils.Error(c, 502, "cannot check for updates right now")
		return
	}
	utils.Success(c, 200, "ok", info)
}

// Job godoc
// @Summary		State and log of the running update
// @Tags			system
// @Security		SessionCookie
// @Router			/system/update/job [get]
func (h *Handler) Job(c *gin.Context) {
	ctx, cancel := ctxTimeout(c)
	defer cancel()
	utils.Success(c, 200, "ok", gin.H{"current": h.svc.cfg.Current, "job": h.svc.JobOnly(ctx)})
}

// Start godoc
// @Summary		Update RPanel to the latest release (superadmin)
// @Tags			system
// @Security		SessionCookie
// @Router			/system/update [post]
func (h *Handler) Start(c *gin.Context) {
	ctx, cancel := ctxTimeout(c)
	defer cancel()
	if err := h.svc.Start(ctx); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, "system", "", gin.H{"rpanel_update": "started"})
	utils.Success(c, 202, "update started", nil)
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, sessionAuth, requireSuperadmin gin.HandlerFunc) {
	g := rg.Group("/system/update")
	g.GET("", sessionAuth, h.Status)
	g.GET("/job", sessionAuth, requireSuperadmin, h.Job)
	g.POST("", sessionAuth, requireSuperadmin, h.Start)
}

func ctxTimeout(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), 25*time.Second)
}
