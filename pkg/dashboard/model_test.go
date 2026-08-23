package dashboard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/containershell/containershell/pkg/runtime"
)

// Pressing 1/2/3 with the detail panel focused must dispatch a debug command;
// without one, the panel would sit in its loading state forever.
func TestDetailTabKeyDispatchesDebugCommand(t *testing.T) {
	m := NewModel(DefaultConfig(), nil, nil)
	m.focus = FocusDetail
	c := runtime.ContainerInfo{ID: "abc123", Name: "web", State: "running"}
	m.detail.SetContainer(&c)

	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if cmd == nil {
		t.Fatal("expected key '1' to dispatch a debug command")
	}
	dm := updated.(Model)
	if !dm.detail.loading || dm.detail.view != ViewEnv {
		t.Fatalf("expected env view in loading state, got view=%d loading=%v",
			dm.detail.view, dm.detail.loading)
	}
}

// A debug result tagged for the detail panel must land there, and a stale
// result (tab changed while the command ran) must be dropped.
func TestDetailDebugOutputRouting(t *testing.T) {
	m := NewModel(DefaultConfig(), nil, nil)
	c := runtime.ContainerInfo{ID: "abc123", Name: "web", State: "running"}
	m.detail.SetContainer(&c)
	m.detail.view = ViewEnv
	m.detail.loading = true

	m = m.handleDebugOutput(debugOutputMsg{content: "PATH=/bin", forDetail: true, view: ViewEnv})
	if m.detail.loading || m.detail.content != "PATH=/bin" {
		t.Fatalf("expected env output applied, got loading=%v content=%q",
			m.detail.loading, m.detail.content)
	}

	// Stale response for a different tab must not overwrite the current one.
	m = m.handleDebugOutput(debugOutputMsg{content: "stale", forDetail: true, view: ViewTop})
	if m.detail.content != "PATH=/bin" {
		t.Fatalf("stale response overwrote content: %q", m.detail.content)
	}
}

// Rendering the overlay must be idempotent: a previous bug mutated the stored
// content lines on every render, walking the text out of the box.
func TestOverlayViewDoesNotMutateContent(t *testing.T) {
	o := NewOverlayModel("Inspect: web", 100, 30)
	o.SetContent("hello\nworld")

	v1 := o.View()
	v2 := o.View()
	if v1 != v2 {
		t.Fatal("overlay render is not idempotent")
	}
	if o.lines[0] != "hello" {
		t.Fatalf("View mutated stored content: %q", o.lines[0])
	}
}
