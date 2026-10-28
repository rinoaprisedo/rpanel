package docker

import (
	"errors"
	"strconv"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "docker"

type Handler struct {
	d *Docker
	c *Compose
}

func NewHandler(cfg *config.Config) *Handler {
	d := &Docker{Run: system.Default}
	return &Handler{d: d, c: &Compose{D: d, Dir: cfg.ComposeDir}}
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBadInput):
		utils.Error(c, 400, err.Error())
	case errors.Is(err, ErrNotFound):
		utils.Error(c, 404, "not found")
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

func audit(c *gin.Context, a activity_logs.ActivityAction, target string, detail gin.H) {
	activity_logs.Audit(c, a, module, target, detail)
}

// Status godoc
// @Summary		Docker installation/daemon status
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/status [get]
func (h *Handler) Status(c *gin.Context) {
	utils.Success(c, 200, "ok", h.d.Status(c.Request.Context()))
}

// Install godoc
// @Summary		Install Docker via apt (blocking)
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/install [post]
func (h *Handler) Install(c *gin.Context) {
	if err := h.d.Install(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionCreate, "", gin.H{"op": "install"})
	utils.Success(c, 200, "docker installed", nil)
}

// ListContainers godoc
// @Summary		List all containers
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/containers [get]
func (h *Handler) ListContainers(c *gin.Context) {
	out, err := h.d.ListContainers(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

// RunContainer godoc
// @Summary		Create and start a container
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/containers [post]
func (h *Handler) RunContainer(c *gin.Context) {
	var in RunInput
	if !bind(c, &in) {
		return
	}
	id, err := h.d.RunContainer(c.Request.Context(), in)
	if err != nil {
		fail(c, err)
		return
	}
	// Environment values may hold secrets: log only the keys' count.
	audit(c, activity_logs.ActionCreate, id, gin.H{"op": "run", "image": in.Image, "name": in.Name, "ports": in.Ports, "volumes": in.Volumes, "env_vars": len(in.Env)})
	utils.Success(c, 201, "container started", gin.H{"id": id})
}

// ContainerAction godoc
// @Summary		start | stop | restart | pause | unpause | kill a container
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/containers/{id}/{action} [post]
func (h *Handler) ContainerAction(c *gin.Context) {
	action := c.Param("action")
	if action == "remove" { // removal has its own, stricter permission
		utils.Error(c, 400, "invalid action")
		return
	}
	if err := h.d.Action(c.Request.Context(), c.Param("id"), action); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, c.Param("id"), gin.H{"op": action})
	utils.Success(c, 200, "done", nil)
}

// RemoveContainer godoc
// @Summary		Force-remove a container
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/containers/{id} [delete]
func (h *Handler) RemoveContainer(c *gin.Context) {
	audit(c, activity_logs.ActionDelete, c.Param("id"), gin.H{"op": "remove"})
	if err := h.d.Action(c.Request.Context(), c.Param("id"), "remove"); err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "container removed", nil)
}

// ContainerLogs godoc
// @Summary		Tail container logs
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/containers/{id}/logs [get]
func (h *Handler) ContainerLogs(c *gin.Context) {
	n, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	out, err := h.d.Logs(c.Request.Context(), c.Param("id"), n)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"log": out})
}

// ListImages godoc
// @Summary		List images
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/images [get]
func (h *Handler) ListImages(c *gin.Context) {
	out, err := h.d.ListImages(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

type pullReq struct {
	Image string `json:"image" binding:"required"`
}

// Pull godoc
// @Summary		Pull an image (blocking)
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/images/pull [post]
func (h *Handler) Pull(c *gin.Context) {
	var in pullReq
	if !bind(c, &in) {
		return
	}
	if err := h.d.Pull(c.Request.Context(), in.Image); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionCreate, in.Image, gin.H{"op": "pull"})
	utils.Success(c, 200, "image pulled", nil)
}

// RemoveImage godoc
// @Summary		Remove an image (?id=)
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/images [delete]
func (h *Handler) RemoveImage(c *gin.Context) {
	id := c.Query("id")
	audit(c, activity_logs.ActionDelete, id, gin.H{"op": "rmi"})
	if err := h.d.RemoveImage(c.Request.Context(), id); err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "image removed", nil)
}

