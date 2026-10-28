package dashboard

import (
	"context"
	"os"
	"path/filepath"
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

func TestReadTCPStatesAndHelpers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tcp")
	os.WriteFile(p, []byte(`  sl  local_address rem_address   st tx_queue
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000
   1: 0100007F:1F91 0100007F:C000 01 00000000:00000000
   2: 0100007F:1F92 0100007F:C001 01 00000000:00000000
   3: 0100007F:1F93 0100007F:C002 06 00000000:00000000
   4: 0100007F:1F94 0100007F:C003 08 00000000:00000000
   5: 0100007F:1F95 0100007F:C004 03 00000000:00000000
   6: 0100007F:1F96 0100007F:C005 04 00000000:00000000
`), 0o644)
	got := readTCPStates(p, filepath.Join(dir, "missing"))
	want := TCPStates{Established: 2, Listen: 1, TimeWait: 1, CloseWait: 1, SynRecv: 1, Other: 1, Total: 7}
	if got != want {
		t.Errorf("tcp states = %+v, want %+v", got, want)
	}
	for name, ok := range map[string]bool{"sda": true, "vdb": true, "nvme0n1": true, "mmcblk0": true, "sda1": false, "nvme0n1p1": false, "loop3": false, "dm-0": false, "ram0": false} {
		if physicalDisk(name) != ok {
			t.Errorf("physicalDisk(%q) = %v, want %v", name, !ok, ok)
		}
	}
	if !skipIface("lo") || !skipIface("veth1a2b3c") || skipIface("eth0") || skipIface("ens3") || skipIface("docker0") {
		t.Error("interface filter wrong")
	}
}
