package website

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	const m = "websites"
	g := rg.Group("/websites")
	g.GET("", guard(m, "view"), h.List)
	g.GET("/env", guard(m, "view"), h.Env)
	g.GET("/:uuid", guard(m, "view"), h.Get)
	g.GET("/:uuid/logs", guard(m, "view"), h.Logs)

	g.POST("", guard(m, "create"), h.Create)
	g.POST("/install", guard(m, "create"), h.Install)

	g.PUT("/:uuid", guard(m, "edit"), h.Update)
	g.POST("/:uuid/start", guard(m, "edit"), h.Start)
	g.POST("/:uuid/stop", guard(m, "edit"), h.Stop)
	g.POST("/:uuid/ssl", guard(m, "edit"), h.IssueSSL)

	g.DELETE("/:uuid", guard(m, "delete"), h.Delete)
}
