package website

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	rateLimitRe = regexp.MustCompile(`(?i)too many (failed authorizations|certificates|new orders|failed authorizations)[^\n]*?retry after (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC`)
	acmeDomain  = regexp.MustCompile(`(?m)^\s*Domain:\s*(\S+)`)
	acmeDetail  = regexp.MustCompile(`(?m)^\s*Detail:\s*(.+)$`)
)

// summarizeCertbotFailure turns certbot's output into the sentence a person needs: the Let's Encrypt rate
// limit with the time to retry, or the validation problem with its detail line. retry is set for rate limits.
// Anything else falls back to the last lines of the output.
func summarizeCertbotFailure(out string) (msg string, retry *time.Time) {
	if m := rateLimitRe.FindStringSubmatch(out); m != nil {
		if t, err := time.Parse("2006-01-02 15:04:05", m[2]); err == nil {
			t = t.UTC()
			return fmt.Sprintf("Let's Encrypt rate limit reached (%s). Wait until %s UTC before trying again; fix the cause first, every failed attempt counts.",
				strings.ToLower(m[1]), t.Format("2006-01-02 15:04")), &t
		}
	}
	if d := acmeDetail.FindStringSubmatch(out); d != nil {
		who := "the domain"
		if m := acmeDomain.FindStringSubmatch(out); m != nil {
			who = m[1]
		}
		return fmt.Sprintf("Let's Encrypt could not validate %s: %s", who, strings.TrimSpace(d[1])), nil
	}
	return "certbot failed: " + lastLines(out, 6), nil
}
