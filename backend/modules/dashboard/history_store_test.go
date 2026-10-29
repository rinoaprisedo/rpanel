package dashboard

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHistorySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "h.gob")
	now := time.Now().Unix()
	a := NewCollector()
	a.long = []Point{{Time: now - 3600, CPU: 12}, {Time: now - 60, CPU: 34}}
	a.hourly = []Point{{Time: now - 40*24*3600, CPU: 56}}
	if err := a.SaveHistory(path); err != nil {
		t.Fatal(err)
	}

	b := NewCollector()
	b.LoadHistory(path)
	if len(b.long) != 2 || b.long[1].CPU != 34 || len(b.hourly) != 1 || b.hourly[0].CPU != 56 {
		t.Fatalf("history not restored: long=%+v hourly=%+v", b.long, b.hourly)
	}
	if pts, _ := b.HistoryRange("24h"); len(pts) == 0 {
		t.Error("restored points must show up in the charts")
	}
}

func TestLoadHistoryDropsOldAndIgnoresBadFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.gob")
	now := time.Now().Unix()
	a := NewCollector()
	a.long = []Point{{Time: now - 30*24*3600, CPU: 1}, {Time: now - 60, CPU: 2}} // 30 days old: beyond the minute series
	if err := a.SaveHistory(path); err != nil {
		t.Fatal(err)
	}
	b := NewCollector()
	b.LoadHistory(path)
	if len(b.long) != 1 || b.long[0].CPU != 2 {
		t.Errorf("old minute points must be dropped: %+v", b.long)
	}

	NewCollector().LoadHistory(filepath.Join(t.TempDir(), "missing.gob")) // must not panic
}
