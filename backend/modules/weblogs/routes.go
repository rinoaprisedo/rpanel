package weblogs

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/logs")
	g.GET("/sites", guard(module, "view"), h.Sites)
	g.GET("/sites/:domain", guard(module, "view"), h.Read)
	g.GET("/sites/:domain/download", guard(module, "view"), h.Download)
	g.DELETE("/sites/:domain", guard(module, "delete"), h.Clear)
}
