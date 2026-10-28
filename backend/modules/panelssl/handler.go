package panelssl

import (
	"errors"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "settings"

type Handler struct {
	svc *Service
}

// NewService builds the service from the panel configuration (also used by the CLI).
func NewServiceFromConfig(cfg *config.Config) *Service {
	return NewService(system.Default, Settings{
		TLSConf: cfg.PanelTLSConf, GuardConf: cfg.PanelGuardConf, SSLDir: cfg.PanelSSLDir,
		Available: cfg.NginxAvailableDir, Enabled: cfg.NginxEnabledDir, AcmeRoot: cfg.AcmeRoot, Port: cfg.PanelPort,
	})
}

func NewHandler(cfg *config.Config) *Handler { return &Handler{svc: NewServiceFromConfig(cfg)} }

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrNotSet):
		utils.Error(c, 409, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

// Status godoc
// @Summary		Panel domain and certificate status
// @Tags			panel-ssl
// @Security		SessionCookie
// @Router			/panel-ssl [get]
func (h *Handler) Status(c *gin.Context) { utils.Success(c, 200, "ok", h.svc.Status()) }

// Issue godoc
// @Summary		Obtain a Let's Encrypt certificate for the panel domain
// @Tags			panel-ssl
// @Security		SessionCookie
// @Router			/panel-ssl/letsencrypt [post]
func (h *Handler) Issue(c *gin.Context) {
	var in struct {
		Domain string `json:"domain"`
		Email  string `json:"email"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "domain and email are required")
		return
	}
	if err := h.svc.IssueLetsEncrypt(c.Request.Context(), in.Domain, in.Email); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, "panel-ssl", gin.H{"domain": in.Domain, "mode": ModeLetsEncrypt})
	utils.Success(c, 200, "panel certificate installed", h.svc.Status())
}

// Custom godoc
// @Summary		Install a custom certificate for the panel domain
// @Tags			panel-ssl
// @Security		SessionCookie
// @Router			/panel-ssl/custom [post]
func (h *Handler) Custom(c *gin.Context) {
	var in struct {
		Domain string `json:"domain"`
		Cert   string `json:"cert"`
		Key    string `json:"key"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Cert == "" || in.Key == "" {
		utils.Error(c, 400, "domain, cert and key are required")
		return
	}
	if err := h.svc.SaveCustom(c.Request.Context(), in.Domain, in.Cert, in.Key); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, "panel-ssl", gin.H{"domain": in.Domain, "mode": ModeCustom})
	utils.Success(c, 200, "panel certificate installed", h.svc.Status())
}
