package filemanager

import "github.com/gin-gonic/gin"

// APIPrefix is the URL prefix of this module; main.go uses it to relax the
// global JSON sanitizer and body-size limit, which would corrupt file content.
const APIPrefix = "/api/v1/file-manager"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	const m = "file-manager"
	g := rg.Group("/file-manager")
	g.GET("/roots", guard(m, "view"), h.Roots)
	g.GET("/list", guard(m, "view"), h.List)
	g.GET("/search", guard(m, "view"), h.Search)
	g.GET("/read", guard(m, "view"), h.Read)
	g.GET("/download", guard(m, "view"), h.Download)

	g.PUT("/write", guard(m, "edit"), h.Write)
	g.POST("/rename", guard(m, "edit"), h.Rename)
	g.POST("/move", guard(m, "edit"), h.Move)
	g.POST("/chmod", guard(m, "edit"), h.Chmod)
	g.POST("/chown", guard(m, "edit"), h.Chown)

	g.POST("/mkdir", guard(m, "create"), h.Mkdir)
	g.POST("/touch", guard(m, "create"), h.NewFile)
	g.POST("/copy", guard(m, "create"), h.Copy)
	g.POST("/upload", guard(m, "create"), h.Upload)
	g.POST("/compress", guard(m, "create"), h.Compress)
	g.POST("/extract", guard(m, "create"), h.Extract)

	g.POST("/delete", guard(m, "delete"), h.Delete)
}
