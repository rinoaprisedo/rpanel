package dashboard

import (
	"fmt"
	"math"
	"sort"
)

// Health levels, best to worst.
const (
	HealthSmooth   = "smooth"   // score >= 85
	HealthFair     = "fair"     // 65-84
	HealthSlow     = "slow"     // 40-64
	HealthStruggle = "struggle" // < 40
)

// HealthFactor is one measured aspect of "is this server running well".
// Score is 0 (bad) to 100 (perfect); Weight says how much it counts overall.
type HealthFactor struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Value  string  `json:"value"`
	Used   float64 `json:"used"` // 0-100 pressure on this resource (what a gauge should fill to); score is the verdict
	Score  float64 `json:"score"`
	Weight float64 `json:"weight"`
	Hint   string  `json:"hint"` // how the score is read, e.g. "Fine below 60%, bad at 100%"
}

// Health is the single "how is the server doing" verdict behind the speedometer.
type Health struct {
	Score      int            `json:"score"`
	Level      string         `json:"level"`
	Label      string         `json:"label"`
	Bottleneck string         `json:"bottleneck,omitempty"` // factor id dragging the score down
	Factors    []HealthFactor `json:"factors"`
}

// lowerBetter maps v to 100 at or below good and 0 at or above bad, linearly between.
func lowerBetter(v, good, bad float64) float64 {
	if v <= good {
		return 100
	}
	if v >= bad {
		return 0
	}
	return 100 * (bad - v) / (bad - good)
}

// buildHealth scores the server from what actually makes a machine feel slow:
// sustained CPU use, run-queue pressure (load per core), memory/swap pressure,
// a nearly full disk, a rising 5xx rate and stopped services. Factors with no
// data are left out rather than counted as perfect. One failing factor caps the
// total so a healthy average cannot hide a real problem.
func buildHealth(in insightInput) Health {
	o := in.Overview
	var fs []HealthFactor
	add := func(id, label, value string, used, score, weight float64, hint string) {
		fs = append(fs, HealthFactor{ID: id, Label: label, Value: value, Used: math.Round(math.Max(0, math.Min(100, used))), Score: math.Round(score), Weight: weight, Hint: hint})
	}

	cpu := o.CPU
	if len(in.Recent) >= 3 { // sustained use matters, one spike does not
		cpu = avg(in.Recent, func(p Point) float64 { return p.CPU })
	}
	add("cpu", "CPU", fmt.Sprintf("%.0f%%", cpu), cpu, lowerBetter(cpu, 60, 100), 25, "Average of the last minutes. Fine below 60%, saturated at 100%")

	cores := float64(o.CPUCores)
	if cores < 1 {
		cores = 1
	}
	perCore := o.Load5 / cores
	add("load", "Load", fmt.Sprintf("%.2f / core", perCore), perCore*100, lowerBetter(perCore, 0.7, 2), 20, "5-minute load per core. Fine below 0.7, tasks queue up beyond 1.0, bad at 2.0")

	mem := lowerBetter(o.MemPct, 80, 98)
	memVal := fmt.Sprintf("%.0f%%", o.MemPct)
	if o.SwapTotal > 0 {
		swap := float64(o.SwapUsed) / float64(o.SwapTotal) * 100
		if s := lowerBetter(swap, 30, 90); s < mem {
			mem = s
		}
		if swap >= 1 {
			memVal += fmt.Sprintf(" · swap %.0f%%", swap)
		}
	}
	add("memory", "Memory", memVal, o.MemPct, mem, 20, "Fine below 80% with little swap in use, heavy swapping makes everything slow")

	if len(o.Disks) > 0 {
		worst := o.Disks[0]
		for _, d := range o.Disks {
			if d.Percent > worst.Percent {
				worst = d
			}
		}
		add("disk", "Disk", fmt.Sprintf("%s free · %s", human(worst.Total-worst.Used), worst.Mount), worst.Percent, lowerBetter(worst.Percent, 80, 98), 15, "Fullest disk. Fine below 80%, writes start failing near 100%")
	}

	if t := in.Traffic1h; t != nil && t.Available && t.Totals.Requests >= 20 {
		rate := float64(t.Totals.ServerErr) / float64(t.Totals.Requests) * 100
		add("errors", "Server errors", fmt.Sprintf("%.1f%% 5xx", rate), rate, lowerBetter(rate, 0.5, 5), 10, "Share of 5xx responses in the last hour. Fine below 0.5%, bad at 5%")
	}

	if len(in.Services) > 0 {
		down := 0
		for _, s := range in.Services {
			if !s.Running {
				down++
			}
		}
		add("services", "Services", fmt.Sprintf("%d of %d running", len(in.Services)-down, len(in.Services)), float64(down)/float64(len(in.Services))*100, math.Max(0, 100-50*float64(down)), 10, "Each stopped service costs half the score")
	}

	var sum, wsum float64
	lowest := -1
	for i, f := range fs {
		sum += f.Score * f.Weight
		wsum += f.Weight
		if lowest < 0 || f.Score < fs[lowest].Score {
			lowest = i
		}
	}
	h := Health{Factors: fs}
	if wsum == 0 {
		h.Score, h.Level, h.Label = 100, HealthSmooth, "Running smoothly"
		return h
	}
	score := sum / wsum
	if fs[lowest].Score < 25 && score > 60 {
		score = 60
	}
	h.Score = int(math.Round(score))
	switch {
	case h.Score >= 85:
		h.Level, h.Label = HealthSmooth, "Running smoothly"
	case h.Score >= 65:
		h.Level, h.Label = HealthFair, "Slightly busy"
	case h.Score >= 40:
		h.Level, h.Label = HealthSlow, "Running slow"
	default:
		h.Level, h.Label = HealthStruggle, "Struggling"
	}
	if fs[lowest].Score < 85 {
		h.Bottleneck = fs[lowest].ID
	}
	sort.SliceStable(h.Factors, func(i, j int) bool { return h.Factors[i].Score < h.Factors[j].Score })
	return h
}
