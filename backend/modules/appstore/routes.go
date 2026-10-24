package appstore

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/app-store")
	g.GET("/apps", guard(module, "view"), h.List)
	g.POST("/apps/:id/install", guard(module, "create"), h.Install)
	g.POST("/apps/:id/uninstall", guard(module, "delete"), h.Uninstall)
	g.POST("/apps/:id/service/:action", guard(module, "edit"), h.Control)
}
