package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// ParseIPList parses entries that are single IPs or CIDRs.
func ParseIPList(entries []string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if ip := net.ParseIP(e); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(e)
		if err != nil {
			return nil, &net.ParseError{Type: "IP address or CIDR", Text: e}
		}
		out = append(out, n)
	}
	return out, nil
}

// IPAllowList rejects every request whose client IP is outside nets. An empty
// list allows everyone. The client IP is c.ClientIP(), which only honours
// X-Forwarded-For from the configured trusted proxies.
func IPAllowList(nets []*net.IPNet) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(nets) == 0 {
			c.Next()
			return
		}
		if ip := net.ParseIP(c.ClientIP()); ip != nil {
			for _, n := range nets {
				if n.Contains(ip) {
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatus(403)
	}
}
