package utils

import "github.com/gin-gonic/gin"

const (
	CtxUserID       = "user_id"
	CtxUserUUID     = "user_uuid"
	CtxIsSuperadmin = "is_superadmin"
	CtxSessionToken = "session_token"
)

// CurrentUserID returns the authenticated user's numeric ID from context, if any.
func CurrentUserID(c *gin.Context) *uint64 {
	v, ok := c.Get(CtxUserID)
	if !ok {
		return nil
	}
	id, ok := v.(uint64)
	if !ok {
		return nil
	}
	return &id
}

func IsSuperadmin(c *gin.Context) bool {
	v, ok := c.Get(CtxIsSuperadmin)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}
