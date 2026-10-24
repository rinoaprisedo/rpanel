package settings

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc, uploadLimiter gin.HandlerFunc) {
	g := rg.Group("/settings")
	g.GET("/public", h.Public)
	g.GET("", guard("settings", "view"), h.List)
	g.PUT("", guard("settings", "edit"), h.BulkUpdate)
	g.POST("/upload", guard("settings", "edit"), uploadLimiter, h.Upload)
}
