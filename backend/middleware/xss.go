package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// XSSSanitizer strips HTML tags from all string values in a JSON request
// body before the handler decodes it. skipPrefixes are bearer-token APIs whose
// text is only ever rendered escaped (notes legitimately contain code like
// <Component>).
func XSSSanitizer(skipPrefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.ContentType() != "application/json" || hasPrefix(c.Request.URL.Path, skipPrefixes) {
			c.Next()
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil || len(body) == 0 {
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			c.Next()
			return
		}

		var generic interface{}
		if err := json.Unmarshal(body, &generic); err == nil {
			sanitized := sanitizeValue(generic)
			if b, err := json.Marshal(sanitized); err == nil {
				body = b
			}
		}

		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Next()
	}
}

func sanitizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return utils.StripTags(val)
	case map[string]interface{}:
		for k, item := range val {
			val[k] = sanitizeValue(item)
		}
		return val
	case []interface{}:
		for i, item := range val {
			val[i] = sanitizeValue(item)
		}
		return val
	default:
		return v
	}
}
