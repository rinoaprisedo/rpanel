package middleware

import (
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequirePermission checks that the authenticated user's role(s) grant the
// given action on the given menu path. Superadmin always bypasses this check.
func RequirePermission(db *gorm.DB, menuPath, action string) gin.HandlerFunc {
	column := map[string]string{
		"view":   "can_view",
		"create": "can_create",
		"edit":   "can_edit",
		"delete": "can_delete",
	}[action]

	return func(c *gin.Context) {
		if utils.IsSuperadmin(c) {
			c.Next()
			return
		}
		userID := utils.CurrentUserID(c)
		if userID == nil {
			utils.Error(c, 401, "authentication required")
			c.Abort()
			return
		}

		var count int64
		db.Table("permissions").
			Joins("JOIN menus ON menus.id = permissions.menu_id").
			Joins("JOIN users ON users.role_id = permissions.role_id").
			Where("users.id = ? AND menus.path = ? AND permissions."+column+" = ?", *userID, menuPath, true).
			Count(&count)

		if count == 0 {
			utils.Error(c, 403, "you do not have permission to perform this action")
			c.Abort()
			return
		}
		c.Next()
	}
}
