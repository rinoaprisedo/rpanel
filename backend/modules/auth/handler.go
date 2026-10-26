package auth

import (
	"errors"

	"rpanel/backend/config"
	"rpanel/backend/middleware"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/modules/users"
	"rpanel/backend/session"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Handler is the HTTP layer for the auth module: it binds/validates
// requests, sets/clears the session cookie, and delegates credential and
// password-policy logic to Service.
type Handler struct {
	Redis   *redis.Client
	Cfg     *config.Config
	Service *Service
}

func NewHandler(db *gorm.DB, rdb *redis.Client, cfg *config.Config) *Handler {
	return &Handler{
		Redis:   rdb,
		Cfg:     cfg,
		Service: NewService(NewRepository(db), rdb, cfg),
	}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	OTP      string `json:"otp"` // TOTP or recovery code; required when 2FA is enabled
}

// Login godoc
// @Summary		Log in and start a session
// @Tags			auth
// @Param			body	body		loginRequest	true	"Credentials"
// @Success		200		{object}	utils.Response
// @Failure		401		{object}	utils.Response
// @Router			/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}

	result, err := h.Service.Login(req.Email, req.Password, req.OTP, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, ErrAccountInactive):
			utils.Error(c, 403, err.Error())
		case errors.Is(err, ErrTwoFactorRequired):
			// Password was correct: ask for the second factor (no session yet, no failure counted).
			utils.Success(c, 200, "two-factor code required", gin.H{"two_factor_required": true})
		case errors.Is(err, ErrInvalidOTP):
			middleware.RegisterLoginFailure(c, h.Redis)
			activity_logs.LogActivity(nil, activity_logs.ActionLogin, "auth", "", nil, gin.H{"email": req.Email, "result": "failed", "reason": "invalid_2fa"}, c.ClientIP(), c.Request.UserAgent())
			utils.Error(c, 401, "invalid two-factor code")
		default:
			middleware.RegisterLoginFailure(c, h.Redis)
			activity_logs.LogActivity(nil, activity_logs.ActionLogin, "auth", "", nil, gin.H{"email": req.Email, "result": "failed"}, c.ClientIP(), c.Request.UserAgent())
			utils.Error(c, 401, "invalid email or password")
		}
		return
	}

	secure := h.Cfg.AppEnv == "production"
	c.SetSameSite(3) // strict
	c.SetCookie(session.CookieName, result.Token, h.Cfg.SessionMaxAge, "/", "", secure, true)

	middleware.ClearLoginFailures(c, h.Redis)
	activity_logs.LogActivity(&result.User.ID, activity_logs.ActionLogin, "auth", result.User.UUID.String(), nil, gin.H{"result": "success"}, c.ClientIP(), c.Request.UserAgent())

	utils.Success(c, 200, "login successful", h.profileResponse(result.User))
}

// Logout godoc
// @Summary		Log out and clear the session
// @Tags			auth
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Router			/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	token, _ := c.Cookie(session.CookieName)
	userID := utils.CurrentUserID(c)
	if token != "" && userID != nil {
		h.Service.Logout(token, userID)
		activity_logs.LogActivity(userID, activity_logs.ActionLogout, "auth", "", nil, nil, c.ClientIP(), c.Request.UserAgent())
	}
	c.SetCookie(session.CookieName, "", -1, "/", "", h.Cfg.AppEnv == "production", true)
	utils.Success(c, 200, "logout successful", nil)
}

// Me godoc
// @Summary		Get the current authenticated user
// @Tags			auth
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Failure		401	{object}	utils.Response
// @Router			/auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	userID := utils.CurrentUserID(c)
	if userID == nil {
		utils.Error(c, 401, "authentication required")
		return
	}
	user, err := h.Service.Me(*userID)
	if err != nil {
		utils.Error(c, 404, "user not found")
		return
	}
	utils.Success(c, 200, "ok", h.profileResponse(user))
}

func (h *Handler) profileResponse(u *users.User) gin.H {
	roleNames, isSuperadmin, perms := h.Service.ProfileDTO(u)
	return gin.H{
		"uuid":               u.UUID,
		"name":               u.Name,
		"email":              u.Email,
		"avatar":             u.Avatar,
		"status":             u.Status,
		"roles":              roleNames,
		"is_superadmin":      isSuperadmin,
		"permissions":        perms,
		"two_factor_enabled": u.TOTPEnabled,
	}
}

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ForgotPassword godoc
// @Summary		Request a password reset token
// @Tags			auth
// @Param			body	body		forgotPasswordRequest	true	"Email"
// @Success		200		{object}	utils.Response
// @Router			/auth/forgot-password [post]
func (h *Handler) ForgotPassword(c *gin.Context) {
	var req forgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	h.Service.ForgotPassword(req.Email)
	utils.Success(c, 200, "if the email exists, a reset link has been sent", nil)
}

type resetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ResetPassword godoc
// @Summary		Reset password using a reset token
// @Tags			auth
// @Param			body	body		resetPasswordRequest	true	"Token and new password"
// @Success		200		{object}	utils.Response
// @Failure		400		{object}	utils.Response
// @Router			/auth/reset-password [post]
func (h *Handler) ResetPassword(c *gin.Context) {
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	if err := h.Service.ResetPassword(req.Token, req.NewPassword); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			utils.Error(c, 404, err.Error())
			return
		}
		utils.Error(c, 400, err.Error())
		return
	}
	utils.Success(c, 200, "password reset successfully", nil)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangePassword godoc
// @Summary		Change the current user's password
// @Tags			auth
// @Security		SessionCookie
// @Param			body	body		changePasswordRequest	true	"Current and new password"
// @Success		200		{object}	utils.Response
// @Failure		400		{object}	utils.Response
// @Router			/auth/change-password [put]
func (h *Handler) ChangePassword(c *gin.Context) {
	userID := utils.CurrentUserID(c)
	if userID == nil {
		utils.Error(c, 401, "authentication required")
		return
	}
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	token, _ := c.Cookie(session.CookieName)
	if err := h.Service.ChangePassword(*userID, req.CurrentPassword, req.NewPassword, token); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			utils.Error(c, 404, err.Error())
			return
		}
		utils.Error(c, 400, err.Error())
		return
	}
	utils.Success(c, 200, "password changed successfully", nil)
}
