package activity_logs

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/activity-logs")
	g.GET("", guard("activity-logs", "view"), h.List)
	g.GET("/:uuid", guard("activity-logs", "view"), h.Get)
	g.DELETE("/cleanup", guard("activity-logs", "delete"), h.Cleanup)
}
