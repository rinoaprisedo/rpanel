package terminal

import (
	"errors"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// TemplateAPIPrefix is exempt from the global tag-stripping JSON sanitizer:
// shell commands legitimately contain < > & characters.
const TemplateAPIPrefix = "/api/v1/terminal/templates"

type TemplateHandler struct{ svc *TemplateService }

func NewTemplateHandler(svc *TemplateService) *TemplateHandler { return &TemplateHandler{svc: svc} }

func templateFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrTemplateNotFound):
		utils.Error(c, 404, err.Error())
	case errors.Is(err, ErrTemplateInput):
		utils.Error(c, 400, "name and command are required (name ≤ 100, description ≤ 255 chars)")
	default:
		utils.Error(c, 422, err.Error())
	}
}

// List godoc
// @Summary		List terminal command templates
// @Tags			terminal
// @Security		SessionCookie
// @Router			/terminal/templates [get]
func (h *TemplateHandler) List(c *gin.Context) {
	out, err := h.svc.List()
	if err != nil {
		utils.Error(c, 500, "failed to list templates")
		return
	}
	utils.Success(c, 200, "ok", out)
}

// Create godoc
// @Summary		Create a terminal command template
// @Tags			terminal
// @Security		SessionCookie
// @Router			/terminal/templates [post]
func (h *TemplateHandler) Create(c *gin.Context) {
	var in TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	t, err := h.svc.Create(in, utils.CurrentUserID(c))
	if err != nil {
		templateFail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, "terminal", t.UUID.String(), gin.H{"event": "template", "name": t.Name, "command": t.Command})
	utils.Success(c, 201, "template created", t)
}

// Update godoc
// @Summary		Update a terminal command template
// @Tags			terminal
// @Security		SessionCookie
// @Router			/terminal/templates/{uuid} [put]
func (h *TemplateHandler) Update(c *gin.Context) {
	var in TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	t, err := h.svc.Update(c.Param("uuid"), in, utils.CurrentUserID(c))
	if err != nil {
		templateFail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, "terminal", t.UUID.String(), gin.H{"event": "template", "name": t.Name, "command": t.Command})
	utils.Success(c, 200, "template updated", t)
}

// Delete godoc
// @Summary		Delete a terminal command template
// @Tags			terminal
// @Security		SessionCookie
// @Router			/terminal/templates/{uuid} [delete]
func (h *TemplateHandler) Delete(c *gin.Context) {
	t, err := h.svc.Delete(c.Param("uuid"), utils.CurrentUserID(c))
	if err != nil {
		templateFail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionDelete, "terminal", t.UUID.String(), gin.H{"event": "template", "name": t.Name})
	utils.Success(c, 200, "template deleted", nil)
}

func (h *TemplateHandler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/terminal/templates")
	g.GET("", guard("terminal", "view"), h.List)
	g.POST("", guard("terminal", "create"), h.Create)
	g.PUT("/:uuid", guard("terminal", "edit"), h.Update)
	g.DELETE("/:uuid", guard("terminal", "delete"), h.Delete)
}
