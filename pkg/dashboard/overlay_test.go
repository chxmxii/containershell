package dashboard

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewOverlayModel(t *testing.T) {
	o := NewOverlayModel("Test Title", 120, 40)

	if o.title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", o.title)
	}
	if o.width != 120 {
		t.Errorf("expected width 120, got %d", o.width)
	}
	if o.height != 40 {
		t.Errorf("expected height 40, got %d", o.height)
	}
	if o.scroll != 0 {
		t.Errorf("expected scroll 0, got %d", o.scroll)
	}
	if o.follow {
		t.Error("expected follow to be false")
	}
	if o.loading {
		t.Error("expected loading to be false")
	}
	if o.err != nil {
		t.Error("expected err to be nil")
	}
}

func TestSetContent(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetContent("line1\nline2\nline3")

	if len(o.lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(o.lines))
	}
	if o.lines[0] != "line1" {
		t.Errorf("expected first line 'line1', got %q", o.lines[0])
	}
	if o.scroll != 0 {
		t.Errorf("expected scroll reset to 0, got %d", o.scroll)
	}
}

func TestSetContent_Empty(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetContent("")

	if o.lines != nil {
		t.Errorf("expected nil lines for empty content, got %v", o.lines)
	}
}

func TestSetLoading(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetLoading(true)
	if !o.loading {
		t.Error("expected loading to be true")
	}
	o.SetLoading(false)
	if o.loading {
		t.Error("expected loading to be false")
	}
}

func TestSetError(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	testErr := errors.New("test error")
	o.SetError(testErr)
	if o.err != testErr {
		t.Errorf("expected error %v, got %v", testErr, o.err)
	}
}

func TestOverlay_SetDimensions(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	// Set content with many lines and scroll to bottom
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	o.scrollToBottom()

	// Resize to larger - scroll should still be clamped
	o.SetDimensions(200, 80)
	if o.width != 200 {
		t.Errorf("expected width 200, got %d", o.width)
	}
	if o.height != 80 {
		t.Errorf("expected height 80, got %d", o.height)
	}
	if o.scroll > o.maxScroll() {
		t.Errorf("scroll %d exceeds max %d after resize", o.scroll, o.maxScroll())
	}
}

func TestToggleFollow(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))

	o.ToggleFollow()
	if !o.follow {
		t.Error("expected follow to be true after first toggle")
	}
	if o.scroll != o.maxScroll() {
		t.Errorf("expected scroll at bottom (%d) when follow enabled, got %d", o.maxScroll(), o.scroll)
	}

	o.ToggleFollow()
	if o.follow {
		t.Error("expected follow to be false after second toggle")
	}
}

func TestAppendLine(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetContent("line1")
	o.follow = true

	o.AppendLine("line2")
	if len(o.lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(o.lines))
	}
	if o.lines[1] != "line2" {
		t.Errorf("expected second line 'line2', got %q", o.lines[1])
	}
}

func TestAppendLine_FollowScrollsToBottom(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	o.follow = true

	o.AppendLine("new line")
	if o.scroll != o.maxScroll() {
		t.Errorf("expected scroll at bottom when follow=true, got scroll=%d max=%d", o.scroll, o.maxScroll())
	}
}

func TestUpdate_EscCloses(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetContent("some content")

	_, _, shouldClose := o.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if !shouldClose {
		t.Error("expected Esc to close the overlay")
	}
}

func TestUpdate_QCloses(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetContent("some content")

	_, _, shouldClose := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !shouldClose {
		t.Error("expected q to close the overlay")
	}
}

func TestUpdate_ScrollDown(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))

	o.Update(tea.KeyMsg{Type: tea.KeyDown})
	if o.scroll != 1 {
		t.Errorf("expected scroll=1 after down, got %d", o.scroll)
	}
}

func TestUpdate_ScrollUp(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	o.scroll = 5

	o.Update(tea.KeyMsg{Type: tea.KeyUp})
	if o.scroll != 4 {
		t.Errorf("expected scroll=4 after up, got %d", o.scroll)
	}
}

func TestUpdate_ScrollUpClampsAtZero(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetContent("line1\nline2")

	o.Update(tea.KeyMsg{Type: tea.KeyUp})
	if o.scroll != 0 {
		t.Errorf("expected scroll=0 (clamped), got %d", o.scroll)
	}
}

