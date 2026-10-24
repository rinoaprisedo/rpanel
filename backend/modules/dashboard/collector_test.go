package dashboard

import (
	"context"
	"testing"
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
