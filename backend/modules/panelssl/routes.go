package panelssl

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/panel-ssl")
	g.GET("", guard(module, "view"), h.Status)
	g.POST("/letsencrypt", guard(module, "edit"), h.Issue)
	g.POST("/custom", guard(module, "edit"), h.Custom)
}
