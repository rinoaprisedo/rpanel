package website

import (
	"errors"
	"strconv"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const module = "websites"

type Handler struct {
	svc *Service
}

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	ng := &Nginx{
		Run: system.Default, Available: cfg.NginxAvailableDir, Enabled: cfg.NginxEnabledDir,
		LogDir: cfg.NginxLogDir, AcmeRoot: cfg.AcmeRoot,
	}
	return &Handler{svc: NewService(db, ng, system.Default, Settings{
		WebRoot: cfg.WebRoot, AcmeRoot: cfg.AcmeRoot, LogDir: cfg.NginxLogDir, CertbotEmail: cfg.CertbotEmail, PanelTLSConf: cfg.PanelTLSConf,
	})}
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		utils.Error(c, 404, err.Error())
	case errors.Is(err, ErrExists):
		utils.Error(c, 409, err.Error())
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	default:
		// Operational failures (nginx -t output, certbot output) are useful to admins.
		utils.Error(c, 422, err.Error())
	}
}

// List godoc
// @Summary		List websites
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites [get]
func (h *Handler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		utils.Error(c, 500, "failed to list websites")
		return
	}
	utils.Success(c, 200, "ok", list)
}

// Get godoc
// @Summary		Get a website
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid} [get]
func (h *Handler) Get(c *gin.Context) {
	w, err := h.svc.Get(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", w)
}

// Create godoc
// @Summary		Create a website (static, php or reverse proxy)
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites [post]
func (h *Handler) Create(c *gin.Context) {
	var in CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	in.ActorID = utils.CurrentUserID(c)
	w, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, w.UUID.String(), gin.H{"domain": w.Domain, "type": w.Type})
	utils.Success(c, 201, "website created", w)
}

// Update godoc
// @Summary		Update site settings and re-apply nginx config
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid} [put]
func (h *Handler) Update(c *gin.Context) {
	var in UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	w, err := h.svc.Update(c.Request.Context(), c.Param("uuid"), in, utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, w.UUID.String(), gin.H{"domain": w.Domain, "change": in})
	utils.Success(c, 200, "website updated", w)
}

func (h *Handler) setStatus(c *gin.Context, status string) {
	w, err := h.svc.SetStatus(c.Request.Context(), c.Param("uuid"), status, utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, w.UUID.String(), gin.H{"domain": w.Domain, "status": status})
	utils.Success(c, 200, "website "+status, w)
}

// Start godoc
// @Summary		Enable the site vhost
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/start [post]
func (h *Handler) Start(c *gin.Context) { h.setStatus(c, StatusRunning) }

// Stop godoc
// @Summary		Disable the site vhost
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/stop [post]
func (h *Handler) Stop(c *gin.Context) { h.setStatus(c, StatusStopped) }

// Delete godoc
// @Summary		Delete a site (add ?delete_files=1 to remove its directory)
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid} [delete]
func (h *Handler) Delete(c *gin.Context) {
	files := c.Query("delete_files") == "1"
	w, err := h.svc.Delete(c.Request.Context(), c.Param("uuid"), files)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionDelete, module, w.UUID.String(), gin.H{"domain": w.Domain, "delete_files": files})
	utils.Success(c, 200, "website deleted", nil)
}

type sslReq struct {
	Email string `json:"email"`
}

// IssueSSL godoc
// @Summary		Issue a Let's Encrypt certificate and enable HTTPS
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/ssl [post]
func (h *Handler) IssueSSL(c *gin.Context) {
	var req sslReq
	_ = c.ShouldBindJSON(&req)
	w, err := h.svc.IssueSSL(c.Request.Context(), c.Param("uuid"), req.Email, utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, w.UUID.String(), gin.H{"domain": w.Domain, "ssl": "issued"})
	utils.Success(c, 200, "SSL enabled", w)
}

type sslStartReq struct {
	Email      string `json:"email"`
	ForceHTTPS *bool  `json:"force_https"`
}

// StartSSL godoc
// @Summary		Start issuing a Let's Encrypt certificate in the background (poll /ssl/progress)
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/ssl/start [post]
func (h *Handler) StartSSL(c *gin.Context) {
	var req sslStartReq
	_ = c.ShouldBindJSON(&req)
	force := true
	if req.ForceHTTPS != nil {
		force = *req.ForceHTTPS
	}
	p, err := h.svc.StartSSL(c.Param("uuid"), req.Email, force, utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, c.Param("uuid"), gin.H{"domain": p.Domain, "ssl": "requested"})
	utils.Success(c, 202, "certificate request started", p)
}

