package roles

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/roles")
	g.GET("", guard("roles", "view"), h.List)
	g.GET("/:uuid", guard("roles", "view"), h.Get)
	g.POST("", guard("roles", "create"), h.Create)
	g.PUT("/:uuid", guard("roles", "edit"), h.Update)
	g.DELETE("/:uuid", guard("roles", "delete"), h.Delete)
	g.GET("/:uuid/permissions", guard("roles", "view"), h.GetPermissions)
	g.PUT("/:uuid/permissions", guard("roles", "edit"), h.UpdatePermissions)
}
