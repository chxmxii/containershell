package shell

import (
	"fmt"
	"testing"
)

func TestDefaultStrategies(t *testing.T) {
	strategies := DefaultStrategies()
	if len(strategies) != 3 {
		t.Fatalf("expected 3 strategies, got %d", len(strategies))
	}

	expected := []string{"CRI exec", "debug container injection", "nsenter"}
	for i, s := range strategies {
		if s.Name() != expected[i] {
			t.Errorf("strategy %d: expected %q, got %q", i, expected[i], s.Name())
		}
	}
}

func TestTierError(t *testing.T) {
	err := &TierError{Tier: 1, Strategy: "CRI exec", Err: fmt.Errorf("no shell found")}
	got := err.Error()
	want := "tier 1 (CRI exec): no shell found"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestShells(t *testing.T) {
	if len(Shells) == 0 {
		t.Fatal("Shells list should not be empty")
	}
	if Shells[0] != "/bin/bash" {
		t.Errorf("first shell should be /bin/bash, got %s", Shells[0])
	}
}
