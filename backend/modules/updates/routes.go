package updates

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/updates")
	g.GET("", guard(module, "view"), h.Status)
	g.GET("/progress", guard(module, "view"), h.Progress)
	g.POST("/run", guard(module, "edit"), h.Start)
	g.PUT("/auto", guard(module, "edit"), h.Auto)
	g.POST("/reboot", guard(module, "edit"), h.Reboot)
}
