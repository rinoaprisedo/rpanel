package roles

import (
	"errors"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the HTTP layer for the roles module: binds/validates requests
// and delegates business logic to Service.
type Handler struct {
	Service *Service
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{Service: NewService(NewRepository(db))}
}

// List godoc
// @Summary		List roles
// @Tags			roles
// @Security		SessionCookie
// @Param			page	query		int		false	"Page number"
// @Param			limit	query		int		false	"Page size"
// @Param			search	query		string	false	"Search by name"
// @Success		200		{object}	utils.Response
// @Router			/roles [get]
func (h *Handler) List(c *gin.Context) {
	p := utils.GetPagination(c, "created_at")
	list, total, err := h.Service.List(p)
	if err != nil {
		utils.Error(c, 500, "failed to fetch roles")
		return
	}
	utils.SuccessList(c, "ok", list, utils.BuildMeta(p.Page, p.Limit, total))
}

// Get godoc
// @Summary		Get a role with its permissions
// @Tags			roles
// @Security		SessionCookie
// @Param			uuid	path		string	true	"Role UUID"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/roles/{uuid} [get]
func (h *Handler) Get(c *gin.Context) {
	role, perms, err := h.Service.Get(c.Param("uuid"))
	if err != nil {
		utils.Error(c, 404, "role not found")
		return
	}
	utils.Success(c, 200, "ok", gin.H{"role": role, "permissions": perms})
}

type roleRequest struct {
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description"`
	IsSuperadmin bool   `json:"is_superadmin"`
}

// Create godoc
// @Summary		Create a role
// @Tags			roles
// @Security		SessionCookie
// @Param			body	body		roleRequest	true	"Role payload"
// @Success		201		{object}	utils.Response
// @Router			/roles [post]
func (h *Handler) Create(c *gin.Context) {
	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	actorID := utils.CurrentUserID(c)
	role, err := h.Service.Create(RoleInput{Name: req.Name, Description: req.Description, IsSuperadmin: req.IsSuperadmin, ActorID: actorID, ActorIsSuperadmin: utils.IsSuperadmin(c)})
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionCreate, "roles", role.UUID.String(), nil, role, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 201, "role created", role)
}

// Update godoc
// @Summary		Update a role
// @Tags			roles
// @Security		SessionCookie
// @Param			uuid	path		string		true	"Role UUID"
// @Param			body	body		roleRequest	true	"Role payload"
// @Success		200		{object}	utils.Response
// @Failure		403		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/roles/{uuid} [put]
func (h *Handler) Update(c *gin.Context) {
	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	actorID := utils.CurrentUserID(c)
	before, after, err := h.Service.Update(c.Param("uuid"), RoleInput{Name: req.Name, Description: req.Description, IsSuperadmin: req.IsSuperadmin, ActorID: actorID, ActorIsSuperadmin: utils.IsSuperadmin(c)})
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			utils.Error(c, 404, "role not found")
		case errors.Is(err, ErrSuperadminLocked):
			utils.Error(c, 403, "superadmin role cannot be demoted")
		default:
			utils.Error(c, 400, "failed to update role")
		}
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionUpdate, "roles", after.UUID.String(), before, after, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "role updated", after)
}

// Delete godoc
// @Summary		Delete a role
// @Tags			roles
// @Security		SessionCookie
// @Param			uuid	path	string	true	"Role UUID"
// @Success		200		{object}	utils.Response
// @Failure		403		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/roles/{uuid} [delete]
func (h *Handler) Delete(c *gin.Context) {
	actorID := utils.CurrentUserID(c)
	role, err := h.Service.Delete(c.Param("uuid"), actorID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			utils.Error(c, 404, "role not found")
		case errors.Is(err, ErrSuperadminLocked):
			utils.Error(c, 403, "superadmin role cannot be deleted")
		case errors.Is(err, ErrRoleInUse):
			utils.Error(c, 409, err.Error())
		default:
			utils.Error(c, 500, "failed to delete role")
		}
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionDelete, "roles", role.UUID.String(), role, nil, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "role deleted", nil)
}

// GetPermissions godoc
// @Summary		Get a role's permissions
// @Tags			roles
// @Security		SessionCookie
// @Param			uuid	path		string	true	"Role UUID"
// @Success		200		{object}	utils.Response
// @Router			/roles/{uuid}/permissions [get]
func (h *Handler) GetPermissions(c *gin.Context) {
	perms, err := h.Service.Permissions(c.Param("uuid"))
	if err != nil {
		utils.Error(c, 404, "role not found")
		return
	}
	utils.Success(c, 200, "ok", perms)
}

type permissionUpdate struct {
	MenuUUID  string `json:"menu_uuid" binding:"required"`
	CanView   bool   `json:"can_view"`
	CanCreate bool   `json:"can_create"`
	CanEdit   bool   `json:"can_edit"`
	CanDelete bool   `json:"can_delete"`
}

type updatePermissionsRequest struct {
	Permissions []permissionUpdate `json:"permissions" binding:"required"`
}

// UpdatePermissions godoc
// @Summary		Replace a role's permissions
// @Tags			roles
// @Security		SessionCookie
// @Param			uuid	path		string						true	"Role UUID"
// @Param			body	body		updatePermissionsRequest	true	"Permissions payload"
// @Success		200		{object}	utils.Response
// @Router			/roles/{uuid}/permissions [put]
func (h *Handler) UpdatePermissions(c *gin.Context) {
	var req updatePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}

	items := make([]PermissionInput, 0, len(req.Permissions))
	for _, p := range req.Permissions {
		items = append(items, PermissionInput{MenuUUID: p.MenuUUID, CanView: p.CanView, CanCreate: p.CanCreate, CanEdit: p.CanEdit, CanDelete: p.CanDelete})
	}

	actorID := utils.CurrentUserID(c)
	if err := h.Service.UpdatePermissions(c.Param("uuid"), items, actorID, utils.IsSuperadmin(c)); err != nil {
		switch {
		case errors.Is(err, ErrSuperadminLocked), errors.Is(err, ErrOwnRole):
			utils.Error(c, 403, err.Error())
		default:
			utils.Error(c, 404, "role not found")
		}
		return
	}

	activity_logs.LogActivity(actorID, activity_logs.ActionUpdate, "role_permissions", c.Param("uuid"), nil, req.Permissions, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "permissions updated", nil)
}
