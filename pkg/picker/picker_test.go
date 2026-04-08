package picker

import (
	"testing"
	"time"

	"github.com/containershell/containershell/pkg/runtime"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
		{48*time.Hour + 3*time.Hour, "2d3h"},
	}
	for _, tt := range tests {
		got := formatAge(tt.d)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"this is a very long string", 10, "this is..."},
		{"ab", 2, "ab"},
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestPick_SingleContainer(t *testing.T) {
	containers := []runtime.ContainerInfo{
		{ID: "abc123", Name: "nginx", PodName: "web-pod", Namespace: "default"},
	}
	result, err := Pick(containers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "abc123" {
		t.Errorf("expected container abc123, got %s", result.ID)
	}
}

func TestPick_NoContainers(t *testing.T) {
	_, err := Pick(nil)
	if err == nil {
		t.Fatal("expected error for empty container list")
	}
}
