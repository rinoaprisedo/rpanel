package weblogs

import (
	"errors"
	"strconv"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const module = "logs"

type Handler struct{ svc *Service }

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	return &Handler{svc: NewService(db, cfg.NginxLogDir)}
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrNotFound):
		utils.Error(c, 404, err.Error())
	default:
		utils.Error(c, 500, "cannot read the log")
	}
}

// Sites godoc
// @Summary		Websites with the size of their logs
// @Tags			logs
// @Security		SessionCookie
// @Router			/logs/sites [get]
func (h *Handler) Sites(c *gin.Context) {
	l, err := h.svc.Sites()
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", l)
}

// Read godoc
// @Summary		Filtered log lines of one website (or "all")
// @Tags			logs
// @Security		SessionCookie
// @Param			type	query	string	false	"access|error"
// @Param			lines	query	int		false	"0 = all (max 5000)"
// @Param			q		query	string	false	"keyword"
// @Param			from	query	string	false	"YYYY-MM-DD"
// @Param			to		query	string	false	"YYYY-MM-DD"
// @Router			/logs/sites/{domain} [get]
func (h *Handler) Read(c *gin.Context) {
	from, err := ParseDay(c.Query("from"))
	if err != nil {
		fail(c, err)
		return
	}
	to, err := ParseDay(c.Query("to"))
	if err != nil {
		fail(c, err)
		return
	}
	n, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))
	r, err := h.svc.Read(Query{Domain: c.Param("domain"), Kind: c.DefaultQuery("type", "access"), Lines: n, Keyword: c.Query("q"), From: from, To: to})
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", r)
}

// Download godoc
// @Summary		Download a log file
// @Tags			logs
// @Security		SessionCookie
// @Router			/logs/sites/{domain}/download [get]
func (h *Handler) Download(c *gin.Context) {
	kind := c.DefaultQuery("type", "access")
	p, err := h.svc.File(c.Param("domain"), kind)
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionView, module, c.Param("domain"), gin.H{"export": kind})
	c.FileAttachment(p, c.Param("domain")+"."+kind+".log")
}

// Clear godoc
// @Summary		Empty a log file
// @Tags			logs
// @Security		SessionCookie
// @Router			/logs/sites/{domain} [delete]
func (h *Handler) Clear(c *gin.Context) {
	kind := c.DefaultQuery("type", "access")
	if err := h.svc.Clear(c.Param("domain"), kind); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionDelete, module, c.Param("domain"), gin.H{"clear": kind})
	utils.Success(c, 200, "log cleared", nil)
}
