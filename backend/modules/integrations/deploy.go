package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"rpanel/backend/config"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/modules/docker"
	"rpanel/backend/modules/filemanager"
	"rpanel/backend/system"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// APIPrefix: bearer-token API, so it is exempt from the CSRF check (it never reads cookies) and from the JSON tag stripper
// (compose files legitimately contain < and >).
const APIPrefix = "/api/v1/integrations"

const (
	MaxChunk = 8 << 20  // stays under the panel's 10 MB request limit and typical nginx client_max_body_size
	maxTotal = 20 << 30 // largest single file accepted
)

// Deploy is the machine API Vibecoder uses: upload image archives, docker load, compose up.
// Uploads are confined to the deploy folders for every key, whoever owns it.
type Deploy struct {
	svc    *Service
	d      *docker.Docker
	c      *docker.Compose
	access filemanager.Access
	roots  []string
	sshOK  []string
}

func NewDeploy(svc *Service, cfg *config.Config) *Deploy {
	d := &docker.Docker{Run: system.Default}
	roots := []string{cfg.DeployDir, cfg.ComposeDir}
	return &Deploy{
		svc: svc, d: d, c: &docker.Compose{D: d, Dir: cfg.ComposeDir}, roots: roots,
		access: filemanager.Access{Roots: roots},
		sshOK:  cfg.DeploySSHUsers,
	}
}

func (h *Deploy) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/integrations")
	up, dk, sh := h.svc.KeyAuth(ScopeUpload), h.svc.KeyAuth(ScopeDocker), h.svc.KeyAuth(ScopeSSH)

	g.GET("/ping", h.svc.KeyAuth(ScopeUpload), h.Ping)
	g.GET("/deploy/roots", up, h.Roots)
	g.GET("/deploy/list", up, h.List)
	g.POST("/deploy/mkdir", up, h.Mkdir)
	g.GET("/deploy/upload/status", up, h.UploadStatus)
	g.POST("/deploy/upload/chunk", up, h.Chunk)

	g.POST("/deploy/images/load", dk, h.LoadImage)
	g.GET("/deploy/images", dk, h.Images)
	g.GET("/deploy/compose", dk, h.ComposeList)
	g.GET("/deploy/compose/:name", dk, h.ComposeGet)
	g.PUT("/deploy/compose/:name", dk, h.ComposeSave)
	g.GET("/deploy/compose/:name/ps", dk, h.ComposePS)
	g.GET("/deploy/compose/:name/logs", dk, h.ComposeLogs)
	g.POST("/deploy/compose/:name/up", dk, h.ComposeUp)
	g.POST("/deploy/compose/:name/down", dk, h.ComposeDown)

	g.GET("/ssh/info", sh, h.SSHInfo)
	g.POST("/ssh/authorize", sh, h.SSHAuthorize)
	g.POST("/ssh/revoke", sh, h.SSHRevoke)
}

func audit(c *gin.Context, a activity_logs.ActivityAction, target string, detail gin.H) {
	detail["via"] = "api-key"
	activity_logs.Audit(c, a, "deploy", target, detail)
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, system.ErrPathDenied):
		utils.Error(c, 403, "path is outside the deploy folders")
	case errors.Is(err, os.ErrNotExist), errors.Is(err, docker.ErrNotFound):
		utils.Error(c, 404, "not found")
	case errors.Is(err, docker.ErrBadInput):
		utils.Error(c, 400, err.Error())
	default:
		utils.Error(c, 422, err.Error())
	}
}

func (h *Deploy) dir(c *gin.Context, raw string) (string, bool) {
	p, err := h.access.Resolve(raw)
	if err != nil {
		fail(c, err)
		return "", false
	}
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		utils.Error(c, 400, "not a directory")
		return "", false
	}
	return p, true
}

func (h *Deploy) Ping(c *gin.Context) {
	utils.Success(c, 200, "ok", gin.H{"roots": h.roots, "max_chunk": MaxChunk})
}

func (h *Deploy) Roots(c *gin.Context) {
	for _, r := range h.roots {
		_ = os.MkdirAll(r, 0o755)
	}
	utils.Success(c, 200, "ok", h.roots)
}

