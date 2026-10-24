package dashboard

import "testing"

func healthy() insightInput {
	return insightInput{
		Overview: Overview{CPU: 20, CPUCores: 4, Load5: 0.5, MemPct: 40, Disks: []DiskInfo{{Mount: "/", Percent: 30}}},
		Services: []ServiceInfo{{ID: "nginx", Running: true}},
	}
}

func TestHealthSmoothWhenIdle(t *testing.T) {
	h := buildHealth(healthy())
	if h.Level != HealthSmooth || h.Score < 95 || h.Bottleneck != "" {
		t.Errorf("idle server: %+v", h)
	}
}

func TestHealthUsesSustainedCPUNotOneSpike(t *testing.T) {
	in := healthy()
	in.Overview.CPU = 100 // instantaneous spike
	in.Recent = []Point{{CPU: 10}, {CPU: 12}, {CPU: 14}, {CPU: 100}}
	if h := buildHealth(in); h.Level != HealthSmooth {
		t.Errorf("a single spike should not slow the verdict: %+v", h)
	}
	in.Recent = []Point{{CPU: 98}, {CPU: 99}, {CPU: 100}, {CPU: 100}}
	h := buildHealth(in)
	if h.Bottleneck != "cpu" || h.Level == HealthSmooth {
		t.Errorf("sustained saturation must show: %+v", h)
	}
}

func TestHealthOneFailingFactorCapsScore(t *testing.T) {
	in := healthy()
	in.Overview.Disks[0].Percent = 99
	h := buildHealth(in)
	if h.Score > 60 || h.Bottleneck != "disk" {
		t.Errorf("full disk should cap the score: %+v", h)
	}
}

func TestHealthSwapAndServices(t *testing.T) {
	in := healthy()
	in.Overview.MemPct = 60
	in.Overview.SwapTotal, in.Overview.SwapUsed = 1<<30, 1<<30
	in.Services = []ServiceInfo{{Running: false}, {Running: false}}
	h := buildHealth(in)
	if h.Level == HealthSmooth || len(h.Factors) != 5 {
		t.Errorf("heavy swap + stopped services: %+v", h)
	}
	if h.Factors[0].Score > h.Factors[1].Score {
		t.Error("factors must be sorted worst first")
	}
}

func TestHealthSkipsFactorsWithoutData(t *testing.T) {
	in := healthy()
	in.Services = nil
	in.Overview.Disks = nil
	for _, f := range buildHealth(in).Factors {
		if f.ID == "services" || f.ID == "disk" || f.ID == "errors" {
			t.Errorf("factor %s should be absent", f.ID)
		}
	}
}
