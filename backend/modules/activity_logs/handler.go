package activity_logs

import (
	"rpanel/backend/config"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the HTTP layer for querying/cleaning up activity logs.
type Handler struct {
	Cfg     *config.Config
	Service *Service
}

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	return &Handler{Cfg: cfg, Service: NewService(NewRepository(db))}
}

// List godoc
// @Summary		List activity logs
// @Tags			activity-logs
// @Security		SessionCookie
// @Param			page		query		int		false	"Page number"
// @Param			limit		query		int		false	"Page size"
// @Param			user		query		string	false	"Filter by user name/email (partial match)"
// @Param			module		query		string	false	"Filter by module"
// @Param			action		query		string	false	"Filter by action"
// @Param			ip			query		string	false	"Filter by IP address (partial match)"
// @Param			date_from	query		string	false	"Filter from date (RFC3339)"
// @Param			date_to		query		string	false	"Filter to date (RFC3339)"
// @Success		200			{object}	utils.Response
// @Router			/activity-logs [get]
func (h *Handler) List(c *gin.Context) {
	p := utils.GetPagination(c, "created_at")
	f := ListFilter{
		User:      c.Query("user"),
		Module:    c.Query("module"),
		Action:    c.Query("action"),
		IPAddress: c.Query("ip"),
		DateFrom:  c.Query("date_from"),
		DateTo:    c.Query("date_to"),
	}
	list, total, err := h.Service.List(p, f)
	if err != nil {
		utils.Error(c, 500, "failed to fetch activity logs")
		return
	}
	utils.SuccessList(c, "ok", list, utils.BuildMeta(p.Page, p.Limit, total))
}

// Get godoc
// @Summary		Get an activity log entry
// @Tags			activity-logs
// @Security		SessionCookie
// @Param			uuid	path		string	true	"Activity log UUID"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/activity-logs/{uuid} [get]
func (h *Handler) Get(c *gin.Context) {
	entry, err := h.Service.Get(c.Param("uuid"))
	if err != nil {
		utils.Error(c, 404, "activity log not found")
		return
	}
	utils.Success(c, 200, "ok", entry)
}

// Cleanup godoc
// @Summary		Delete activity logs older than the retention window
// @Tags			activity-logs
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Router			/activity-logs/cleanup [delete]
func (h *Handler) Cleanup(c *gin.Context) {
	count, err := h.Service.Cleanup(h.Cfg.LogCleanupDays)
	if err != nil {
		utils.Error(c, 500, "failed to clean up activity logs")
		return
	}
	utils.Success(c, 200, "activity logs cleaned up", gin.H{"deleted": count})
}