func (h *Deploy) List(c *gin.Context) {
	p, ok := h.dir(c, c.Query("path"))
	if !ok {
		return
	}
	entries, err := filemanager.List(p)
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"path": p, "entries": entries})
}

func (h *Deploy) Mkdir(c *gin.Context) {
	var in struct {
		Path string `json:"path" binding:"required"`
	}
	if c.ShouldBindJSON(&in) != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, err := h.access.Resolve(in.Path)
	if err != nil {
		fail(c, err)
		return
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		utils.Error(c, 422, "could not create the folder")
		return
	}
	audit(c, activity_logs.ActionCreate, p, gin.H{"op": "mkdir"})
	utils.Success(c, 200, "ok", gin.H{"path": p})
}

// target resolves ?dir=&name= to the final file and its .part companion, rejecting anything that could leave the folder.
func (h *Deploy) target(c *gin.Context) (final, part string, ok bool) {
	dir, ok := h.dir(c, c.Query("dir"))
	if !ok {
		return
	}
	name := c.Query("name")
	if !filemanager.ValidName(name) || strings.HasSuffix(name, ".part") {
		utils.Error(c, 400, "invalid file name")
		return "", "", false
	}
	final = filepath.Join(dir, name)
	part = final + ".part"
	for _, p := range []string{final, part} {
		if _, err := h.access.Resolve(p); err != nil {
			fail(c, err)
			return "", "", false
		}
	}
	return final, part, true
}

// UploadStatus tells the client how many bytes of an interrupted upload are already there, so it can resume.
func (h *Deploy) UploadStatus(c *gin.Context) {
	final, part, ok := h.target(c)
	if !ok {
		return
	}
	var size int64
	if st, err := os.Lstat(part); err == nil && st.Mode().IsRegular() {
		size = st.Size()
	}
	_, err := os.Lstat(final)
	utils.Success(c, 200, "ok", gin.H{"size": size, "exists": err == nil})
}

