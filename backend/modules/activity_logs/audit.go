package activity_logs

import (
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// Audit records a system-level action (nginx reload, file delete, terminal
// session, ...) performed by the current user. detail should never contain
// secrets such as passwords.
func Audit(c *gin.Context, action ActivityAction, module, target string, detail interface{}) {
	LogActivity(utils.CurrentUserID(c), action, module, target, nil, detail, c.ClientIP(), c.Request.UserAgent())
}
