package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared modern color palette for the dashboard TUI. Kept as 256-color codes so
// the look degrades gracefully on terminals without truecolor.
var (
	colAccent   = lipgloss.Color("99")  // primary — purple
	colAccent2  = lipgloss.Color("212") // secondary — magenta
	colText     = lipgloss.Color("252")
	colDim      = lipgloss.Color("244")
	colSubtle   = lipgloss.Color("240")
	colInset    = lipgloss.Color("236") // status/action bar background
	colOnAccent = lipgloss.Color("231") // text on an accent fill

	colRunning = lipgloss.Color("42")  // green
	colAmber   = lipgloss.Color("214") // amber
	colDanger  = lipgloss.Color("203") // red
	colInfo    = lipgloss.Color("39")  // blue
)

// stateColor maps a container state to its semantic color.
func stateColor(state string) lipgloss.Color {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running", "up":
		return colRunning
	case "paused":
		return colAmber
	case "exited", "stopped", "dead", "removing":
		return colDanger
	case "created", "configured", "restarting":
		return colInfo
	default:
		return colDim
	}
}

// statusDot renders a small ● colored by container state, for list rows.
func statusDot(state string) string {
	return lipgloss.NewStyle().Foreground(stateColor(state)).Render("●")
}
