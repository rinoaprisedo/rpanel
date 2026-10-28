package fail2ban

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/fail2ban")
	g.GET("/installed", guard(module, "view"), h.Installed)
	g.GET("/status", guard(module, "view"), h.Status)
	g.GET("/settings", guard(module, "view"), h.GetSettings)
	g.GET("/log", guard(module, "view"), h.Log)
	g.PUT("/settings", guard(module, "edit"), h.SaveSettings)
	g.POST("/jails/:jail/ban", guard(module, "edit"), h.Ban)
	g.POST("/jails/:jail/unban", guard(module, "edit"), h.Unban)
	g.POST("/service/:action", guard(module, "edit"), h.Control)
}