func TestUpdate_ScrollDownClampsAtMax(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 5)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	// With only 5 lines and large viewport, max scroll is 0
	o.Update(tea.KeyMsg{Type: tea.KeyDown})
	if o.scroll != 0 {
		t.Errorf("expected scroll=0 when content fits viewport, got %d", o.scroll)
	}
}

func TestUpdate_JK_Navigation(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))

	o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if o.scroll != 1 {
		t.Errorf("expected scroll=1 after j, got %d", o.scroll)
	}

	o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if o.scroll != 0 {
		t.Errorf("expected scroll=0 after k, got %d", o.scroll)
	}
}

func TestUpdate_G_ScrollToTop(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	o.scroll = 50

	o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if o.scroll != 0 {
		t.Errorf("expected scroll=0 after g, got %d", o.scroll)
	}
}

func TestUpdate_ShiftG_ScrollToBottom(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))

	o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if o.scroll != o.maxScroll() {
		t.Errorf("expected scroll=%d after G, got %d", o.maxScroll(), o.scroll)
	}
}

func TestUpdate_PgUp(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	o.scrollToBottom()
	prevScroll := o.scroll

	o.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	expected := prevScroll - o.viewportHeight()
	if expected < 0 {
		expected = 0
	}
	if o.scroll != expected {
		t.Errorf("expected scroll=%d after PgUp, got %d", expected, o.scroll)
	}
}

func TestUpdate_PgDn(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))

	o.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	expected := o.viewportHeight()
	if expected > o.maxScroll() {
		expected = o.maxScroll()
	}
	if o.scroll != expected {
		t.Errorf("expected scroll=%d after PgDn, got %d", expected, o.scroll)
	}
}

func TestUpdate_F_ToggleFollow(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))

	o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !o.follow {
		t.Error("expected follow=true after f")
	}

	o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if o.follow {
		t.Error("expected follow=false after second f")
	}
}

func TestUpdate_ScrollUpDisablesFollow(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	o.follow = true
	o.scrollToBottom()

	o.Update(tea.KeyMsg{Type: tea.KeyUp})
	if o.follow {
		t.Error("expected follow=false after scrolling up")
	}
}

func TestView_ContainsTitle(t *testing.T) {
	o := NewOverlayModel("My Title", 100, 40)
	o.SetContent("hello world")

	view := o.View()
	if !strings.Contains(view, "My Title") {
		t.Error("expected view to contain the title")
	}
}

func TestView_ContainsFollowIndicator(t *testing.T) {
	o := NewOverlayModel("Logs", 100, 40)
	o.SetContent("log line")
	o.follow = true

	view := o.View()
	if !strings.Contains(view, "FOLLOW") {
		t.Error("expected view to contain FOLLOW indicator when follow=true")
	}
}

func TestView_NoFollowIndicatorWhenDisabled(t *testing.T) {
	o := NewOverlayModel("Logs", 100, 40)
	o.SetContent("log line")
	o.follow = false

	view := o.View()
	if strings.Contains(view, "FOLLOW") {
		t.Error("expected view to NOT contain FOLLOW indicator when follow=false")
	}
}

func TestView_ShowsError(t *testing.T) {
	o := NewOverlayModel("Error Test", 100, 40)
	o.SetError(errors.New("something went wrong"))

	view := o.View()
	if !strings.Contains(view, "something went wrong") {
		t.Error("expected view to contain error message")
	}
}

func TestView_ShowsLoadingWhenNoContent(t *testing.T) {
	o := NewOverlayModel("Loading Test", 100, 40)
	o.SetLoading(true)

	view := o.View()
	if !strings.Contains(view, "Loading") {
		t.Error("expected view to contain loading indicator")
	}
}

func TestScrollClamping_NeverNegative(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	o.SetContent("single line")
	o.scroll = -10
	o.clampScroll()

	if o.scroll < 0 {
		t.Errorf("scroll should never be negative, got %d", o.scroll)
	}
}

func TestScrollClamping_NeverExceedsMax(t *testing.T) {
	o := NewOverlayModel("Test", 100, 40)
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	o.SetContent(strings.Join(lines, "\n"))
	o.scroll = 9999
	o.clampScroll()

	if o.scroll > o.maxScroll() {
		t.Errorf("scroll %d exceeds max %d", o.scroll, o.maxScroll())
	}
}
