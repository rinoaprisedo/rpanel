package dashboard

import (
	"context"
	"encoding/gob"
	"os"
	"path/filepath"
	"time"
)

const persistEvery = 2 * time.Minute

// historyFile is what is written to disk: the finished per-minute and per-hour series.
// The minute and hour still in progress are not saved, so a restart loses at most a couple of minutes.
type historyFile struct {
	Long   []Point
	Hourly []Point
}

// LoadHistory restores the series saved by an earlier run. A missing or unreadable file is not an
// error: the charts simply start empty. Call it before Start.
func (c *Collector) LoadHistory(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	var h historyFile
	if gob.NewDecoder(f).Decode(&h) != nil {
		return
	}
	now := time.Now().Unix()
	keep := func(src []Point, window time.Duration, max int) []Point {
		out := src[:0:0]
		for _, p := range src {
			if p.Time >= now-int64(window/time.Second) && p.Time <= now {
				out = append(out, p)
			}
		}
		if len(out) > max {
			out = out[len(out)-max:]
		}
		return out
	}
	c.mu.Lock()
	c.long = keep(h.Long, minuteSeriesWindow, longHistoryMinutes)
	c.hourly = keep(h.Hourly, 366*day, hourlyHistoryHours)
	c.mu.Unlock()
}

// SaveHistory writes the series atomically (temp file, then rename).
func (c *Collector) SaveHistory(path string) error {
	c.mu.RLock()
	h := historyFile{Long: append([]Point(nil), c.long...), Hourly: append([]Point(nil), c.hourly...)}
	c.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".history-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := gob.NewEncoder(tmp).Encode(h); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// PersistHistory saves the series every few minutes until ctx is done, and once more on the way out.
func (c *Collector) PersistHistory(ctx context.Context, path string) {
	go func() {
		t := time.NewTicker(persistEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = c.SaveHistory(path)
				return
			case <-t.C:
				_ = c.SaveHistory(path)
			}
		}
	}()
}
