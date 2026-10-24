package databases

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	const m = "databases"
	g := rg.Group("/databases/servers")

	g.GET("", guard(m, "view"), h.ListServers)
	g.POST("", guard(m, "create"), h.AddServer)
	g.POST("/install", guard(m, "create"), h.InstallServer)
	g.DELETE("/:uuid", guard(m, "delete"), h.DeleteServer)

	g.GET("/:uuid/databases", guard(m, "view"), h.ListDatabases)
	g.POST("/:uuid/databases", guard(m, "create"), h.CreateDatabase)
	g.DELETE("/:uuid/databases/:name", guard(m, "delete"), h.DropDatabase)

	g.GET("/:uuid/users", guard(m, "view"), h.ListUsers)
	g.POST("/:uuid/users", guard(m, "create"), h.CreateUser)
	g.DELETE("/:uuid/users", guard(m, "delete"), h.DropUser)
	g.PUT("/:uuid/users/password", guard(m, "edit"), h.SetPassword)
	g.POST("/:uuid/users/grant", guard(m, "edit"), h.Grant)

	// Dumps contain all data: listing is "view", but producing, downloading and
	// restoring them require "edit".
	g.GET("/:uuid/backups", guard(m, "view"), h.ListBackups)
	g.POST("/:uuid/backups", guard(m, "edit"), h.CreateBackup)
	g.POST("/:uuid/backups/restore", guard(m, "edit"), h.Restore)
	g.GET("/:uuid/backups/:file/download", guard(m, "edit"), h.DownloadBackup)
	g.DELETE("/:uuid/backups/:file", guard(m, "delete"), h.DeleteBackup)
}
