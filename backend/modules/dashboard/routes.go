package dashboard

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/dashboard")
	g.GET("/overview", guard("/", "view"), h.Overview)
	g.GET("/history", guard("/", "view"), h.History)
	g.GET("/processes", guard("/", "view"), h.Processes)
}
