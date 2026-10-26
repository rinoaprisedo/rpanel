package filemanager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

const module = "file-manager"

// Handler is the HTTP layer for the file manager.
type Handler struct {
	roots  []string
	denied []string
}

// NewHandler: roots bound non-superadmin users; denied are directories inside
// those roots they may never touch (cron scripts run as root, compose files
// start containers, database dumps hold every row).
func NewHandler(roots []string, denied ...string) *Handler {
	return &Handler{roots: roots, denied: denied}
}

func (h *Handler) access(c *gin.Context) Access {
	return Access{Roots: h.roots, Denied: h.denied, Unrestricted: utils.IsSuperadmin(c)}
}

// fail maps service/system errors to HTTP responses without leaking OS paths.
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, system.ErrPathDenied):
		utils.Error(c, 403, "path is outside the allowed directories")
	case errors.Is(err, ErrProtected):
		utils.Error(c, 403, err.Error())
	case errors.Is(err, os.ErrNotExist):
		utils.Error(c, 404, "file or directory not found")
	case errors.Is(err, os.ErrPermission):
		utils.Error(c, 403, "permission denied by the operating system")
	case errors.Is(err, ErrExists):
		utils.Error(c, 409, err.Error())
	case errors.Is(err, ErrTooLarge):
		utils.Error(c, 413, err.Error())
	case errors.Is(err, ErrNotDir), errors.Is(err, ErrIsDir), errors.Is(err, ErrBinary),
		errors.Is(err, ErrBadName), errors.Is(err, ErrBadMode), errors.Is(err, ErrBadOwner):
		utils.Error(c, 400, err.Error())
	default:
		utils.Error(c, 400, "operation failed: "+err.Error())
	}
}

// resolve1 binds+validates a single path string; returns false after replying.
func (h *Handler) resolve1(c *gin.Context, raw string) (string, bool) {
	p, err := h.access(c).Resolve(raw)
	if err != nil {
		fail(c, err)
		return "", false
	}
	return p, true
}

func (h *Handler) resolveMany(c *gin.Context, raws []string) ([]string, bool) {
	if len(raws) == 0 {
		utils.Error(c, 400, "no paths given")
		return nil, false
	}
	out := make([]string, 0, len(raws))
	for _, r := range raws {
		p, ok := h.resolve1(c, r)
		if !ok {
			return nil, false
		}
		out = append(out, p)
	}
	return out, true
}

// Roots godoc
// @Summary		Starting directories for the current user
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/roots [get]
func (h *Handler) Roots(c *gin.Context) {
	roots := h.roots
	if utils.IsSuperadmin(c) {
		roots = []string{"/"}
	}
	utils.Success(c, 200, "ok", roots)
}

// List godoc
// @Summary		List a directory
// @Tags			file-manager
// @Security		SessionCookie
// @Param			path	query	string	true	"absolute directory path"
// @Router			/file-manager/list [get]
func (h *Handler) List(c *gin.Context) {
	p, ok := h.resolve1(c, c.Query("path"))
	if !ok {
		return
	}
	if err := mustBeDir(p); err != nil {
		fail(c, err)
		return
	}
	entries, err := List(p)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"path": p, "entries": entries})
}

// Read godoc
// @Summary		Read a text file for editing (max 2 MB)
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/read [get]
func (h *Handler) Read(c *gin.Context) {
	p, ok := h.resolve1(c, c.Query("path"))
	if !ok {
		return
	}
	content, err := ReadText(p)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"path": p, "content": content})
}

type writeReq struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content"`
}

// Write godoc
// @Summary		Save a text file
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/write [put]
func (h *Handler) Write(c *gin.Context) {
	var req writeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, ok := h.resolve1(c, req.Path)
	if !ok {
		return
	}
	if err := WriteText(p, req.Content); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, p, gin.H{"op": "write", "bytes": len(req.Content)})
	utils.Success(c, 200, "saved", nil)
}

type pathReq struct {
	Path string `json:"path" binding:"required"`
}

// Mkdir godoc
// @Summary		Create a directory
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/mkdir [post]
func (h *Handler) Mkdir(c *gin.Context) {
	var req pathReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, ok := h.resolve1(c, req.Path)
	if !ok {
		return
	}
	if err := Mkdir(p); err != nil {
		if errors.Is(err, os.ErrExist) {
			err = ErrExists
		}
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, p, gin.H{"op": "mkdir"})
	utils.Success(c, 201, "created", nil)
}

