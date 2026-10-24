package middleware

import (
	"context"
	"time"

	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const maintenanceCacheKey = "settings:maintenance_mode"

// MaintenanceMode blocks all requests with 503 when maintenance_mode=true,
// except for superadmins. The setting is cached in Redis for 60s to avoid a
// DB hit on every request.
func MaintenanceMode(rdb *redis.Client, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()
		val, err := rdb.Get(ctx, maintenanceCacheKey).Result()
		if err != nil {
			var value string
			db.Table("settings").Select("value").Where("key = ?", "maintenance_mode").Row().Scan(&value)
			val = value
			rdb.Set(ctx, maintenanceCacheKey, val, 60*time.Second)
		}

		if val == "true" {
			if utils.IsSuperadmin(c) {
				c.Next()
				return
			}
			utils.Error(c, 503, "the application is currently under maintenance")
			c.Abort()
			return
		}
		c.Next()
	}
}
