package picker

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
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

// A rendered row must never be wider than the terminal, or it wraps onto a
// second line and desyncs the cursor from what's displayed.
func TestRowNeverExceedsWidth(t *testing.T) {
	long := strings.Repeat("x", 80)
	for width := 0; width <= 200; width++ {
		for _, hp := range []bool{false, true} {
			for _, hn := range []bool{false, true} {
				cols := computeColumns(width, hp, hn)
				row := renderRow(cols, "▸ ", long, long, long, long, long)
				line := clipLine(row, width)
				if w := ansi.StringWidth(line); width > 0 && w > width {
					t.Fatalf("width=%d hasPod=%v hasNs=%v: rendered width %d exceeds %d",
						width, hp, hn, w, width)
				}
			}
		}
	}
}

// Pod and namespace columns should be hidden for plain Docker/Podman lists and
// shown once containers populate them.
func TestColumnsHidePodNsWhenAbsent(t *testing.T) {
	cols := computeColumns(120, false, false)
	if cols.pod != 0 || cols.ns != 0 {
		t.Errorf("pod/ns should be hidden when absent, got pod=%d ns=%d", cols.pod, cols.ns)
	}
	if cols.name == 0 || cols.image == 0 || cols.age == 0 {
		t.Errorf("name/image/age must always be present, got %+v", cols)
	}

	cols = computeColumns(200, true, true)
	if cols.pod == 0 || cols.ns == 0 {
		t.Errorf("pod/ns should show when present and wide, got pod=%d ns=%d", cols.pod, cols.ns)
	}
}

// Scrolling must keep the cursor within the visible window for any list size.
func TestScrollKeepsCursorVisible(t *testing.T) {
	containers := make([]runtime.ContainerInfo, 100)
	for i := range containers {
		containers[i] = runtime.ContainerInfo{ID: string(rune('a' + i%26)), Name: "c"}
	}
	m := initialModel(containers)
	m.width, m.height = 80, 24

	// Walk the cursor all the way down, asserting visibility at each step.
	for step := 0; step < len(containers); step++ {
		visible := m.visibleRows()
		if m.cursor < m.offset || m.cursor >= m.offset+visible {
			t.Fatalf("cursor %d outside window [%d,%d)", m.cursor, m.offset, m.offset+visible)
		}
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.scrollIntoView()
		}
	}
	if m.cursor != len(containers)-1 {
		t.Fatalf("cursor should reach last item, got %d", m.cursor)
	}
}

// The View output must never contain a line wider than the terminal.
func TestViewFitsWidth(t *testing.T) {
	containers := []runtime.ContainerInfo{
		{ID: "deadbeef", Name: "very-long-container-name-that-overflows", Image: "registry.example.com/team/service:v1.2.3", PodName: "web-pod-xyz", Namespace: "production"},
		{ID: "cafebabe", Name: "db", Image: "postgres:16"},
	}
	for _, width := range []int{40, 60, 80, 120, 200} {
		m := initialModel(containers)
		m.width, m.height = width, 20
		for _, line := range strings.Split(m.View(), "\n") {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("width=%d: line %q has width %d", width, line, w)
			}
		}
	}
}