// NewFile creates an empty file.
// @Summary		Create an empty file
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/touch [post]
func (h *Handler) NewFile(c *gin.Context) {
	var req pathReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, ok := h.resolve1(c, req.Path)
	if !ok {
		return
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			err = ErrExists
		}
		fail(c, err)
		return
	}
	f.Close()
	activity_logs.Audit(c, activity_logs.ActionCreate, module, p, gin.H{"op": "touch"})
	utils.Success(c, 201, "created", nil)
}

type renameReq struct {
	Path    string `json:"path" binding:"required"`
	NewName string `json:"new_name" binding:"required"`
}

// Rename godoc
// @Summary		Rename a file or directory
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/rename [post]
func (h *Handler) Rename(c *gin.Context) {
	var req renameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, ok := h.resolve1(c, req.Path)
	if !ok {
		return
	}
	if err := Rename(h.access(c), p, req.NewName); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, p, gin.H{"op": "rename", "to": req.NewName})
	utils.Success(c, 200, "renamed", nil)
}

type transferReq struct {
	Paths []string `json:"paths" binding:"required"`
	Dest  string   `json:"dest" binding:"required"`
}

// Move godoc
// @Summary		Move paths into a directory
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/move [post]
func (h *Handler) Move(c *gin.Context) { h.transfer(c, "move") }

// Copy godoc
// @Summary		Copy paths into a directory
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/copy [post]
func (h *Handler) Copy(c *gin.Context) { h.transfer(c, "copy") }

func (h *Handler) transfer(c *gin.Context, op string) {
	var req transferReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	srcs, ok := h.resolveMany(c, req.Paths)
	if !ok {
		return
	}
	dst, ok := h.resolve1(c, req.Dest)
	if !ok {
		return
	}
	var err error
	if op == "move" {
		err = Move(h.access(c), srcs, dst)
	} else {
		err = Copy(srcs, dst)
	}
	if err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, dst, gin.H{"op": op, "paths": srcs})
	utils.Success(c, 200, "done", nil)
}

type pathsReq struct {
	Paths []string `json:"paths" binding:"required"`
}

// Delete godoc
// @Summary		Delete paths (recursive)
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/delete [post]
func (h *Handler) Delete(c *gin.Context) {
	var req pathsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	ps, ok := h.resolveMany(c, req.Paths)
	if !ok {
		return
	}
	// Audit first: a failed/partial delete is still worth recording.
	activity_logs.Audit(c, activity_logs.ActionDelete, module, "", gin.H{"op": "delete", "paths": ps})
	if err := Delete(h.access(c), ps); err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "deleted", nil)
}

type chmodReq struct {
	Path      string `json:"path" binding:"required"`
	Mode      string `json:"mode" binding:"required"`
	Recursive bool   `json:"recursive"`
}

// Chmod godoc
// @Summary		Change permissions (octal mode)
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/chmod [post]
func (h *Handler) Chmod(c *gin.Context) {
	var req chmodReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, ok := h.resolve1(c, req.Path)
	if !ok {
		return
	}
	if err := Chmod(p, req.Mode, req.Recursive); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, p, gin.H{"op": "chmod", "mode": req.Mode, "recursive": req.Recursive})
	utils.Success(c, 200, "updated", nil)
}

type chownReq struct {
	Path      string `json:"path" binding:"required"`
	Owner     string `json:"owner" binding:"required"`
	Group     string `json:"group"`
	Recursive bool   `json:"recursive"`
}

// Chown godoc
// @Summary		Change owner/group (superadmin only)
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/chown [post]
func (h *Handler) Chown(c *gin.Context) {
	if !utils.IsSuperadmin(c) {
		utils.Error(c, 403, "superadmin access required")
		return
	}
	var req chownReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, ok := h.resolve1(c, req.Path)
	if !ok {
		return
	}
	if err := Chown(c.Request.Context(), p, req.Owner, req.Group, req.Recursive); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionUpdate, module, p, gin.H{"op": "chown", "owner": req.Owner, "group": req.Group, "recursive": req.Recursive})
	utils.Success(c, 200, "updated", nil)
}

type zipReq struct {
	Paths []string `json:"paths" binding:"required"`
	Dest  string   `json:"dest" binding:"required"` // full path of the .zip to create
}

