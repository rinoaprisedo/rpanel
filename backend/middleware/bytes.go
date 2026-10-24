package middleware

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxBytesReader is a thin wrapper around http.MaxBytesReader kept in its own
// file for clarity/reuse across middleware.
func MaxBytesReader(c *gin.Context, body io.ReadCloser, n int64) io.ReadCloser {
	return http.MaxBytesReader(c.Writer, body, n)
}
