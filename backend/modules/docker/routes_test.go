package docker

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"rpanel/backend/config"

	"github.com/gin-gonic/gin"
)

// Gin panics at registration time on conflicting wildcards; this catches that.
func TestRoutesRegisterAndRemoveGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	allow := func(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
	NewHandler(&config.Config{ComposeDir: t.TempDir()}).RegisterRoutes(r.Group("/api/v1"), allow)

	// "remove" through the generic action route must be refused: it needs the delete permission.
	req := httptest.NewRequest("POST", "/api/v1/docker/containers/web/remove", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