// SSLProgress godoc
// @Summary		Progress of the latest certificate request of a site
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/ssl/progress [get]
func (h *Handler) SSLProgress(c *gin.Context) {
	p := h.svc.SSLProgress(c.Param("uuid"))
	if p == nil {
		utils.Error(c, 404, "no certificate request for this site")
		return
	}
	utils.Success(c, 200, "ok", p)
}

// Logs godoc
// @Summary		Tail the access or error log of a site
// @Tags			websites
// @Security		SessionCookie
// @Param			type	query	string	false	"access|error"
// @Param			lines	query	int		false	"default 200, max 1000"
// @Router			/websites/{uuid}/logs [get]
func (h *Handler) Logs(c *gin.Context) {
	n, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	out, err := h.svc.Logs(c.Param("uuid"), c.DefaultQuery("type", "access"), n)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"log": out})
}

// Env godoc
// @Summary		Installed components (nginx, certbot, php versions)
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/env [get]
func (h *Handler) Env(c *gin.Context) {
	utils.Success(c, 200, "ok", h.svc.Env(c.Request.Context()))
}

type installReq struct {
	Component string `json:"component" binding:"required"`
	Version   string `json:"version"`
}

// Install godoc
// @Summary		Install nginx, certbot or php-fpm via apt (blocking)
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/install [post]
func (h *Handler) Install(c *gin.Context) {
	var req installReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	if err := h.svc.Install(c.Request.Context(), req.Component, req.Version); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, "", gin.H{"install": req.Component, "version": req.Version})
	utils.Success(c, 200, "installed", nil)
}

// Dirs godoc
// @Summary		List sub-folders inside the web root (folder picker)
// @Tags			websites
// @Security		SessionCookie
// @Param			path	query	string	false	"folder inside the web root; default is the web root"
// @Router			/websites/dirs [get]
func (h *Handler) Dirs(c *gin.Context) {
	out, err := h.svc.ListDirs(c.Query("path"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

type proxiesReq struct {
	Rules []ProxyRule `json:"rules"`
}

// SetProxies godoc
// @Summary		Replace the reverse-proxy rules of a site
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/proxies [put]
func (h *Handler) SetProxies(c *gin.Context) {
	var req proxiesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	w, err := h.svc.SetProxyRules(c.Request.Context(), c.Param("uuid"), req.Rules, utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, w.UUID.String(), gin.H{"domain": w.Domain, "proxy_rules": len(w.ProxyRules)})
	utils.Success(c, 200, "saved", w)
}

// Config godoc
// @Summary		Custom nginx directives and the full generated vhost of a site
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/config [get]
func (h *Handler) Config(c *gin.Context) {
	out, err := h.svc.Config(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

type configReq struct {
	ExtraConfig string `json:"extra_config"`
}

// SaveConfig godoc
// @Summary		Save custom nginx directives (superadmin only; validated with nginx -t and rolled back on failure)
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/config [put]
func (h *Handler) SaveConfig(c *gin.Context) {
	// Raw directives can expose any path (alias/root), which is as powerful as the file manager.
	if !utils.IsSuperadmin(c) {
		utils.Error(c, 403, "only a superadmin can edit raw nginx directives")
		return
	}
	var req configReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	w, err := h.svc.SetExtraConfig(c.Request.Context(), c.Param("uuid"), req.ExtraConfig, utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, w.UUID.String(), gin.H{"domain": w.Domain, "custom_config_bytes": len(w.ExtraConfig)})
	utils.Success(c, 200, "saved", nil)
}

// SSLInfo godoc
// @Summary		SSL state and certificate details of a site
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/ssl [get]
func (h *Handler) SSLInfo(c *gin.Context) {
	out, err := h.svc.SSLStatus(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

// DisableSSL godoc
// @Summary		Serve a site over plain HTTP again (the certificate is kept)
// @Tags			websites
// @Security		SessionCookie
// @Router			/websites/{uuid}/ssl [delete]
func (h *Handler) DisableSSL(c *gin.Context) {
	w, err := h.svc.DisableSSL(c.Request.Context(), c.Param("uuid"), utils.CurrentUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, w.UUID.String(), gin.H{"domain": w.Domain, "ssl": "disabled"})
	utils.Success(c, 200, "SSL disabled", w)
}
