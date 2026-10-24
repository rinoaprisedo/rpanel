package dashboard

import (
	"context"
	"strconv"

	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// Handler is the HTTP layer for the dashboard module.
type Handler struct {
	collector *Collector
}

// NewHandler starts the background sampler (stopped when ctx is cancelled).
func NewHandler(ctx context.Context) *Handler {
	c := NewCollector()
	c.Start(ctx)
	return &Handler{collector: c}
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
// @Summary		Recent metric samples (about 5 minutes, 3s apart)
// @Tags			dashboard
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Router			/dashboard/history [get]
func (h *Handler) History(c *gin.Context) {
	utils.Success(c, 200, "ok", h.collector.History())
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
