package dashboard

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// releaseRepo is where RPanel releases are published (GitHub Releases).
const releaseRepo = "rinoaprisedo/rpanel"

// Version is the running build's version; main sets it at start-up.
var Version = "dev"

// Release says whether a newer RPanel than the running one has been published.
type Release struct {
	Known     bool   `json:"known"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	URL       string `json:"url,omitempty"`
}

// versionParts turns "v1.12.3" into [1 12 3]; non-numeric parts count as 0.
func versionParts(v string) [3]int {
	var out [3]int
	for i, s := range strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".", 3) {
		n, _ := strconv.Atoi(strings.SplitN(s, "-", 2)[0])
		out[i] = n
	}
	return out
}

func newerVersion(latest, current string) bool {
	a, b := versionParts(latest), versionParts(current)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// latestTag reads the tag GitHub redirects "/releases/latest" to, which needs no
// API token and has no rate limit worth worrying about at one request per hours.
func latestTag(ctx context.Context, base string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, base+"/"+releaseRepo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	res.Body.Close()
	loc := res.Header.Get("Location")
	i := strings.LastIndex(loc, "/releases/tag/")
	if i < 0 {
		return "", http.ErrNoLocation
	}
	return loc[i+len("/releases/tag/"):], nil
}

func (p *Probes) refreshRelease(ctx context.Context) {
	tag, err := latestTag(ctx, p.releaseBase)
	if err != nil {
		return
	}
	latest := strings.TrimPrefix(tag, "v")
	p.mu.Lock()
	defer p.mu.Unlock()
	p.release = Release{
		Known: true, Current: Version, Latest: latest, Available: newerVersion(latest, Version),
		URL: "https://github.com/" + releaseRepo + "/releases/tag/" + tag,
	}
}

// StartReleaseChecks looks for a newer published version shortly after start and then every 6 hours.
func (p *Probes) StartReleaseChecks(ctx context.Context) {
	go func() {
		t := time.NewTimer(45 * time.Second)
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
				p.refreshRelease(ctx)
				t.Reset(6 * time.Hour)
			}
		}
	}()
}
