package dashboard

import (
	"context"
	"errors"
	"strconv"

	"rpanel/backend/config"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the HTTP layer for the dashboard module.
type Handler struct {
	collector *Collector
	analyzer  *Analyzer
	probes    *Probes
}

// NewHandler starts the background sampler and update checker (stopped when ctx is cancelled).
func NewHandler(ctx context.Context, db *gorm.DB, cfg *config.Config) *Handler {
	c := NewCollector()
	c.Start(ctx)
	h := &Handler{collector: c}
	h.probes = NewProbes(system.Default, db, "", nil)
	h.analyzer = NewAnalyzer(cfg.NginxLogDir, h.probes.Domains)
	h.probes.analyzer = h.analyzer
	h.probes.StartUpdateChecks(ctx)
	h.probes.StartReleaseChecks(ctx)
	return h
}

// RefreshUpdates recounts pending package updates in the background (called after an update job).
func (h *Handler) RefreshUpdates() {
	go h.probes.refreshUpdates(context.Background())
}

// Overview godoc
// @Summary		Current host metrics (CPU, memory, disk, network, load)
// @Tags			dashboard
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Router			/dashboard/overview [get]
func (h *Handler) Overview(c *gin.Context) {
	utils.Success(c, 200, "ok", h.collector.Overview())
}

// History godoc
// @Summary		Metric samples: the last ~5 minutes (3s apart), or averaged points for range 1h|6h|24h|7d
// @Tags			dashboard
// @Security		SessionCookie
// @Param			range	query	string	false	"1h|6h|24h|7d (history is kept in memory since the panel started)"
// @Success		200	{object}	utils.Response
// @Router			/dashboard/history [get]
func (h *Handler) History(c *gin.Context) {
	if r := c.Query("range"); r != "" {
		pts, err := h.collector.HistoryRange(r)
		if err != nil {
			utils.Error(c, 400, "unknown range")
			return
		}
		utils.Success(c, 200, "ok", pts)
		return
	}
	utils.Success(c, 200, "ok", h.collector.History())
}

// Traffic godoc
// @Summary		Website traffic analytics from the nginx access logs
// @Tags			dashboard
// @Security		SessionCookie
// @Param			range	query	string	false	"1h|6h|24h|7d, default 24h"
// @Param			site	query	string	false	"a managed domain; default all"
// @Success		200	{object}	utils.Response
// @Router			/dashboard/traffic [get]
func (h *Handler) Traffic(c *gin.Context) {
	t, err := h.analyzer.Traffic(c.DefaultQuery("range", "24h"), c.Query("site"))
	switch {
	case errors.Is(err, ErrBadRange), errors.Is(err, ErrUnknownSite):
		utils.Error(c, 400, err.Error())
	case err != nil:
		utils.Error(c, 500, "could not read the access logs")
	default:
		utils.Success(c, 200, "ok", t)
	}
}

// Insights godoc
// @Summary		Prioritised findings plus site, service, update and firewall status
// @Tags			dashboard
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Router			/dashboard/insights [get]
func (h *Handler) Insights(c *gin.Context) {
	utils.Success(c, 200, "ok", h.probes.Build(c.Request.Context(), h.collector))
}

// Processes godoc
// @Summary		Top processes by cpu or mem
// @Tags			dashboard
// @Security		SessionCookie
// @Param			sort	query	string	false	"cpu|mem"
// @Param			limit	query	int		false	"max 50, default 10"
// @Success		200	{object}	utils.Response
// @Router			/dashboard/processes [get]
func (h *Handler) Processes(c *gin.Context) {
	by := "cpu"
	if c.Query("sort") == "mem" {
		by = "mem"
	}
	n, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if n < 1 || n > 50 {
		n = 10
	}
	utils.Success(c, 200, "ok", TopProcesses(c.Request.Context(), by, n))
}
