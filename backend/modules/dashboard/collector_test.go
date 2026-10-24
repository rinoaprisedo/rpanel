package dashboard

import (
	"context"
	"testing"

	"rpanel/backend/system"

	"github.com/gin-gonic/gin"
)

func TestSampleFillsOverviewAndCapsHistory(t *testing.T) {
	c := NewCollector()
	ctx := context.Background()
	for i := 0; i < historySize+5; i++ {
		c.sample(ctx)
	}
	o := c.Overview()
	if o.MemTotal == 0 || o.CPUCores == 0 || o.Hostname == "" {
		t.Fatalf("overview not populated: %+v", o)
	}
	if got := len(c.History()); got != historySize {
		t.Fatalf("history len = %d, want %d", got, historySize)
	}
}

func TestTopProcessesLimit(t *testing.T) {
	ps := TopProcesses(context.Background(), "mem", 3)
	if len(ps) == 0 || len(ps) > 3 {
		t.Fatalf("got %d processes", len(ps))
	}
}

func TestRoutesRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allow := func(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
	h := &Handler{collector: NewCollector(), analyzer: NewAnalyzer(t.TempDir(), func() []string { return nil })}
	h.probes = NewProbes(system.Default, nil, "", h.analyzer)
	h.RegisterRoutes(gin.New().Group("/api/v1"), allow)
}
