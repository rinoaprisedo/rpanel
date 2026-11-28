package panelssl

import (
	"context"
	"strings"
	"time"

	"rpanel/backend/system"
)

// OpenWebPorts allows 80/tcp (the ACME challenge) and 443/tcp in UFW before a certificate is requested,
// so a firewall on this server never makes Let's Encrypt time out. It only acts when UFW is installed and
// active, is idempotent, and never fails the caller: the returned note says what happened ("" = nothing to do).
func OpenWebPorts(ctx context.Context, run system.Runner) string {
	res, err := run.Run(ctx, 15*time.Second, "ufw", "status")
	if err != nil || !strings.Contains(res.Stdout, "Status: active") {
		return ""
	}
	for _, p := range []string{"80/tcp", "443/tcp"} {
		if r, err := run.Run(ctx, 30*time.Second, "ufw", "allow", p); err != nil {
			return "Could not open " + p + " in the firewall: " + strings.TrimSpace(r.Stdout+" "+r.Stderr)
		}
	}
	return "Firewall: ports 80 and 443 are open"
}
