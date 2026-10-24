package menus

import (
	"errors"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the HTTP layer for the menus module.
type Handler struct {
	Service *Service
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{Service: NewService(NewRepository(db))}
}

// List godoc
// @Summary		List menus grouped by section
// @Tags			menus
// @Security		SessionCookie
// @Param			is_active	query		string	false	"Filter by active state"
// @Param			section		query		string	false	"Filter by section UUID"
// @Success		200			{object}	utils.Response
// @Router			/menus [get]
func (h *Handler) List(c *gin.Context) {
	result, err := h.Service.Tree(c.Query("is_active"), c.Query("section"))
	if err != nil {
		utils.Error(c, 500, "failed to fetch menus")
		return
	}
	utils.Success(c, 200, "ok", result)
}

// Get godoc
// @Summary		Get a menu
// @Tags			menus
// @Security		SessionCookie
// @Param			uuid	path		string	true	"Menu UUID"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/menus/{uuid} [get]
func (h *Handler) Get(c *gin.Context) {
	menu, err := h.Service.Get(c.Param("uuid"))
	if err != nil {
		utils.Error(c, 404, "menu not found")
		return
	}
	utils.Success(c, 200, "ok", menu)
}

type menuRequest struct {
	MenuSectionUUID string `json:"menu_section_uuid" binding:"required"`
	Name            string `json:"name" binding:"required"`
	Icon            string `json:"icon"`
	Path            string `json:"path"`
	Order           int    `json:"order"`
	IsActive        bool   `json:"is_active"`
}

func toMenuInput(req menuRequest, actorID *uint64) MenuInput {
	return MenuInput{
		MenuSectionUUID: req.MenuSectionUUID,
		Name:            req.Name,
		Icon:            req.Icon,
		Path:            req.Path,
		Order:           req.Order,
		IsActive:        req.IsActive,
		ActorID:         actorID,
	}
}

// Create godoc
// @Summary		Create a menu
// @Tags			menus
// @Security		SessionCookie
// @Param			body	body		menuRequest	true	"Menu payload"
// @Success		201		{object}	utils.Response
// @Failure		400		{object}	utils.Response
// @Router			/menus [post]
func (h *Handler) Create(c *gin.Context) {
	var req menuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	actorID := utils.CurrentUserID(c)
	menu, err := h.Service.Create(toMenuInput(req, actorID))
	if err != nil {
		if errors.Is(err, ErrInvalidSectionRef) {
			utils.Error(c, 400, "invalid menu_section_uuid")
			return
		}
		utils.Error(c, 400, "failed to create menu")
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionCreate, "menus", menu.UUID.String(), nil, menu, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 201, "menu created", menu)
}

// Update godoc
// @Summary		Update a menu
// @Tags			menus
// @Security		SessionCookie
// @Param			uuid	path		string		true	"Menu UUID"
// @Param			body	body		menuRequest	true	"Menu payload"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/menus/{uuid} [put]
func (h *Handler) Update(c *gin.Context) {
	var req menuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	actorID := utils.CurrentUserID(c)
	before, after, err := h.Service.Update(c.Param("uuid"), toMenuInput(req, actorID))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			utils.Error(c, 404, "menu not found")
		case errors.Is(err, ErrInvalidSectionRef):
			utils.Error(c, 400, "invalid menu_section_uuid")
		default:
			utils.Error(c, 400, "failed to update menu")
		}
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionUpdate, "menus", after.UUID.String(), before, after, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "menu updated", after)
}

// Delete godoc
// @Summary		Delete a menu
// @Tags			menus
// @Security		SessionCookie
// @Param			uuid	path	string	true	"Menu UUID"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/menus/{uuid} [delete]
func (h *Handler) Delete(c *gin.Context) {
	actorID := utils.CurrentUserID(c)
	menu, err := h.Service.Delete(c.Param("uuid"), actorID)
	if err != nil {
		utils.Error(c, 404, "menu not found")
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionDelete, "menus", menu.UUID.String(), menu, nil, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "menu deleted", nil)
}

type reorderItem struct {
	UUID  string `json:"uuid" binding:"required"`
	Order int    `json:"order"`
}

// Reorder godoc
// @Summary		Reorder menus
// @Tags			menus
// @Security		SessionCookie
// @Param			body	body		[]reorderItem	true	"Ordered list of menu UUIDs"
// @Success		200		{object}	utils.Response
// @Router			/menus/reorder [put]
func (h *Handler) Reorder(c *gin.Context) {
	var items []reorderItem
	if err := c.ShouldBindJSON(&items); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	serviceItems := make([]ReorderItem, 0, len(items))
	for _, item := range items {
		serviceItems = append(serviceItems, ReorderItem{UUID: item.UUID, Order: item.Order})
	}
	h.Service.Reorder(serviceItems)
	utils.Success(c, 200, "reordered", nil)
}
