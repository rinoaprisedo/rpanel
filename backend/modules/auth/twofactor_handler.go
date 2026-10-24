package auth

import (
	"errors"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/modules/users"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) currentUser(c *gin.Context) (*users.User, bool) {
	id := utils.CurrentUserID(c)
	if id == nil {
		utils.Error(c, 401, "authentication required")
		return nil, false
	}
	u, err := h.Service.Me(*id)
	if err != nil {
		utils.Error(c, 404, "user not found")
		return nil, false
	}
	return u, true
}

func twoFAFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidOTP), errors.Is(err, ErrWrongPassword):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrTwoFactorEnabled):
		utils.Error(c, 409, err.Error())
	case errors.Is(err, ErrTwoFactorNotStarted):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrTwoFactorNoSecret):
		utils.Error(c, 503, err.Error())
	default:
		utils.Error(c, 500, "two-factor operation failed")
	}
}

// TwoFASetup godoc
// @Summary		Start 2FA enrolment: returns the secret, otpauth URL and a QR code (PNG data URL)
// @Tags			auth
// @Security		SessionCookie
// @Router			/auth/2fa/setup [post]
func (h *Handler) TwoFASetup(c *gin.Context) {
	u, ok := h.currentUser(c)
	if !ok {
		return
	}
	en, err := h.Service.TwoFA.Begin(u)
	if err != nil {
		twoFAFail(c, err)
		return
	}
	utils.Success(c, 200, "scan the QR code, then confirm with a code", en)
}

type codeRequest struct {
	Code string `json:"code" binding:"required"`
}

// TwoFAEnable godoc
// @Summary		Confirm enrolment with a TOTP code; returns one-time recovery codes
// @Tags			auth
// @Security		SessionCookie
// @Router			/auth/2fa/enable [post]
func (h *Handler) TwoFAEnable(c *gin.Context) {
	var req codeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	u, ok := h.currentUser(c)
	if !ok {
		return
	}
	codes, err := h.Service.TwoFA.Enable(c.Request.Context(), u, req.Code)
	if err != nil {
		twoFAFail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, "auth", u.UUID.String(), gin.H{"event": "2fa_enabled"})
	utils.Success(c, 200, "two-factor authentication enabled", gin.H{"recovery_codes": codes})
}

type disableRequest struct {
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

// TwoFADisable godoc
// @Summary		Disable 2FA (requires password and a valid code)
// @Tags			auth
// @Security		SessionCookie
// @Router			/auth/2fa/disable [post]
func (h *Handler) TwoFADisable(c *gin.Context) {
	var req disableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	u, ok := h.currentUser(c)
	if !ok {
		return
	}
	if !CheckPassword(u.Password, req.Password) {
		twoFAFail(c, ErrWrongPassword)
		return
	}
	if err := h.Service.TwoFA.Disable(c.Request.Context(), u, req.Code); err != nil {
		twoFAFail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, "auth", u.UUID.String(), gin.H{"event": "2fa_disabled"})
	utils.Success(c, 200, "two-factor authentication disabled", nil)
}

// TwoFARecovery godoc
// @Summary		Replace recovery codes (requires a valid code)
// @Tags			auth
// @Security		SessionCookie
// @Router			/auth/2fa/recovery-codes [post]
func (h *Handler) TwoFARecovery(c *gin.Context) {
	var req codeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	u, ok := h.currentUser(c)
	if !ok {
		return
	}
	codes, err := h.Service.TwoFA.RegenerateRecovery(c.Request.Context(), u, req.Code)
	if err != nil {
		twoFAFail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, "auth", u.UUID.String(), gin.H{"event": "2fa_recovery_regenerated"})
	utils.Success(c, 200, "recovery codes replaced", gin.H{"recovery_codes": codes})
}
