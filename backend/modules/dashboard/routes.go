package dashboard

import "github.com/gin-gonic/gin"

// The old single "Dashboard" menu was split in two: server metrics and website traffic have their own permission.
const (
	MenuServer  = "monitor-server"
	MenuTraffic = "monitor-traffic"
)

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/dashboard")
	g.GET("/overview", guard(MenuServer, "view"), h.Overview)
	g.GET("/history", guard(MenuServer, "view"), h.History)
	g.GET("/insights", guard(MenuServer, "view"), h.Insights)
	g.GET("/processes", guard(MenuServer, "view"), h.Processes)
	g.GET("/traffic", guard(MenuTraffic, "view"), h.Traffic)
}
