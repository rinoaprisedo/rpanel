package databases

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// APIPrefix is exempt from the global tag-stripping JSON sanitizer: row values
// and SQL statements legitimately contain < > & characters.
const APIPrefix = "/api/v1/databases"

const browseTimeout = 30 * time.Second

// withBrowser runs fn against one (non-system) database of a registered server.
func (s *Service) withBrowser(ctx context.Context, uuid, dbname string, fn func(Browser) error) error {
	if IsSystemDB(dbname) {
		return ErrProtected
	}
	return s.withEngine(ctx, uuid, func(e Engine, _ *Server) error {
		br, ok := e.(Browsable)
		if !ok {
			return fmt.Errorf("%w: engine cannot be browsed", ErrBadInput)
		}
		dbs, err := e.ListDatabases(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, d := range dbs {
			if d.Name == dbname {
				found = true
			}
		}
		if !found {
			return ErrNotFound
		}
		b, err := br.Browse(ctx, dbname)
		if err != nil {
			return err
		}
		defer b.Close()
		return fn(b)
	})
}

func (h *Handler) browse(c *gin.Context, fn func(ctx context.Context, b Browser) error) bool {
	ctx, cancel := context.WithTimeout(c.Request.Context(), browseTimeout)
	defer cancel()
	if err := h.svc.withBrowser(ctx, c.Param("uuid"), c.Param("db"), func(b Browser) error { return fn(ctx, b) }); err != nil {
		fail(c, err)
		return false
	}
	return true
}

func target(c *gin.Context, table string) string {
	t := c.Param("uuid") + "/" + c.Param("db")
	if table != "" {
		t += "/" + table
	}
	return t
}

// ListTables godoc
// @Summary		List tables and views of a database
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/tables [get]
func (h *Handler) ListTables(c *gin.Context) {
	var out []TableInfo
	if h.browse(c, func(ctx context.Context, b Browser) (err error) { out, err = b.Tables(ctx); return }) {
		utils.Success(c, 200, "ok", out)
	}
}

// TableData godoc
// @Summary		Table structure plus one page of rows (sort and search supported)
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/tables/{table} [get]
func (h *Handler) TableData(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	o := SelectOpts{Limit: limit, Offset: offset, Order: c.Query("order"), Desc: c.Query("dir") == "desc", Search: c.Query("q")}
	var res struct {
		Columns []Column `json:"columns"`
		*Page
	}
	if h.browse(c, func(ctx context.Context, b Browser) (err error) {
		if res.Columns, err = b.Columns(ctx, c.Param("table")); err != nil {
			return err
		}
		if len(res.Columns) == 0 {
			return ErrNotFound
		}
		res.Page, err = b.Select(ctx, c.Param("table"), o)
		return
	}) {
		utils.Success(c, 200, "ok", res)
	}
}

type rowReq struct {
	PK     map[string]string  `json:"pk"`
	Values map[string]*string `json:"values"`
}

// InsertRow godoc
// @Summary		Insert a row
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/tables/{table}/rows [post]
func (h *Handler) InsertRow(c *gin.Context) {
	var in rowReq
	if !bind(c, &in) {
		return
	}
	if h.browse(c, func(ctx context.Context, b Browser) error { return b.Insert(ctx, c.Param("table"), in.Values) }) {
		activity_logs.Audit(c, activity_logs.ActionCreate, module, target(c, c.Param("table")), gin.H{"op": "insert_row"})
		utils.Success(c, 201, "row inserted", nil)
	}
}

// UpdateRow godoc
// @Summary		Update a row identified by its primary key
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/tables/{table}/rows [put]
func (h *Handler) UpdateRow(c *gin.Context) {
	var in rowReq
	if !bind(c, &in) {
		return
	}
	var n int64
	if h.browse(c, func(ctx context.Context, b Browser) (err error) {
		n, err = b.Update(ctx, c.Param("table"), in.PK, in.Values)
		return
	}) {
		activity_logs.Audit(c, activity_logs.ActionUpdate, module, target(c, c.Param("table")), gin.H{"op": "update_row", "affected": n})
		utils.Success(c, 200, "row updated", gin.H{"affected": n})
	}
}

// DeleteRow godoc
// @Summary		Delete a row identified by its primary key
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/tables/{table}/rows [delete]
func (h *Handler) DeleteRow(c *gin.Context) {
	var in rowReq
	if !bind(c, &in) {
		return
	}
	var n int64
	if h.browse(c, func(ctx context.Context, b Browser) (err error) {
		n, err = b.Delete(ctx, c.Param("table"), in.PK)
		return
	}) {
		activity_logs.Audit(c, activity_logs.ActionDelete, module, target(c, c.Param("table")), gin.H{"op": "delete_row", "affected": n})
		utils.Success(c, 200, "row deleted", gin.H{"affected": n})
	}
}

// TruncateTable godoc
// @Summary		Remove all rows of a table
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/tables/{table}/truncate [post]
func (h *Handler) TruncateTable(c *gin.Context) {
	if h.browse(c, func(ctx context.Context, b Browser) error { return b.Truncate(ctx, c.Param("table")) }) {
		activity_logs.Audit(c, activity_logs.ActionDelete, module, target(c, c.Param("table")), gin.H{"op": "truncate_table"})
		utils.Success(c, 200, "table truncated", nil)
	}
}

// DropTable godoc
// @Summary		Drop a table or view
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/tables/{table} [delete]
func (h *Handler) DropTable(c *gin.Context) {
	if h.browse(c, func(ctx context.Context, b Browser) error { return b.Drop(ctx, c.Param("table")) }) {
		activity_logs.Audit(c, activity_logs.ActionDelete, module, target(c, c.Param("table")), gin.H{"op": "drop_table"})
		utils.Success(c, 200, "table dropped", nil)
	}
}

type queryReq struct {
	SQL string `json:"sql" binding:"required"`
}

// RunQuery godoc
// @Summary		Run one SQL statement as the server admin in the selected database
// @Tags			databases
// @Security		SessionCookie
// @Router			/databases/servers/{uuid}/browse/{db}/query [post]
func (h *Handler) RunQuery(c *gin.Context) {
	var in queryReq
	if !bind(c, &in) {
		return
	}
	var res *QueryResult
	if h.browse(c, func(ctx context.Context, b Browser) (err error) { res, err = b.Exec(ctx, in.SQL); return }) {
		// Statement text can carry secrets (passwords in INSERTs), so only the verb is recorded.
		verb := strings.ToUpper(strings.Fields(in.SQL + " ")[0])
		activity_logs.Audit(c, activity_logs.ActionUpdate, module, target(c, ""), gin.H{"op": "sql", "verb": verb, "affected": res.Affected, "rows": len(res.Rows)})
		utils.Success(c, 200, "ok", res)
	}
}