// Compress godoc
// @Summary		Create a zip archive
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/compress [post]
func (h *Handler) Compress(c *gin.Context) {
	var req zipReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	srcs, ok := h.resolveMany(c, req.Paths)
	if !ok {
		return
	}
	dst, ok := h.resolve1(c, req.Dest)
	if !ok {
		return
	}
	if err := Zip(srcs, dst); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, dst, gin.H{"op": "compress", "paths": srcs})
	utils.Success(c, 201, "archive created", nil)
}

type unzipReq struct {
	Path string `json:"path" binding:"required"`
	Dest string `json:"dest" binding:"required"`
}

// Extract godoc
// @Summary		Extract a zip archive into a directory
// @Tags			file-manager
// @Security		SessionCookie
// @Router			/file-manager/extract [post]
func (h *Handler) Extract(c *gin.Context) {
	var req unzipReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	src, ok := h.resolve1(c, req.Path)
	if !ok {
		return
	}
	dst, ok := h.resolve1(c, req.Dest)
	if !ok {
		return
	}
	if err := mustBeDir(dst); err != nil {
		fail(c, err)
		return
	}
	if err := Unzip(src, dst); err != nil {
		fail(c, err)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, dst, gin.H{"op": "extract", "archive": src})
	utils.Success(c, 200, "extracted", nil)
}

// Download godoc
// @Summary		Download a file
// @Tags			file-manager
// @Security		SessionCookie
// @Param			path	query	string	true	"absolute file path"
// @Router			/file-manager/download [get]
func (h *Handler) Download(c *gin.Context) {
	p, ok := h.resolve1(c, c.Query("path"))
	if !ok {
		return
	}
	info, err := os.Stat(p)
	if err != nil {
		fail(c, err)
		return
	}
	if info.IsDir() {
		fail(c, ErrIsDir)
		return
	}
	activity_logs.Audit(c, activity_logs.ActionView, module, p, gin.H{"op": "download"})
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(p, filepath.Base(p))
}

// Upload godoc
// @Summary		Upload one or more files into a directory (query: dir; multipart body: files)
// @Tags			file-manager
// @Security		SessionCookie
// @Accept			mpfd
// @Router			/file-manager/upload [post]
func (h *Handler) Upload(c *gin.Context) {
	dir, ok := h.resolve1(c, c.Query("dir"))
	if !ok {
		return
	}
	if err := mustBeDir(dir); err != nil {
		fail(c, err)
		return
	}
	reader, err := c.Request.MultipartReader()
	if err != nil {
		utils.Error(c, 400, "multipart form expected")
		return
	}
	overwrite := c.Query("overwrite") == "1"
	saved := []string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			utils.Error(c, 400, "upload interrupted")
			return
		}
		name := part.FileName()
		if name == "" { // plain form field
			continue
		}
		name = filepath.Base(name)
		if !ValidName(name) {
			utils.Error(c, 400, "invalid file name")
			return
		}
		dst, err := h.access(c).Resolve(filepath.Join(dir, name))
		if err != nil {
			fail(c, err)
			return
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if overwrite {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		out, err := os.OpenFile(dst, flags, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				err = ErrExists
			}
			fail(c, err)
			return
		}
		_, cerr := io.Copy(out, part)
		if e := out.Close(); cerr == nil {
			cerr = e
		}
		if cerr != nil {
			os.Remove(dst)
			utils.Error(c, 500, "failed to write file")
			return
		}
		saved = append(saved, name)
	}
	activity_logs.Audit(c, activity_logs.ActionCreate, module, dir, gin.H{"op": "upload", "files": saved})
	utils.Success(c, 201, "uploaded", gin.H{"files": saved})
}

// Search godoc
// @Summary		Find files and folders by name below a directory
// @Tags			file-manager
// @Security		SessionCookie
// @Param			path	query	string	true	"absolute directory path to search in"
// @Param			q		query	string	true	"part of the name (case-insensitive)"
// @Router			/file-manager/search [get]
func (h *Handler) Search(c *gin.Context) {
	p, ok := h.resolve1(c, c.Query("path"))
	if !ok {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" || len(q) > 100 {
		utils.Error(c, 400, "enter 1-100 characters to search for")
		return
	}
	if err := mustBeDir(p); err != nil {
		fail(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	entries, truncated, err := Search(ctx, p, q, maxSearchResults)
	if err != nil {
		fail(c, err)
		return
	}
	if entries == nil {
		entries = []Entry{}
	}
	utils.Success(c, 200, "ok", gin.H{"path": p, "query": q, "entries": entries, "truncated": truncated})
}
