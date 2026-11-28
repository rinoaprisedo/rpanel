package panelssl

import (
	"context"
	"errors"
	"testing"
	"time"

	"rpanel/backend/system"
)

type ufwRunner struct {
	status string
	calls  [][]string
}

func (r *ufwRunner) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if name != "ufw" {
		return system.Result{}, errors.New("unexpected")
	}
	if args[0] == "status" {
		return system.Result{Stdout: r.status}, nil
	}
	return system.Result{}, nil
}

func TestOpenWebPorts(t *testing.T) {
	r := &ufwRunner{status: "Status: active\n"}
	if note := OpenWebPorts(context.Background(), r); note == "" {
		t.Fatal("expected a note")
	}
	if len(r.calls) != 3 || r.calls[1][2] != "80/tcp" || r.calls[2][2] != "443/tcp" {
		t.Fatalf("calls = %v", r.calls)
	}
	r = &ufwRunner{status: "Status: inactive\n"}
	if note := OpenWebPorts(context.Background(), r); note != "" || len(r.calls) != 1 {
		t.Fatalf("inactive firewall must be left alone: %q %v", note, r.calls)
	}
}
