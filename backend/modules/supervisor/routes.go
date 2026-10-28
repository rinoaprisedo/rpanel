package supervisor

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/supervisor")
	g.GET("/installed", guard(module, "view"), h.Installed)
	g.GET("/status", guard(module, "view"), h.Status)
	g.GET("/programs/:name/log", guard(module, "view"), h.Log)
	g.POST("/programs", guard(module, "create"), h.Create)
	g.PUT("/programs/:name", guard(module, "edit"), h.Update)
	g.POST("/programs/:name/:action", guard(module, "edit"), h.Action)
	g.DELETE("/programs/:name", guard(module, "delete"), h.Delete)
	g.POST("/service/:action", guard(module, "edit"), h.Daemon)
}
