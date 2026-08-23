package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestNewActionBarModel(t *testing.T) {
	m := NewActionBarModel()
	if len(m.shortcuts) == 0 {
		t.Fatal("expected default shortcuts to be set (ContextList)")
	}
	// Default context is ContextList which has 14 shortcuts
	if len(m.shortcuts) != 14 {
		t.Fatalf("expected 14 shortcuts for ContextList, got %d", len(m.shortcuts))
	}
}

func TestSetContext_List(t *testing.T) {
	m := NewActionBarModel()
	m.SetContext(ContextList)

	expected := []Shortcut{
		{Key: "↑↓", Label: "navigate"},
		{Key: "Enter/s", Label: "shell"},
		{Key: "l", Label: "logs"},
		{Key: "i", Label: "inspect"},
		{Key: "e", Label: "env"},
		{Key: "t", Label: "top"},
		{Key: "n", Label: "netstat"},
		{Key: "/", Label: "filter"},
		{Key: "S", Label: "sort"},
		{Key: "r", Label: "refresh"},
		{Key: "Tab", Label: "panel"},
		{Key: "T", Label: "theme"},
		{Key: "?", Label: "help"},
		{Key: "q", Label: "quit"},
	}

	if len(m.shortcuts) != len(expected) {
		t.Fatalf("expected %d shortcuts, got %d", len(expected), len(m.shortcuts))
	}
	for i, s := range m.shortcuts {
		if s.Key != expected[i].Key || s.Label != expected[i].Label {
			t.Errorf("shortcut[%d]: got %s:%s, want %s:%s", i, s.Key, s.Label, expected[i].Key, expected[i].Label)
		}
	}
}

func TestSetContext_Detail(t *testing.T) {
	m := NewActionBarModel()
	m.SetContext(ContextDetail)

	if len(m.shortcuts) != 7 {
		t.Fatalf("expected 7 shortcuts for ContextDetail, got %d", len(m.shortcuts))
	}
	// First shortcut should be scroll
	if m.shortcuts[0].Key != "↑↓" || m.shortcuts[0].Label != "scroll" {
		t.Errorf("expected first shortcut ↑↓:scroll, got %s:%s", m.shortcuts[0].Key, m.shortcuts[0].Label)
	}
}

func TestSetContext_Overlay(t *testing.T) {
	m := NewActionBarModel()
	m.SetContext(ContextOverlay)

	if len(m.shortcuts) != 4 {
		t.Fatalf("expected 4 shortcuts for ContextOverlay, got %d", len(m.shortcuts))
	}
	// Should have scroll, follow, Esc:close, q:close
	if m.shortcuts[1].Key != "f" || m.shortcuts[1].Label != "follow" {
		t.Errorf("expected second shortcut f:follow, got %s:%s", m.shortcuts[1].Key, m.shortcuts[1].Label)
	}
}

func TestSetContext_Help(t *testing.T) {
	m := NewActionBarModel()
	m.SetContext(ContextHelp)

	if len(m.shortcuts) != 2 {
		t.Fatalf("expected 2 shortcuts for ContextHelp, got %d", len(m.shortcuts))
	}
	if m.shortcuts[0].Key != "Esc" || m.shortcuts[0].Label != "close" {
		t.Errorf("expected first shortcut Esc:close, got %s:%s", m.shortcuts[0].Key, m.shortcuts[0].Label)
	}
	if m.shortcuts[1].Key != "?" || m.shortcuts[1].Label != "close" {
		t.Errorf("expected second shortcut ?:close, got %s:%s", m.shortcuts[1].Key, m.shortcuts[1].Label)
	}
}

func TestView_ContainsKeyLabelPairs(t *testing.T) {
	m := NewActionBarModel()
	m.SetContext(ContextHelp)
	m.SetDimensions(200) // wide enough to fit everything

	view := m.View()
	// The rendered view should contain the key and label text
	if !strings.Contains(view, "Esc") {
		t.Error("expected view to contain 'Esc'")
	}
	if !strings.Contains(view, "close") {
		t.Error("expected view to contain 'close'")
	}
}

func TestView_EmptyShortcuts(t *testing.T) {
	m := ActionBarModel{}
	view := m.View()
	if view != "" {
		t.Errorf("expected empty view for empty shortcuts, got %q", view)
	}
}

func TestView_Truncation(t *testing.T) {
	m := NewActionBarModel()
	m.SetContext(ContextList)
	m.SetDimensions(30) // Very narrow, should truncate

	view := m.View()
	visibleWidth := lipgloss.Width(view)
	if visibleWidth > 30 {
		t.Errorf("expected rendered width <= 30, got %d", visibleWidth)
	}
}

func TestView_NoTruncationWhenWide(t *testing.T) {
	m := NewActionBarModel()
	m.SetContext(ContextHelp) // Only 2 shortcuts, short
	m.SetDimensions(200)

	view := m.View()
	// Should not contain ellipsis when there's plenty of room
	if strings.Contains(view, "…") {
		t.Error("expected no truncation with wide width")
	}
}

func TestSetDimensions(t *testing.T) {
	m := NewActionBarModel()
	m.SetDimensions(120)
	if m.width != 120 {
		t.Errorf("expected width 120, got %d", m.width)
	}
}

func TestContextChangesShortcuts(t *testing.T) {
	m := NewActionBarModel()

	// Start with list context
	m.SetContext(ContextList)
	listCount := len(m.shortcuts)

	// Switch to detail
	m.SetContext(ContextDetail)
	detailCount := len(m.shortcuts)

	if listCount == detailCount {
		t.Error("expected different shortcut counts for different contexts")
	}

	// Switch to overlay
	m.SetContext(ContextOverlay)
	overlayCount := len(m.shortcuts)

	if detailCount == overlayCount {
		t.Error("expected different shortcut counts for detail vs overlay")
	}
}
