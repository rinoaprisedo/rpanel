package docker

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	const m = "docker"
	g := rg.Group("/docker")

	g.GET("/status", guard(m, "view"), h.Status)
	g.POST("/install", guard(m, "create"), h.Install)

	g.GET("/containers", guard(m, "view"), h.ListContainers)
	g.POST("/containers", guard(m, "create"), h.RunContainer)
	g.GET("/containers/:id/logs", guard(m, "view"), h.ContainerLogs)
	g.POST("/containers/:id/:action", guard(m, "edit"), h.ContainerAction)
	g.DELETE("/containers/:id", guard(m, "delete"), h.RemoveContainer)

	g.GET("/images", guard(m, "view"), h.ListImages)
	g.POST("/images/pull", guard(m, "create"), h.Pull)
	g.DELETE("/images", guard(m, "delete"), h.RemoveImage)

	g.GET("/compose", guard(m, "view"), h.ListProjects)
	g.POST("/compose", guard(m, "create"), h.CreateProject)
	g.GET("/compose/:name", guard(m, "view"), h.GetProject)
	g.PUT("/compose/:name", guard(m, "edit"), h.UpdateProject)
	g.GET("/compose/:name/ps", guard(m, "view"), h.ProjectPS)
	g.GET("/compose/:name/logs", guard(m, "view"), h.ProjectLogs)
	g.POST("/compose/:name/up", guard(m, "edit"), h.ProjectUp)
	g.POST("/compose/:name/down", guard(m, "edit"), h.ProjectDown)
	g.DELETE("/compose/:name", guard(m, "delete"), h.DeleteProject)
}

// APIPrefix is exempted from the global JSON tag-stripping sanitizer: compose
// files and environment values legitimately contain characters like < and >.
const APIPrefix = "/api/v1/docker"
