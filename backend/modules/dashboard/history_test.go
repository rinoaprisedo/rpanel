package dashboard

import (
	"testing"
	"time"
)

func TestAddLongAveragesPerMinute(t *testing.T) {
	c := NewCollector()
	base := int64(1_700_000_040) // exactly on a minute
	for i := 0; i < 20; i++ {    // 20 samples, 3 s apart = one minute
		c.mu.Lock()
		c.addLong(Point{Time: base + int64(i)*3, CPU: float64(i % 2 * 100), MemPercent: 50})
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.addLong(Point{Time: base + 60, CPU: 10, MemPercent: 70}) // next minute closes the first
	c.mu.Unlock()
	if len(c.long) != 1 {
		t.Fatalf("want 1 finished minute, got %d", len(c.long))
	}
	if got := c.long[0]; got.CPU != 50 || got.MemPercent != 50 || got.Time != base {
		t.Errorf("minute average wrong: %+v", got)
	}
}

func TestDownsampleBucketsAndWindow(t *testing.T) {
	var src []Point
	for m := int64(0); m < 120; m++ { // 2 hours of minutes, cpu = minute index
		src = append(src, Point{Time: 3600*1000 + m*60, CPU: float64(m)})
	}
	got := downsample(src, 3600*1000+60*60, 15*60) // last hour, 15-minute buckets
	if len(got) != 4 {
		t.Fatalf("want 4 buckets, got %d: %+v", len(got), got)
	}
	if got[0].CPU != 67 { // minutes 60..74 average
		t.Errorf("first bucket avg = %v, want 67", got[0].CPU)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Time <= got[i-1].Time {
			t.Error("buckets must be in time order")
		}
	}
}

func TestHistoryRangeRejectsUnknown(t *testing.T) {
	if _, err := NewCollector().HistoryRange("30y"); err == nil {
		t.Error("unknown range must be rejected")
	}
}

func TestAddHourlyAveragesPerHour(t *testing.T) {
	c := NewCollector()
	base := int64(1_700_002_800)     // on an hour boundary
	for m := int64(0); m < 61; m++ { // 61 minutes: the last one closes hour 1
		c.mu.Lock()
		c.addHourly(Point{Time: base + m*60, CPU: float64(m)})
		c.mu.Unlock()
	}
	if len(c.hourly) != 1 {
		t.Fatalf("want 1 finished hour, got %d", len(c.hourly))
	}
	if got := c.hourly[0]; got.CPU != 29.5 || got.Time != base {
		t.Errorf("hour average wrong: %+v", got)
	}
}

func TestLongRangesUseHourlySeries(t *testing.T) {
	c := NewCollector()
	now := time.Now().Unix()
	c.hourly = []Point{{Time: now - 100*24*3600, CPU: 40}}
	for _, k := range []string{"30d", "90d", "180d", "365d"} {
		if _, err := c.HistoryRange(k); err != nil {
			t.Errorf("%s must be accepted: %v", k, err)
		}
	}
	if pts, _ := c.HistoryRange("90d"); len(pts) != 0 {
		t.Errorf("a point from 100 days ago is outside 90d: %+v", pts)
	}
	if pts, _ := c.HistoryRange("180d"); len(pts) != 1 {
		t.Errorf("180d should include it: %+v", pts)
	}
}
