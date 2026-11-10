package integrations

import (
	"errors"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const module = "api-keys"

// KeysHandler manages API keys from the panel UI (session auth, menu permission "api-keys").
type KeysHandler struct{ svc *Service }

func NewKeysHandler(svc *Service) *KeysHandler { return &KeysHandler{svc: svc} }

func (h *KeysHandler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	g := rg.Group("/api-keys")
	g.GET("", guard(module, "view"), h.List)
	g.POST("", guard(module, "create"), h.Create)
	g.PUT("/:uuid", guard(module, "edit"), h.Update)
	g.POST("/:uuid/revoke", guard(module, "edit"), h.Revoke)
	g.DELETE("/:uuid", guard(module, "delete"), h.Delete)
}

func (h *KeysHandler) List(c *gin.Context) {
	ks, err := h.svc.List()
	if err != nil {
		utils.Error(c, 500, "could not load API keys")
		return
	}
	utils.Success(c, 200, "ok", gin.H{"keys": ks, "scopes": AllScopes})
}

func (h *KeysHandler) Create(c *gin.Context) {
	var in CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	uid := utils.CurrentUserID(c)
	if uid == nil {
		utils.Error(c, 401, "authentication required")
		return
	}
	k, plain, err := h.svc.Create(*uid, in)
	if errors.Is(err, ErrBadInput) {
		utils.Error(c, 400, "invalid scopes, IP list or expiry")
		return
	} else if err != nil {
		utils.Error(c, 500, "could not create the key")
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, k.UUID.String(), gin.H{"name": k.Name, "scopes": in.Scopes, "allowed_ips": k.AllowedIPs})
	// The plain key is returned this once; only its hash is stored.
	utils.Success(c, 201, "created", gin.H{"key": plain, "item": view(*k)})
}

func (h *KeysHandler) Update(c *gin.Context) {
	var in UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	k, err := h.svc.Update(c.Param("uuid"), in)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.Error(c, 404, "not found")
		return
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, "invalid name, scopes, IP list or date, or the key is revoked")
		return
	case err != nil:
		utils.Error(c, 500, "could not update the key")
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, k.UUID.String(), gin.H{"op": "edit", "name": k.Name, "scopes": in.Scopes, "allowed_ips": k.AllowedIPs, "expires_at": in.ExpiresAt})
	utils.Success(c, 200, "updated", view(*k))
}

func (h *KeysHandler) Revoke(c *gin.Context) {
	h.act(c, "revoke", h.svc.Revoke, activity_logs.ActionUpdate)
}

func (h *KeysHandler) Delete(c *gin.Context) {
	h.act(c, "delete", h.svc.Delete, activity_logs.ActionDelete)
}

func (h *KeysHandler) act(c *gin.Context, op string, fn func(string) error, a activity_logs.ActivityAction) {
	id := c.Param("uuid")
	if err := fn(id); errors.Is(err, gorm.ErrRecordNotFound) {
		utils.Error(c, 404, "not found")
		return
	} else if err != nil {
		utils.Error(c, 500, "operation failed")
		return
	}
	activity_logs.Audit(c, a, module, id, gin.H{"op": op})
	utils.Success(c, 200, "ok", nil)
}