// ListProjects godoc
// @Summary		List compose projects
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose [get]
func (h *Handler) ListProjects(c *gin.Context) {
	out, err := h.c.List()
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", out)
}

// GetProject godoc
// @Summary		Read a project's compose file
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose/{name} [get]
func (h *Handler) GetProject(c *gin.Context) {
	content, err := h.c.Get(c.Param("name"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"name": c.Param("name"), "content": content})
}

type projectReq struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// CreateProject godoc
// @Summary		Create a compose project
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose [post]
func (h *Handler) CreateProject(c *gin.Context) {
	var in projectReq
	if !bind(c, &in) {
		return
	}
	if err := h.c.Save(in.Name, in.Content, true); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionCreate, in.Name, gin.H{"op": "compose_create"})
	utils.Success(c, 201, "project created", nil)
}

// UpdateProject godoc
// @Summary		Replace a project's compose file
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose/{name} [put]
func (h *Handler) UpdateProject(c *gin.Context) {
	var in projectReq
	if !bind(c, &in) {
		return
	}
	if _, err := h.c.Get(c.Param("name")); err != nil {
		fail(c, err)
		return
	}
	if err := h.c.Save(c.Param("name"), in.Content, false); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, c.Param("name"), gin.H{"op": "compose_update"})
	utils.Success(c, 200, "saved", nil)
}

func (h *Handler) projectCmd(c *gin.Context, op string, fn func() (string, error)) {
	out, err := fn()
	audit(c, activity_logs.ActionUpdate, c.Param("name"), gin.H{"op": "compose_" + op, "ok": err == nil})
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "done", gin.H{"output": out})
}

// ProjectUp godoc
// @Summary		docker compose up -d (blocking)
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose/{name}/up [post]
func (h *Handler) ProjectUp(c *gin.Context) {
	h.projectCmd(c, "up", func() (string, error) { return h.c.Up(c.Request.Context(), c.Param("name")) })
}

// ProjectDown godoc
// @Summary		docker compose down
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose/{name}/down [post]
func (h *Handler) ProjectDown(c *gin.Context) {
	h.projectCmd(c, "down", func() (string, error) { return h.c.Down(c.Request.Context(), c.Param("name")) })
}

// ProjectPS godoc
// @Summary		docker compose ps
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose/{name}/ps [get]
func (h *Handler) ProjectPS(c *gin.Context) {
	out, err := h.c.PS(c.Request.Context(), c.Param("name"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"output": out})
}

// ProjectLogs godoc
// @Summary		docker compose logs (tail)
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose/{name}/logs [get]
func (h *Handler) ProjectLogs(c *gin.Context) {
	n, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	out, err := h.c.Logs(c.Request.Context(), c.Param("name"), n)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"output": out})
}

// DeleteProject godoc
// @Summary		Bring a project down and delete its directory
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/compose/{name} [delete]
func (h *Handler) DeleteProject(c *gin.Context) {
	audit(c, activity_logs.ActionDelete, c.Param("name"), gin.H{"op": "compose_delete"})
	if err := h.c.Delete(c.Request.Context(), c.Param("name")); err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "project deleted", nil)
}

// Overview godoc
// @Summary		Resource overview: containers, compose projects, images, networks, volumes, registries
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/overview [get]
func (h *Handler) Overview(c *gin.Context) {
	var names []string
	if ps, err := h.c.List(); err == nil {
		for _, p := range ps {
			names = append(names, p.Name)
		}
	}
	ov, err := h.d.Overview(c.Request.Context(), names, defaultDockerConfig())
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", ov)
}

// Stats godoc
// @Summary		One CPU/memory sample of every running container
// @Tags			docker
// @Security		SessionCookie
// @Router			/docker/stats [get]
func (h *Handler) Stats(c *gin.Context) {
	st, err := h.d.Stats(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", st)
}
