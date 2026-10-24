package terminal

import "github.com/gin-gonic/gin"

// Opening a shell is gated by the "create" permission on the terminal menu so
// that "view" alone never grants command execution.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	rg.GET("/terminal/ws", guard("terminal", "create"), h.Connect)
}
