package middleware

import (
	"context"

	"rpanel/backend/session"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SessionAuth validates the httpOnly session cookie against Redis and
// attaches the resolved user identity to the gin context.
func SessionAuth(rdb *redis.Client, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(session.CookieName)
		if err != nil || token == "" {
			utils.Error(c, 401, "authentication required")
			c.Abort()
			return
		}

		data, err := session.Get(context.Background(), rdb, token)
		if err != nil {
			utils.Error(c, 401, "session expired or invalid")
			c.Abort()
			return
		}

		var count int64
		db.Table("roles").
			Joins("JOIN users ON users.role_id = roles.id").
			Where("users.id = ? AND roles.is_superadmin = ?", data.UserID, true).
			Count(&count)
		isSuperadmin := count > 0

		c.Set(utils.CtxUserID, data.UserID)
		c.Set(utils.CtxUserUUID, data.UserUUID)
		c.Set(utils.CtxIsSuperadmin, isSuperadmin)
		c.Set(utils.CtxSessionToken, token)
		c.Next()
	}
}

var _ = uuid.Nil
