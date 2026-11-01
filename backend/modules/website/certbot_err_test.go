package website

import (
	"strings"
	"testing"
)

func TestSummarizeCertbotFailure(t *testing.T) {
	rl := "Saving debug log\nAn unexpected error occurred:\ntoo many failed authorizations (5) for \"reprimehub.com\" in the last 1h0m0s, retry after 2026-11-01 13:03:17 UTC: see https://letsencrypt.org/docs/rate-limits/\n"
	msg, retry := summarizeCertbotFailure(rl)
	if retry == nil || retry.Format("15:04:05") != "13:03:17" || !strings.Contains(msg, "2026-11-01 13:03 UTC") {
		t.Errorf("rate limit: %q %v", msg, retry)
	}
	val := "Certbot failed to authenticate some domains\n  Domain: reprimehub.com\n  Type:   connection\n  Detail: 1.2.3.4: Fetching http://reprimehub.com/.well-known/acme-challenge/x: Timeout during connect (likely firewall problem)\n\nHint: blah\n"
	msg, retry = summarizeCertbotFailure(val)
	if retry != nil || !strings.Contains(msg, "reprimehub.com: 1.2.3.4: Fetching") || !strings.Contains(msg, "firewall") {
		t.Errorf("validation: %q", msg)
	}
	msg, _ = summarizeCertbotFailure("a\nb")
	if !strings.HasPrefix(msg, "certbot failed:") {
		t.Errorf("fallback: %q", msg)
	}
}
