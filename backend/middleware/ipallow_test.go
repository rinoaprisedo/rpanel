package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func engine(t *testing.T, allow []string, trusted []string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	nets, err := ParseIPList(allow)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.SetTrustedProxies(trusted)
	r.Use(IPAllowList(nets))
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })
	return r
}

func get(r *gin.Engine, remote, xff string) int {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestIPAllowList(t *testing.T) {
	r := engine(t, []string{"203.0.113.7", "10.0.0.0/8", "2001:db8::/32"}, nil)
	for remote, want := range map[string]int{
		"203.0.113.7:1234": 200, "10.9.8.7:1": 200, "[2001:db8::5]:1": 200,
		"203.0.113.8:1": 403, "8.8.8.8:1": 403, "[2001:dead::1]:1": 403,
	} {
		if got := get(r, remote, ""); got != want {
			t.Errorf("%s: got %d want %d", remote, got, want)
		}
	}
}

func TestEmptyListAllowsAll(t *testing.T) {
	if got := get(engine(t, nil, nil), "8.8.8.8:1", ""); got != 200 {
		t.Fatalf("got %d", got)
	}
}

// X-Forwarded-For must not be believed from untrusted peers (spoofing bypass).
func TestForwardedForOnlyFromTrustedProxy(t *testing.T) {
	r := engine(t, []string{"203.0.113.7"}, []string{"127.0.0.1"})
	if got := get(r, "8.8.8.8:1", "203.0.113.7"); got != 403 {
		t.Fatalf("spoofed XFF from untrusted peer must be rejected, got %d", got)
	}
	if got := get(r, "127.0.0.1:1", "203.0.113.7"); got != 200 {
		t.Fatalf("XFF via trusted proxy must be honoured, got %d", got)
	}
	if got := get(r, "127.0.0.1:1", "8.8.8.8"); got != 403 {
		t.Fatalf("real client outside list via proxy must be rejected, got %d", got)
	}
}

func TestParseIPListRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"not-an-ip", "1.2.3.4/99", "1.2.3"} {
		if _, err := ParseIPList([]string{bad}); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}
