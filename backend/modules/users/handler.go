package users

import (
	"errors"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/storage"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Handler is the HTTP layer for the users module: it binds/validates
// requests, delegates business logic to Service, and shapes responses. It
// holds no direct DB access — that lives in Repository via Service.
type Handler struct {
	Cfg     *config.Config
	Service *Service
}

func NewHandler(db *gorm.DB, cfg *config.Config, s storage.StorageInterface, rdb *redis.Client) *Handler {
	return &Handler{
		Cfg:     cfg,
		Service: NewService(NewRepository(db), s, rdb),
	}
}

// List godoc
// @Summary		List users
// @Tags			users
// @Security		SessionCookie
// @Param			page		query		int		false	"Page number"
// @Param			limit		query		int		false	"Page size"
// @Param			search		query		string	false	"Search by name/email"
// @Param			status		query		string	false	"Filter by status"
// @Param			role		query		string	false	"Filter by role UUID"
// @Success		200			{object}	utils.Response
// @Router			/users [get]
func (h *Handler) List(c *gin.Context) {
	p := utils.GetPagination(c, "created_at")
	list, total, err := h.Service.List(p, c.Query("status"), c.Query("role"))
	if err != nil {
		utils.Error(c, 500, "failed to fetch users")
		return
	}
	utils.SuccessList(c, "ok", list, utils.BuildMeta(p.Page, p.Limit, total))
}

// Get godoc
// @Summary		Get a user by UUID
// @Tags			users
// @Security		SessionCookie
// @Param			uuid	path		string	true	"User UUID"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/users/{uuid} [get]
func (h *Handler) Get(c *gin.Context) {
	user, err := h.Service.Get(c.Param("uuid"))
	if err != nil {
		utils.Error(c, 404, "user not found")
		return
	}
	utils.Success(c, 200, "ok", user)
}

type createRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Status   string `json:"status"`
	RoleUUID string `json:"role_uuid"`
}

// Create godoc
// @Summary		Create a user
// @Tags			users
// @Security		SessionCookie
// @Param			body	body		createRequest	true	"User payload"
// @Success		201		{object}	utils.Response
// @Failure		400		{object}	utils.Response
// @Router			/users [post]
func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}

	actorID := utils.CurrentUserID(c)
	user, err := h.Service.Create(CreateInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		Status:   req.Status,
		RoleUUID: req.RoleUUID,
		ActorID:  actorID,

		ActorIsSuperadmin: utils.IsSuperadmin(c),
	})
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	activity_logs.LogActivity(actorID, activity_logs.ActionCreate, "users", user.UUID.String(), nil, user, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 201, "user created", user)
}

type updateRequest struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Status   string  `json:"status"`
	RoleUUID *string `json:"role_uuid"`
}

// Update godoc
// @Summary		Update a user
// @Tags			users
// @Security		SessionCookie
// @Param			uuid	path		string			true	"User UUID"
// @Param			body	body		updateRequest	true	"Fields to update"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/users/{uuid} [put]
func (h *Handler) Update(c *gin.Context) {
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}

	actorID := utils.CurrentUserID(c)
	in := UpdateInput{Name: req.Name, Email: req.Email, Password: req.Password, Status: req.Status, ActorID: actorID, ActorIsSuperadmin: utils.IsSuperadmin(c)}
	if req.RoleUUID != nil {
		in.RoleProvided = true
		in.RoleUUID = *req.RoleUUID
	}
	before, after, err := h.Service.Update(c.Param("uuid"), in)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			utils.Error(c, 404, "user not found")
		case errors.Is(err, ErrSuperadminLocked), errors.Is(err, ErrSuperadminRole):
			utils.Error(c, 403, err.Error())
		case errors.Is(err, ErrBadStatus), errors.Is(err, ErrWeakPassword):
			utils.Error(c, 400, err.Error())
		default:
			utils.Error(c, 400, "failed to update user")
		}
		return
	}

	activity_logs.LogActivity(actorID, activity_logs.ActionUpdate, "users", after.UUID.String(), before, after, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "user updated", after)
}

// Delete godoc
// @Summary		Delete a user
// @Tags			users
// @Security		SessionCookie
// @Param			uuid	path	string	true	"User UUID"
// @Success		200		{object}	utils.Response
// @Failure		403		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/users/{uuid} [delete]
func (h *Handler) Delete(c *gin.Context) {
	actorID := utils.CurrentUserID(c)
	user, err := h.Service.Delete(c.Param("uuid"), actorID, utils.IsSuperadmin(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			utils.Error(c, 404, "user not found")
		case errors.Is(err, ErrSuperadminLocked):
			utils.Error(c, 403, "only a superadmin can delete a superadmin user")
		case errors.Is(err, ErrSelfDelete):
			utils.Error(c, 403, "you cannot delete your own account")
		default:
			utils.Error(c, 500, "failed to delete user")
		}
		return
	}

	activity_logs.LogActivity(actorID, activity_logs.ActionDelete, "users", user.UUID.String(), user, nil, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "user deleted", nil)
}

type statusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateStatus godoc
// @Summary		Change a user's status
// @Tags			users
// @Security		SessionCookie
// @Param			uuid	path		string			true	"User UUID"
// @Param			body	body		statusRequest	true	"New status"
// @Success		200		{object}	utils.Response
// @Failure		403		{object}	utils.Response
// @Router			/users/{uuid}/status [put]
func (h *Handler) UpdateStatus(c *gin.Context) {
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}

	oldStatus, user, err := h.Service.UpdateStatus(c.Param("uuid"), req.Status)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			utils.Error(c, 404, "user not found")
		case errors.Is(err, ErrSuperadminLocked):
			utils.Error(c, 403, "superadmin status cannot be changed")
		case errors.Is(err, ErrBadStatus):
			utils.Error(c, 400, err.Error())
		default:
			utils.Error(c, 500, "failed to update status")
		}
		return
	}

	actorID := utils.CurrentUserID(c)
	activity_logs.LogActivity(actorID, activity_logs.ActionUpdate, "users", user.UUID.String(), gin.H{"status": oldStatus}, gin.H{"status": user.Status}, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "status updated", user)
}

// UploadAvatar godoc
// @Summary		Upload a user's avatar
// @Tags			users
// @Security		SessionCookie
// @Param			uuid	path	string	true	"User UUID"
// @Accept			multipart/form-data
// @Param			avatar	formData	file	true	"Avatar image"
// @Success		200		{object}	utils.Response
// @Router			/users/{uuid}/avatar [post]
func (h *Handler) UploadAvatar(c *gin.Context) {
	fileHeader, err := c.FormFile("avatar")
	if err != nil {
		utils.Error(c, 400, "avatar file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		utils.Error(c, 400, "failed to read uploaded file")
		return
	}
	defer file.Close()

	url, err := h.Service.UploadAvatar(c.Param("uuid"), file, fileHeader, utils.IsSuperadmin(c))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			utils.Error(c, 404, "user not found")
			return
		}
		utils.Error(c, 400, err.Error())
		return
	}
	utils.Success(c, 200, "avatar uploaded", gin.H{"avatar": url})
}
