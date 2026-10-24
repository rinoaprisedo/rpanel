package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		latest, cur string
		want        bool
	}{
		{"1.3.0", "1.2.0", true}, {"v1.10.0", "1.9.9", true}, {"1.2.0", "1.2.0", false},
		{"1.2.0", "1.3.0", false}, {"2.0.0", "1.99.99", true}, {"1.2.1-rc1", "1.2.0", true}, {"1.2.0", "dev", true},
	} {
		if got := newerVersion(c.latest, c.cur); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", c.latest, c.cur, got, c.want)
		}
	}
}

func TestRefreshReleaseReadsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+releaseRepo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/"+releaseRepo+"/releases/tag/v9.9.9", http.StatusFound)
	}))
	defer srv.Close()
	Version = "1.2.0"
	p := &Probes{releaseBase: srv.URL}
	p.refreshRelease(context.Background())
	if !p.release.Known || p.release.Latest != "9.9.9" || !p.release.Available {
		t.Fatalf("release = %+v", p.release)
	}
	got := buildInsights(insightInput{Release: p.release, Overview: Overview{CPUCores: 1}})
	if find3(got, "release") == nil {
		t.Errorf("update insight missing: %s", ids(got))
	}
}

func TestRefreshReleaseIgnoresFailures(t *testing.T) {
	p := &Probes{releaseBase: "http://127.0.0.1:1"}
	p.refreshRelease(context.Background())
	if p.release.Known {
		t.Error("a failed check must not report a release")
	}
}
