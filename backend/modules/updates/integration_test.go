//go:build integration

package updates

import (
	"context"
	"testing"
	"time"

	"rpanel/backend/system"
)

// Runs against real apt (see the docker command in the commit notes).
func TestIntegrationReal(t *testing.T) {
	dir := t.TempDir()
	s := NewService(system.Default, Paths{RebootFile: dir + "/reboot-required", AutoConf: dir + "/52rpanel"})
	st := s.Status(context.Background())
	t.Logf("before: known=%v total=%d security=%d", st.Known, st.Total, st.Security)
	if _, err := s.Start(KindAll); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1500; i++ {
		p := s.Progress()
		if p.State != "running" {
			t.Logf("job: state=%s percent=%d installed=%d/%d err=%q elapsed=%ds", p.State, p.Percent, p.Done, p.Total, p.Error, p.Elapsed)
			for _, st := range p.Steps {
				t.Logf("  step %s: %s %s", st.Key, st.Status, st.Detail)
			}
			if n := len(p.Log); n > 0 {
				t.Logf("log tail: %v", p.Log[max(0, n-6):])
			}
			if p.State != "done" {
				t.Fatalf("failed: %s", p.Error)
			}
			after := s.Status(context.Background())
			t.Logf("after: total=%d security=%d", after.Total, after.Security)
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("timeout")
}
