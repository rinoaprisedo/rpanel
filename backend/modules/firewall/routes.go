package firewall

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	const m = "firewall"
	g := rg.Group("/firewall")
	g.GET("/status", guard(m, "view"), h.Status)
	g.POST("/install", guard(m, "create"), h.Install)
	g.POST("/rules", guard(m, "create"), h.AddRule)
	g.POST("/enable", guard(m, "edit"), h.Enable)
	g.POST("/disable", guard(m, "edit"), h.Disable)
	g.POST("/default", guard(m, "edit"), h.SetDefault)
	g.DELETE("/rules/:number", guard(m, "delete"), h.DeleteRule)
}
