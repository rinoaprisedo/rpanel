package dashboard

import (
	"fmt"
	"sort"
	"time"
)

const longHistoryMinutes = 7 * 24 * 60

// minuteAcc averages the 3-second samples of the minute in progress.
type minuteAcc struct {
	minute int64
	n      int
	sum    Point
}

func (a *minuteAcc) add(p Point) {
	a.n++
	a.sum.CPU += p.CPU
	a.sum.MemPercent += p.MemPercent
	a.sum.NetRxBps += p.NetRxBps
	a.sum.NetTxBps += p.NetTxBps
	a.sum.Load1 += p.Load1
	a.sum.Disk += p.Disk
}

func (a *minuteAcc) avg() Point {
	n := float64(a.n)
	return Point{
		Time: a.minute * 60, CPU: a.sum.CPU / n, MemPercent: a.sum.MemPercent / n,
		NetRxBps: a.sum.NetRxBps / n, NetTxBps: a.sum.NetTxBps / n, Load1: a.sum.Load1 / n, Disk: a.sum.Disk / n,
	}
}

// addLong folds a sample into the per-minute series. Caller holds c.mu.
func (c *Collector) addLong(p Point) {
	m := p.Time / 60
	if c.acc.n > 0 && c.acc.minute != m {
		c.long = append(c.long, c.acc.avg())
		if len(c.long) > longHistoryMinutes {
			c.long = c.long[len(c.long)-longHistoryMinutes:]
		}
		c.acc = minuteAcc{}
	}
	c.acc.minute = m
	c.acc.add(p)
}

// rangeSpec maps a range key to its window and chart resolution.
type rangeSpec struct {
	window time.Duration
	bucket time.Duration
}

var historyRanges = map[string]rangeSpec{
	"1h":  {time.Hour, time.Minute},
	"6h":  {6 * time.Hour, 5 * time.Minute},
	"24h": {24 * time.Hour, 15 * time.Minute},
	"7d":  {7 * 24 * time.Hour, 2 * time.Hour},
}

func validRange(r string) bool { _, ok := historyRanges[r]; return ok }

// HistoryRange returns averaged points for the window, at most ~170 of them.
func (c *Collector) HistoryRange(key string) ([]Point, error) {
	spec, ok := historyRanges[key]
	if !ok {
		return nil, fmt.Errorf("unknown range %q", key)
	}
	c.mu.RLock()
	src := make([]Point, len(c.long), len(c.long)+1)
	copy(src, c.long)
	if c.acc.n > 0 {
		src = append(src, c.acc.avg()) // the minute in progress, so the right edge is live
	}
	c.mu.RUnlock()
	return downsample(src, time.Now().Add(-spec.window).Unix(), int64(spec.bucket/time.Second)), nil
}

// downsample averages points at or after since into fixed buckets.
func downsample(src []Point, since, bucket int64) []Point {
	type agg struct {
		n   int
		sum Point
	}
	groups := map[int64]*agg{}
	for _, p := range src {
		if p.Time < since {
			continue
		}
		k := p.Time / bucket * bucket
		g := groups[k]
		if g == nil {
			g = &agg{}
			groups[k] = g
		}
		g.n++
		g.sum.CPU += p.CPU
		g.sum.MemPercent += p.MemPercent
		g.sum.NetRxBps += p.NetRxBps
		g.sum.NetTxBps += p.NetTxBps
		g.sum.Load1 += p.Load1
		g.sum.Disk += p.Disk
	}
	keys := make([]int64, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]Point, 0, len(keys))
	for _, k := range keys {
		g, n := groups[k], float64(groups[k].n)
		out = append(out, Point{
			Time: k, CPU: g.sum.CPU / n, MemPercent: g.sum.MemPercent / n, NetRxBps: g.sum.NetRxBps / n,
			NetTxBps: g.sum.NetTxBps / n, Load1: g.sum.Load1 / n, Disk: g.sum.Disk / n,
		})
	}
	return out
}
