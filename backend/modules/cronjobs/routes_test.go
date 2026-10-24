package cronjobs

import (
	"testing"

	"rpanel/backend/config"

	"github.com/gin-gonic/gin"
)

func TestRoutesRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allow := func(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
	NewHandler(nil, &config.Config{}).RegisterRoutes(gin.New().Group("/api/v1"), allow)
}
