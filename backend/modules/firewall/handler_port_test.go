package firewall

import (
	"testing"

	"rpanel/backend/config"
)

func TestPublicPanelPortPrefersNginxPort(t *testing.T) {
	if got := publicPanelPort(&config.Config{AppPort: "8800", PanelPort: "43210"}); got != "43210" {
		t.Errorf("production: want the nginx port 43210, got %s", got)
	}
	if got := publicPanelPort(&config.Config{AppPort: "8800"}); got != "8800" {
		t.Errorf("dev without nginx: want the backend port 8800, got %s", got)
	}
}

func TestPanelRuleCannotBeDeleted(t *testing.T) {
	s := &Service{panelPort: 40750}
	for _, c := range []struct {
		r    Rule
		want bool
	}{
		{Rule{To: "40750/tcp", Action: "ALLOW", Direction: "IN"}, true},
		{Rule{To: "40750/tcp (v6)", Action: "ALLOW", Direction: "IN"}, true},
		{Rule{To: "40000:41000/tcp", Action: "ALLOW", Direction: "IN"}, true},
		{Rule{To: "40750/tcp", Action: "DENY", Direction: "IN"}, false},
		{Rule{To: "443/tcp", Action: "ALLOW", Direction: "IN"}, false},
	} {
		if got := s.isPanelRule(c.r); got != c.want {
			t.Errorf("%+v: locked=%v, want %v", c.r, got, c.want)
		}
	}
}
