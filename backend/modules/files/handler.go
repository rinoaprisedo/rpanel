package files

import (
	"errors"
	"log"
	"strings"

	"rpanel/backend/storage"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// Handler serves private uploads (outside the public /uploads store) to
// staff whose role may view the menu that owns the folder.
type Handler struct {
	store storage.StorageInterface
}

func NewHandler(store storage.StorageInterface) *Handler {
	return &Handler{store: store}
}

// folderMenus maps a private folder to the menu path whose "view" permission
// unlocks it.
var folderMenus = map[string]string{}

// RegisterRoutes mounts GET /files/*path; guard(menuPath, action) is the same
// session + permission check the modules use.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard func(menuPath, action string) gin.HandlerFunc) {
	handlers := map[string]gin.HandlerFunc{}
	for folder, menu := range folderMenus {
		handlers[folder] = guard(menu, "view")
	}
	rg.GET("/files/*path", func(c *gin.Context) {
		rel := strings.TrimPrefix(c.Param("path"), "/")
		folder, _, _ := strings.Cut(rel, "/")
		guarded, ok := handlers[folder]
		if !ok || strings.Contains(rel, "..") {
			utils.Error(c, 404, "file not found")
			return
		}
		guarded(c)
		if c.IsAborted() {
			return
		}
		h.serve(c, rel)
	})
}

func (h *Handler) serve(c *gin.Context, rel string) {
	obj, err := h.store.Open(rel)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			log.Printf("files %s: %v", rel, err)
		}
		utils.Error(c, 404, "file not found")
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	c.Header("Cache-Control", "private, max-age=300")
	storage.Serve(c.Writer, c.Request, obj, rel)
}
