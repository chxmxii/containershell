package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/tui"
)

// ActionContext represents the current UI context determining which shortcuts to display.
type ActionContext int

const (
	// ContextList is the context when the container list panel has focus.
	ContextList ActionContext = iota
	// ContextDetail is the context when the detail panel has focus.
	ContextDetail
	// ContextOverlay is the context when a debug/log overlay is open.
	ContextOverlay
	// ContextHelp is the context when the help overlay is open.
	ContextHelp
)

// Shortcut represents a single key-label pair displayed in the action bar.
type Shortcut struct {
	Key   string
	Label string
}

// ActionBarModel manages the contextual shortcut display at the bottom of the dashboard.
type ActionBarModel struct {
	shortcuts []Shortcut
	width     int
}

// NewActionBarModel creates a new ActionBarModel with the default (list) context.
func NewActionBarModel() ActionBarModel {
	m := ActionBarModel{}
	m.SetContext(ContextList)
	return m
}

// SetContext updates the displayed shortcuts based on the current UI context.
func (m *ActionBarModel) SetContext(ctx ActionContext) {
	switch ctx {
	case ContextList:
		m.shortcuts = []Shortcut{
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
			{Key: "?", Label: "help"},
			{Key: "q", Label: "quit"},
		}
	case ContextDetail:
		m.shortcuts = []Shortcut{
			{Key: "↑↓", Label: "scroll"},
			{Key: "1", Label: "env"},
			{Key: "2", Label: "top"},
			{Key: "3", Label: "netstat"},
			{Key: "Tab", Label: "panel"},
			{Key: "?", Label: "help"},
			{Key: "q", Label: "quit"},
		}
	case ContextOverlay:
		m.shortcuts = []Shortcut{
			{Key: "↑↓", Label: "scroll"},
			{Key: "f", Label: "follow"},
			{Key: "Esc", Label: "close"},
			{Key: "q", Label: "close"},
		}
	case ContextHelp:
		m.shortcuts = []Shortcut{
			{Key: "Esc", Label: "close"},
			{Key: "?", Label: "close"},
		}
	}
}

// SetDimensions updates the available width for rendering.
func (m *ActionBarModel) SetDimensions(width int) {
	m.width = width
}

// View renders the action bar as a single row of key/label entries.
func (m ActionBarModel) View() string {
	if len(m.shortcuts) == 0 {
		return ""
	}

	var entries []string
	for _, s := range m.shortcuts {
		entry := tui.KeyStyle.Render(s.Key) + " " + tui.DimStyle.Render(s.Label)
		entries = append(entries, entry)
	}

	rendered := strings.Join(entries, "  ")

	// Truncate if the visible width exceeds the available width.
	// We measure the visible (printable) width to decide truncation.
	if m.width > 0 && lipgloss.Width(rendered) > m.width {
		rendered = truncateToWidth(entries, m.width)
	}

	return rendered
}

// truncateToWidth joins entries with double-space separators, stopping when the
// next entry would exceed the available width. Adds "…" if truncated.
func truncateToWidth(entries []string, maxWidth int) string {
	if len(entries) == 0 {
		return ""
	}

	separator := "  "
	ellipsis := "…"
	ellipsisWidth := lipgloss.Width(ellipsis)

	var result strings.Builder
	currentWidth := 0

	for i, entry := range entries {
		entryWidth := lipgloss.Width(entry)
		sepWidth := 0
		if i > 0 {
			sepWidth = lipgloss.Width(separator)
		}

		needed := sepWidth + entryWidth
		// Check if adding this entry would exceed max width (leaving room for ellipsis if not last)
		if currentWidth+needed > maxWidth {
			// Check if we can fit the ellipsis
			if currentWidth+sepWidth+ellipsisWidth <= maxWidth {
				if i > 0 {
					result.WriteString(separator)
				}
				result.WriteString(ellipsis)
			}
			break
		}

		if i > 0 {
			result.WriteString(separator)
		}
		result.WriteString(entry)
		currentWidth += needed
	}

	return result.String()
}
