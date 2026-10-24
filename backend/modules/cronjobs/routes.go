package cronjobs

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	const m = "cron"
	g := rg.Group("/cron")
	g.GET("", guard(m, "view"), h.List)
	g.GET("/:uuid", guard(m, "view"), h.Get)
	g.GET("/:uuid/runs", guard(m, "view"), h.Runs)

	g.POST("", guard(m, "create"), h.Create)

	g.PUT("/:uuid", guard(m, "edit"), h.Update)
	g.POST("/:uuid/enable", guard(m, "edit"), h.Enable)
	g.POST("/:uuid/disable", guard(m, "edit"), h.Disable)
	g.POST("/:uuid/run", guard(m, "edit"), h.RunNow)

	g.DELETE("/:uuid", guard(m, "delete"), h.Delete)
}
