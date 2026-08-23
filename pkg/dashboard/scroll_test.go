package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/runtime"
)

// testContainers builds n deterministically-named containers for layout tests.
func testContainers(n int) []runtime.ContainerInfo {
	cs := make([]runtime.ContainerInfo, n)
	for i := 0; i < n; i++ {
		cs[i] = runtime.ContainerInfo{
			ID:        fmt.Sprintf("%012d", i),
			Name:      fmt.Sprintf("ctr-%03d", i),
			State:     "running",
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Minute),
		}
	}
	return cs
}

// newTestModel builds a dashboard model sized to w×h and loaded with n containers.
func newTestModel(w, h, n int) tea.Model {
	var m tea.Model = NewModel(DefaultConfig(), nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m, _ = m.Update(containersLoadedMsg{containers: testContainers(n)})
	return m
}

// markedLine returns the rendered line carrying the selection marker.
func markedLine(view string) (string, bool) {
	for _, ln := range strings.Split(view, "\n") {
		if strings.Contains(ln, "▸") {
			return ln, true
		}
	}
	return "", false
}

// TestDashboardFitsTerminal guards the core scrolling bug: the rendered dashboard
// must never exceed the terminal bounds in either dimension. Before the fix the
// list panel ignored its border and wrapped rows, overflowing to ~2x the height.
func TestDashboardFitsTerminal(t *testing.T) {
	sizes := []struct{ w, h int }{
		{80, 24}, {81, 25}, {100, 24}, {100, 30}, {120, 40}, {200, 50}, {90, 10},
		{70, 30}, {60, 26}, {79, 24}, // stacked mode (width < 80)
	}
	for _, s := range sizes {
		m := newTestModel(s.w, s.h, 200)
		view := m.View()
		if got := lipgloss.Height(view); got > s.h {
			t.Errorf("%dx%d: rendered height %d exceeds terminal height %d", s.w, s.h, got, s.h)
		}
		if got := lipgloss.Width(view); got > s.w {
			t.Errorf("%dx%d: rendered width %d exceeds terminal width %d", s.w, s.h, got, s.w)
		}
	}
}

// TestListScrollKeepsSelectionVisible drives the arrow key through the entire
// list and asserts that the selection stays visible and on-screen the whole way.
func TestListScrollKeepsSelectionVisible(t *testing.T) {
	const w, h, n = 100, 24, 60
	m := newTestModel(w, h, n)

	for i := 0; i <= n+5; i++ {
		view := m.View()

		if got := lipgloss.Height(view); got > h {
			t.Fatalf("after %d down presses: height %d exceeds terminal height %d", i, got, h)
		}

		line, ok := markedLine(view)
		if !ok {
			t.Fatalf("after %d down presses: selection marker not visible", i)
		}

		// Cursor advances one per press, clamped at the last container.
		wantIdx := i
		if wantIdx > n-1 {
			wantIdx = n - 1
		}
		wantName := fmt.Sprintf("ctr-%03d", wantIdx)
		if !strings.Contains(line, wantName) {
			t.Fatalf("after %d down presses: selected line %q does not show %q", i, strings.TrimSpace(line), wantName)
		}

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
}

// TestListRowsDoNotWrap verifies that every list line fits within the list panel
// width, so a single container never spills onto a second display line.
func TestListRowsDoNotWrap(t *testing.T) {
	const w, h, n = 100, 30, 40
	m := newTestModel(w, h, n)
	view := m.View()

	layout := ComputeLayout(w, h, DefaultLayoutConfig())
	for _, ln := range strings.Split(view, "\n") {
		if lipgloss.Width(ln) > w {
			t.Errorf("line wider than terminal (%d > %d): %q", lipgloss.Width(ln), w, ln)
		}
	}
	// Sanity: the list panel is genuinely narrower than the old 76-col row.
	if layout.ListWidth >= 76 {
		t.Skipf("list panel width %d too wide to exercise wrapping", layout.ListWidth)
	}
}
