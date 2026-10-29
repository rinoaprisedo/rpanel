package dashboard

import (
	"fmt"
	"sort"
	"time"
)

const (
	longHistoryMinutes = 7 * 24 * 60
	hourlyHistoryHours = 366 * 24
)

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
	a.sum.DiskRead += p.DiskRead
	a.sum.DiskWrite += p.DiskWrite
}

func (a *minuteAcc) avg() Point {
	n := float64(a.n)
	return Point{
		Time: a.minute * 60, CPU: a.sum.CPU / n, MemPercent: a.sum.MemPercent / n,
		NetRxBps: a.sum.NetRxBps / n, NetTxBps: a.sum.NetTxBps / n, Load1: a.sum.Load1 / n, Disk: a.sum.Disk / n,
		DiskRead: a.sum.DiskRead / n, DiskWrite: a.sum.DiskWrite / n,
	}
}

// addLong folds a sample into the per-minute series. Caller holds c.mu.
func (c *Collector) addLong(p Point) {
	m := p.Time / 60
	if c.acc.n > 0 && c.acc.minute != m {
		done := c.acc.avg()
		c.long = append(c.long, done)
		if len(c.long) > longHistoryMinutes {
			c.long = c.long[len(c.long)-longHistoryMinutes:]
		}
		c.addHourly(done)
		c.acc = minuteAcc{}
	}
	c.acc.minute = m
	c.acc.add(p)
}

// addHourly folds a finished minute into the per-hour series. Caller holds c.mu.
// The accumulator's "minute" field holds the hour start, in minutes, so avg() stamps it correctly.
func (c *Collector) addHourly(p Point) {
	h := p.Time / 3600 * 60
	if c.hacc.n > 0 && c.hacc.minute != h {
		c.hourly = append(c.hourly, c.hacc.avg())
		if len(c.hourly) > hourlyHistoryHours {
			c.hourly = c.hourly[len(c.hourly)-hourlyHistoryHours:]
		}
		c.hacc = minuteAcc{}
	}
	c.hacc.minute = h
	c.hacc.add(p)
}

// rangeSpec maps a range key to its window and chart resolution.
type rangeSpec struct {
	window time.Duration
	bucket time.Duration
}

// Windows longer than the per-minute series are drawn from the hourly one.
const minuteSeriesWindow = 7 * 24 * time.Hour

const day = 24 * time.Hour

var historyRanges = map[string]rangeSpec{
	"1h":   {time.Hour, time.Minute},
	"6h":   {6 * time.Hour, 5 * time.Minute},
	"24h":  {24 * time.Hour, 15 * time.Minute},
	"7d":   {7 * day, 2 * time.Hour},
	"30d":  {30 * day, 6 * time.Hour},
	"90d":  {90 * day, 12 * time.Hour},
	"180d": {180 * day, day},
	"365d": {365 * day, 2 * day},
}

func validRange(r string) bool { _, ok := historyRanges[r]; return ok }

// HistoryRange returns averaged points for the window, at most ~170 of them.
func (c *Collector) HistoryRange(key string) ([]Point, error) {
	spec, ok := historyRanges[key]
	if !ok {
		return nil, fmt.Errorf("unknown range %q", key)
	}
	c.mu.RLock()
	var src []Point
	if spec.window > minuteSeriesWindow {
		src = make([]Point, len(c.hourly), len(c.hourly)+1)
		copy(src, c.hourly)
		if c.hacc.n > 0 {
			src = append(src, c.hacc.avg()) // the hour in progress, so the right edge is live
		}
	} else {
		src = make([]Point, len(c.long), len(c.long)+1)
		copy(src, c.long)
		if c.acc.n > 0 {
			src = append(src, c.acc.avg()) // the minute in progress, so the right edge is live
		}
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
		g.sum.DiskRead += p.DiskRead
		g.sum.DiskWrite += p.DiskWrite
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
			DiskRead: g.sum.DiskRead / n, DiskWrite: g.sum.DiskWrite / n,
		})
	}
	return out
}
