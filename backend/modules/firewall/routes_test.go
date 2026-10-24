package firewall

import (
	"testing"

	"rpanel/backend/config"

	"github.com/gin-gonic/gin"
)

func TestRoutesRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allow := func(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
	NewHandler(&config.Config{AppPort: "8800"}).RegisterRoutes(gin.New().Group("/api/v1"), allow)
}
