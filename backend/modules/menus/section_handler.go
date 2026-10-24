package menus

import (
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// ListSections godoc
// @Summary		List menu sections
// @Tags			menu-sections
// @Security		SessionCookie
// @Success		200	{object}	utils.Response
// @Router			/menu-sections [get]
func (h *Handler) ListSections(c *gin.Context) {
	list, err := h.Service.ListSections()
	if err != nil {
		utils.Error(c, 500, "failed to fetch menu sections")
		return
	}
	utils.Success(c, 200, "ok", list)
}

// GetSection godoc
// @Summary		Get a menu section
// @Tags			menu-sections
// @Security		SessionCookie
// @Param			uuid	path		string	true	"Menu section UUID"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/menu-sections/{uuid} [get]
func (h *Handler) GetSection(c *gin.Context) {
	section, err := h.Service.GetSection(c.Param("uuid"))
	if err != nil {
		utils.Error(c, 404, "menu section not found")
		return
	}
	utils.Success(c, 200, "ok", section)
}

type sectionRequest struct {
	Name  string `json:"name" binding:"required"`
	Icon  string `json:"icon"`
	Order int    `json:"order"`
}

// CreateSection godoc
// @Summary		Create a menu section
// @Tags			menu-sections
// @Security		SessionCookie
// @Param			body	body		sectionRequest	true	"Menu section payload"
// @Success		201		{object}	utils.Response
// @Router			/menu-sections [post]
func (h *Handler) CreateSection(c *gin.Context) {
	var req sectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	actorID := utils.CurrentUserID(c)
	section, err := h.Service.CreateSection(SectionInput{Name: req.Name, Icon: req.Icon, Order: req.Order, ActorID: actorID})
	if err != nil {
		utils.Error(c, 400, "failed to create menu section")
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionCreate, "menu_sections", section.UUID.String(), nil, section, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 201, "menu section created", section)
}

// UpdateSection godoc
// @Summary		Update a menu section
// @Tags			menu-sections
// @Security		SessionCookie
// @Param			uuid	path		string			true	"Menu section UUID"
// @Param			body	body		sectionRequest	true	"Menu section payload"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/menu-sections/{uuid} [put]
func (h *Handler) UpdateSection(c *gin.Context) {
	var req sectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	actorID := utils.CurrentUserID(c)
	before, after, err := h.Service.UpdateSection(c.Param("uuid"), SectionInput{Name: req.Name, Icon: req.Icon, Order: req.Order, ActorID: actorID})
	if err != nil {
		utils.Error(c, 404, "menu section not found")
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionUpdate, "menu_sections", after.UUID.String(), before, after, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "menu section updated", after)
}

// DeleteSection godoc
// @Summary		Delete a menu section
// @Tags			menu-sections
// @Security		SessionCookie
// @Param			uuid	path	string	true	"Menu section UUID"
// @Success		200		{object}	utils.Response
// @Failure		404		{object}	utils.Response
// @Router			/menu-sections/{uuid} [delete]
func (h *Handler) DeleteSection(c *gin.Context) {
	actorID := utils.CurrentUserID(c)
	section, err := h.Service.DeleteSection(c.Param("uuid"), actorID)
	if err != nil {
		utils.Error(c, 404, "menu section not found")
		return
	}
	activity_logs.LogActivity(actorID, activity_logs.ActionDelete, "menu_sections", section.UUID.String(), section, nil, c.ClientIP(), c.Request.UserAgent())
	utils.Success(c, 200, "menu section deleted", nil)
}

type sectionReorderItem struct {
	UUID  string `json:"uuid" binding:"required"`
	Order int    `json:"order"`
}

// ReorderSections godoc
// @Summary		Reorder menu sections
// @Tags			menu-sections
// @Security		SessionCookie
// @Param			body	body		[]sectionReorderItem	true	"Ordered list of section UUIDs"
// @Success		200		{object}	utils.Response
// @Router			/menu-sections/reorder [put]
func (h *Handler) ReorderSections(c *gin.Context) {
	var items []sectionReorderItem
	if err := c.ShouldBindJSON(&items); err != nil {
		utils.Error(c, 400, "invalid request payload")
		return
	}
	serviceItems := make([]SectionReorderItem, 0, len(items))
	for _, item := range items {
		serviceItems = append(serviceItems, SectionReorderItem{UUID: item.UUID, Order: item.Order})
	}
	h.Service.ReorderSections(serviceItems)
	utils.Success(c, 200, "reordered", nil)
}
