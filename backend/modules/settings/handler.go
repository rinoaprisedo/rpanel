package settings

import (
	"errors"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/storage"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Handler is the HTTP layer for the settings module.
type Handler struct {
	Service *Service
}

func NewHandler(db *gorm.DB, rdb *redis.Client, s storage.StorageInterface) *Handler {
	return &Handler{Service: NewService(NewRepository(db), rdb, s)}
}

// List godoc
// @Summary		List all settings as a key/value map
// @Tags			settings
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Router			/settings [get]
func (h *Handler) List(c *gin.Context) {
	result, err := h.Service.List()
	if err != nil {
		utils.Error(c, 500, "failed to fetch settings")
		return
	}
	utils.Success(c, 200, "ok", result)
}

// Public godoc
// @Summary		Get public branding settings (app name, logo, favicon, primary color)
// @Tags			settings
// @Success		200	{object}	utils.Response
// @Router			/settings/public [get]
func (h *Handler) Public(c *gin.Context) {
	result, err := h.Service.Public()
	if err != nil {
		utils.Error(c, 500, "failed to fetch settings")
		return
	}
	utils.Success(c, 200, "ok", result)
}

// BulkUpdate godoc
// @Summary		Upsert multiple settings
// @Tags			settings
// @Security		SessionCookie
// @Param			body	body		map[string]string	true	"Key/value settings payload"
// @Success		200		{object}	utils.Response
// @Router			/settings [put]
func (h *Handler) BulkUpdate(c *gin.Context) {
	var payload map[string]string
	if err := c.ShouldBindJSON(&payload); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}

	if err := h.Service.BulkUpdate(payload); err != nil {
		if errors.Is(err, ErrInvalidSetting) {
			utils.Error(c, 400, err.Error())
			return
		}
		utils.Error(c, 500, "failed to update settings")
		return
	}

	actorID := utils.CurrentUserID(c)
	activity_logs.LogActivity(actorID, activity_logs.ActionUpdate, "settings", "", nil, payload, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "settings updated", nil)
}

// Upload godoc
// @Summary		Upload app logo or favicon
// @Tags			settings
// @Security		SessionCookie
// @Accept			multipart/form-data
// @Param			target	formData	string	true	"app_logo or app_favicon"
// @Param			file	formData	file	true	"Image file"
// @Success		200		{object}	utils.Response
// @Failure		400		{object}	utils.Response
// @Router			/settings/upload [post]
func (h *Handler) Upload(c *gin.Context) {
	target := c.PostForm("target")

	fileHeader, err := c.FormFile("file")
	if err != nil {
		utils.Error(c, 400, "file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		utils.Error(c, 400, "failed to read uploaded file")
		return
	}
	defer file.Close()

	url, err := h.Service.Upload(target, file, fileHeader)
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.Success(c, 200, "file uploaded", gin.H{"url": url})
}
