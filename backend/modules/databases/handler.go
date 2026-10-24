package databases

import (
	"errors"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const module = "databases"

type Handler struct{ svc *Service }

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	return &Handler{svc: NewService(db, cfg.DBSecretKey, system.Default, cfg.BackupDir)}
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		utils.Error(c, 404, err.Error())
	case errors.Is(err, ErrExists):
		utils.Error(c, 409, err.Error())
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrProtected):
		utils.Error(c, 403, err.Error())
	case errors.Is(err, ErrNoSecret):
		utils.Error(c, 503, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		utils.Error(c, 400, "invalid request")
		return false
	}
	return true
}

// ListServers godoc
// @Summary		List registered database servers
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers [get]
func (h *Handler) ListServers(c *gin.Context) {
	list, err := h.svc.ListServers()
	if err != nil {
		utils.Error(c, 500, "failed to list servers")
		return
	}
	utils.Success(c, 200, "ok", list)
}

// AddServer godoc
// @Summary		Register a database server (credentials are verified, then stored encrypted)
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers [post]
func (h *Handler) AddServer(c *gin.Context) {
	var in ServerInput
	if !bind(c, &in) {
		return
	}
	in.ActorID = utils.CurrentUserID(c)
	sv, err := h.svc.AddServer(c.Request.Context(), in)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, sv.UUID.String(), gin.H{"op": "add_server", "name": sv.Name, "engine": sv.Engine, "host": sv.Host})
	utils.Success(c, 201, "server added", sv)
}

// DeleteServer godoc
// @Summary		Forget a server connection (the server itself is not touched)
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid} [delete]
func (h *Handler) DeleteServer(c *gin.Context) {
	sv, err := h.svc.DeleteServer(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionDelete, module, sv.UUID.String(), gin.H{"op": "remove_server", "name": sv.Name})
	utils.Success(c, 200, "server removed", nil)
}

// ListDatabases godoc
// @Summary		List databases on a server
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/databases [get]
func (h *Handler) ListDatabases(c *gin.Context) {
	out, err := h.svc.ListDatabases(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

// CreateDatabase godoc
// @Summary		Create a database (optionally with a new owner user)
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/databases [post]
func (h *Handler) CreateDatabase(c *gin.Context) {
	var in CreateDBInput
	if !bind(c, &in) {
		return
	}
	if err := h.svc.CreateDatabase(c.Request.Context(), c.Param("uuid"), in); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, c.Param("uuid"), gin.H{"op": "create_database", "name": in.Name, "user": in.User})
	utils.Success(c, 201, "database created", nil)
}

// DropDatabase godoc
// @Summary		Drop a database
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/databases/{name} [delete]
func (h *Handler) DropDatabase(c *gin.Context) {
	name := c.Param("name")
	activity_logs.Audit(c, activity_logs.ActionDelete, module, c.Param("uuid"), gin.H{"op": "drop_database", "name": name})
	if err := h.svc.DropDatabase(c.Request.Context(), c.Param("uuid"), name); err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "database dropped", nil)
}

// ListUsers godoc
// @Summary		List database users
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	out, err := h.svc.ListUsers(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

// CreateUser godoc
// @Summary		Create a database user
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var in UserInput
	if !bind(c, &in) {
		return
	}
	if err := h.svc.CreateUser(c.Request.Context(), c.Param("uuid"), in); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, c.Param("uuid"), gin.H{"op": "create_user", "user": in.User, "host": in.Host})
	utils.Success(c, 201, "user created", nil)
}

// DropUser godoc
// @Summary		Drop a database user (?user=&host=)
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/users [delete]
func (h *Handler) DropUser(c *gin.Context) {
	in := UserInput{User: c.Query("user"), Host: c.Query("host")}
	activity_logs.Audit(c, activity_logs.ActionDelete, module, c.Param("uuid"), gin.H{"op": "drop_user", "user": in.User, "host": in.Host})
	if err := h.svc.DropUser(c.Request.Context(), c.Param("uuid"), in); err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "user dropped", nil)
}

// SetPassword godoc
// @Summary		Change a database user's password
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/users/password [put]
func (h *Handler) SetPassword(c *gin.Context) {
	var in UserInput
	if !bind(c, &in) {
		return
	}
	if err := h.svc.SetPassword(c.Request.Context(), c.Param("uuid"), in); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, c.Param("uuid"), gin.H{"op": "set_password", "user": in.User, "host": in.Host})
	utils.Success(c, 200, "password updated", nil)
}

// Grant godoc
// @Summary		Grant a user all privileges on a database
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/users/grant [post]
func (h *Handler) Grant(c *gin.Context) {
	var in UserInput
	if !bind(c, &in) {
		return
	}
	if err := h.svc.Grant(c.Request.Context(), c.Param("uuid"), in); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, c.Param("uuid"), gin.H{"op": "grant", "user": in.User, "database": in.Database})
	utils.Success(c, 200, "privileges granted", nil)
}

// ListBackups godoc
// @Summary		List backups of a server
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/backups [get]
func (h *Handler) ListBackups(c *gin.Context) {
	out, err := h.svc.ListBackups(c.Param("uuid"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

type backupReq struct {
	Database string `json:"database" binding:"required"`
}

// CreateBackup godoc
// @Summary		Dump a database to a backup file (blocking)
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/backups [post]
func (h *Handler) CreateBackup(c *gin.Context) {
	var in backupReq
	if !bind(c, &in) {
		return
	}
	b, err := h.svc.CreateBackup(c.Request.Context(), c.Param("uuid"), in.Database)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, c.Param("uuid"), gin.H{"op": "backup", "database": in.Database, "file": b.File})
	utils.Success(c, 201, "backup created", b)
}

type restoreReq struct {
	File   string `json:"file" binding:"required"`
	Target string `json:"target"`
}

// Restore godoc
// @Summary		Restore a backup into a database (blocking)
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/backups/restore [post]
func (h *Handler) Restore(c *gin.Context) {
	var in restoreReq
	if !bind(c, &in) {
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, c.Param("uuid"), gin.H{"op": "restore", "file": in.File, "target": in.Target})
	if err := h.svc.Restore(c.Request.Context(), c.Param("uuid"), in.File, in.Target); err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "backup restored", nil)
}

// DeleteBackup godoc
// @Summary		Delete a backup file
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/backups/{file} [delete]
func (h *Handler) DeleteBackup(c *gin.Context) {
	if err := h.svc.DeleteBackup(c.Param("uuid"), c.Param("file")); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionDelete, module, c.Param("uuid"), gin.H{"op": "delete_backup", "file": c.Param("file")})
	utils.Success(c, 200, "backup deleted", nil)
}

// DownloadBackup godoc
// @Summary		Download a backup file
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/backups/{file}/download [get]
func (h *Handler) DownloadBackup(c *gin.Context) {
	p, err := h.svc.BackupFilePath(c.Param("uuid"), c.Param("file"))
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionView, module, c.Param("uuid"), gin.H{"op": "download_backup", "file": c.Param("file")})
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(p, c.Param("file"))
}