// Chunk appends one piece (raw body, at most MaxChunk) at ?offset= of a file of ?total= bytes. Offsets must line up with what is
// already stored (409 + the stored size otherwise); the last piece moves the .part file into place and, if ?sha256= is given, verifies it.
func (h *Deploy) Chunk(c *gin.Context) {
	final, part, ok := h.target(c)
	if !ok {
		return
	}
	offset, e1 := strconv.ParseInt(c.Query("offset"), 10, 64)
	total, e2 := strconv.ParseInt(c.Query("total"), 10, 64)
	if e1 != nil || e2 != nil || offset < 0 || total < 1 || total > maxTotal || offset >= total {
		utils.Error(c, 400, "invalid offset or total")
		return
	}
	overwrite := c.Query("overwrite") == "1"
	if _, err := os.Lstat(final); err == nil && !overwrite {
		utils.Error(c, 409, "file already exists")
		return
	}
	var size int64
	if st, err := os.Lstat(part); err == nil {
		if !st.Mode().IsRegular() {
			utils.Error(c, 400, "invalid upload state")
			return
		}
		size = st.Size()
	}
	if offset != 0 && offset != size {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"success": false, "message": "offset does not match the stored size", "data": gin.H{"size": size}})
		return
	}
	flags := os.O_WRONLY | os.O_CREATE | syscall.O_NOFOLLOW
	if offset == 0 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flags, 0o640)
	if err != nil {
		utils.Error(c, 422, "could not write the file")
		return
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		utils.Error(c, 422, "could not write the file")
		return
	}
	n, err := io.Copy(f, io.LimitReader(c.Request.Body, MaxChunk+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil || n > MaxChunk || offset+n > total {
		_ = os.Truncate(part, offset)
		utils.Error(c, 400, "chunk too large or upload interrupted")
		return
	}
	if offset+n < total {
		utils.Success(c, 200, "ok", gin.H{"done": false, "size": offset + n})
		return
	}
	if want := strings.ToLower(c.Query("sha256")); want != "" {
		got, err := fileSHA(part)
		if err != nil || got != want {
			_ = os.Remove(part)
			utils.Error(c, 422, "checksum mismatch, upload again")
			return
		}
	}
	backedUp, err := replaceWithBackup(part, final)
	if err != nil {
		utils.Error(c, 422, "could not finish the upload")
		return
	}
	audit(c, activity_logs.ActionCreate, final, gin.H{"op": "upload", "size": total, "backup": backedUp})
	utils.Success(c, 200, "ok", gin.H{"done": true, "size": total, "path": final, "backup": backedUp})
}

// replaceWithBackup moves the finished upload `part` into place as `final`. A file that is already there is kept as
// "bak-<name>" first; an older bak-<name> is deleted to make room, so exactly one previous version is kept. The swap happens
// only now that the new file is complete and verified, so a failed upload never touches the current one. If the final move
// fails, the previous file is put back.
func replaceWithBackup(part, final string) (backup string, err error) {
	if st, e := os.Lstat(final); e == nil {
		if !st.Mode().IsRegular() {
			return "", errors.New("not a regular file")
		}
		backup = filepath.Join(filepath.Dir(final), "bak-"+filepath.Base(final))
		if bst, e := os.Lstat(backup); e == nil {
			if !bst.Mode().IsRegular() {
				return "", errors.New("backup is not a regular file")
			}
			if err := os.Remove(backup); err != nil {
				return "", err
			}
		}
		if err := os.Rename(final, backup); err != nil {
			return "", err
		}
	}
	if err := os.Rename(part, final); err != nil {
		if backup != "" {
			_ = os.Rename(backup, final)
		}
		return "", err
	}
	return backup, nil
}

func fileSHA(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (h *Deploy) LoadImage(c *gin.Context) {
	var in struct {
		Path string `json:"path" binding:"required"`
	}
	if c.ShouldBindJSON(&in) != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	p, err := h.access.Resolve(in.Path)
	if err != nil {
		fail(c, err)
		return
	}
	if st, err := os.Stat(p); err != nil || !st.Mode().IsRegular() {
		utils.Error(c, 404, "file not found")
		return
	}
	refs, err := h.d.Load(context.WithoutCancel(c.Request.Context()), p)
	if err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionCreate, p, gin.H{"op": "docker load", "images": refs})
	utils.Success(c, 200, "image loaded", gin.H{"images": refs})
}

func (h *Deploy) Images(c *gin.Context) {
	imgs, err := h.d.ListImages(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", imgs)
}

func (h *Deploy) ComposeList(c *gin.Context) {
	ps, err := h.c.List()
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"dir": h.c.Dir, "projects": ps})
}

func (h *Deploy) ComposeGet(c *gin.Context) {
	s, err := h.c.Get(c.Param("name"))
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"name": c.Param("name"), "content": s})
}

// ComposeSave creates the project or replaces its compose file.
func (h *Deploy) ComposeSave(c *gin.Context) {
	var in struct {
		Content string `json:"content" binding:"required"`
	}
	if c.ShouldBindJSON(&in) != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	name := c.Param("name")
	_, getErr := h.c.Get(name)
	if err := h.c.Save(name, in.Content, getErr != nil); err != nil {
		fail(c, err)
		return
	}
	audit(c, activity_logs.ActionUpdate, name, gin.H{"op": "compose save"})
	utils.Success(c, 200, "saved", nil)
}

func (h *Deploy) compose(c *gin.Context, op string, fn func(context.Context, string) (string, error)) {
	out, err := fn(context.WithoutCancel(c.Request.Context()), c.Param("name"))
	if op != "ps" {
		audit(c, activity_logs.ActionUpdate, c.Param("name"), gin.H{"op": "compose " + op, "ok": err == nil})
	}
	if err != nil {
		fail(c, err)
		return
	}
	utils.Success(c, 200, "ok", gin.H{"output": out})
}

func (h *Deploy) ComposeUp(c *gin.Context)   { h.compose(c, "up", h.c.Up) }
func (h *Deploy) ComposeDown(c *gin.Context) { h.compose(c, "down", h.c.Down) }
func (h *Deploy) ComposePS(c *gin.Context)   { h.compose(c, "ps", h.c.PS) }
func (h *Deploy) ComposeLogs(c *gin.Context) {
	h.compose(c, "ps", func(ctx context.Context, n string) (string, error) {
		return h.c.Logs(ctx, n, utils.QueryInt(c, "tail", 200))
	})
}
