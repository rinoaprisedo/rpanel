package website

import (
	"testing"

	"rpanel/backend/config"

	"github.com/gin-gonic/gin"
)

// Gin panics at registration on conflicting wildcard routes (e.g. /env vs /:uuid).
func TestRoutesRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allow := func(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
	NewHandler(nil, &config.Config{}).RegisterRoutes(gin.New().Group("/api/v1"), allow)
}
