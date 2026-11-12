package integrations

import (
	"errors"
	"strings"

	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const (
	ctxKeyID   = "api_key_uuid"
	ctxKeyDirs = "api_key_dirs"
)

// KeyAuth authenticates a request by its "Authorization: Bearer rpk_..." header and requires `scope`.
// The key's owner becomes the acting user (so the audit log names them), but the scope is the only authority granted.
func (s *Service) KeyAuth(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok {
			utils.Error(c, 401, "API key required")
			c.Abort()
			return
		}
		k, err := s.Authenticate(strings.TrimSpace(token), c.ClientIP(), scope)
		switch {
		case errors.Is(err, ErrScope):
			utils.Error(c, 403, err.Error())
			c.Abort()
			return
		case err != nil:
			utils.Error(c, 401, "invalid API key")
			c.Abort()
			return
		}
		c.Set(utils.CtxUserID, k.OwnerID)
		c.Set(utils.CtxIsSuperadmin, false)
		c.Set(ctxKeyID, k.UUID.String())
		c.Set(ctxKeyDirs, splitDirs(k.AllowedDirs))
		c.Next()
	}
}
