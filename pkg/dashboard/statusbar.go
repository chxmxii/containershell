package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/runtime"
	"github.com/containershell/containershell/pkg/tui"
)

// StatusBarModel holds state for the top status bar row.
type StatusBarModel struct {
	runtimeName    string
	runtimeVersion string
	socketPath     string
	containerCount int
	lastRefresh    time.Time
	connected      bool
	connecting     bool
	width          int
}

// NewStatusBarModel creates a new StatusBarModel in the connecting state.
func NewStatusBarModel() StatusBarModel {
	return StatusBarModel{
		connecting: true,
	}
}

// SetRuntimeInfo updates the status bar with runtime connection details.
func (m *StatusBarModel) SetRuntimeInfo(info *runtime.RuntimeInfo) {
	if info == nil {
		return
	}
	m.runtimeName = info.Name
	m.runtimeVersion = info.Version
	m.socketPath = info.SocketPath
	m.connected = true
	m.connecting = false
}

// SetContainerCount updates the displayed container count.
func (m *StatusBarModel) SetContainerCount(count int) {
	m.containerCount = count
}

// SetLastRefresh updates the last refresh timestamp.
func (m *StatusBarModel) SetLastRefresh(t time.Time) {
	m.lastRefresh = t
}

// SetDisconnected marks the runtime as disconnected.
func (m *StatusBarModel) SetDisconnected() {
	m.connected = false
	m.connecting = false
}

// SetDimensions sets the available width for rendering.
func (m *StatusBarModel) SetDimensions(width int) {
	m.width = width
}

// TruncateSocketPath truncates a socket path for display.
// If len(s) <= 40, it returns s unchanged.
// If len(s) > 40, it returns "…" + the last 40 characters.
func TruncateSocketPath(s string) string {
	if len(s) <= 40 {
		return s
	}
	return "…" + s[len(s)-40:]
}

// View renders the status bar as a single row of segments: a product badge,
// the runtime, a colored connection indicator, the socket path, and the last
// refresh time right-aligned.
func (m StatusBarModel) View() string {
	sep := tui.DimStyle.Render(" │ ")
	badge := tui.BadgeStyle.Render(" " + tui.Logo + " containershell ")

	var segments []string
	switch {
	case m.connecting:
		segments = append(segments,
			lipgloss.NewStyle().Foreground(tui.ColorYellow).Render("● connecting…"))
	case !m.connected:
		segments = append(segments,
			lipgloss.NewStyle().Foreground(tui.ColorRed).Render("● disconnected"),
			tui.DimStyle.Render(fmt.Sprintf("%d containers (stale)", m.containerCount)))
	default:
		segments = append(segments,
			lipgloss.NewStyle().Bold(true).Foreground(tui.ColorBlue).Render(
				fmt.Sprintf("%s v%s", m.runtimeName, m.runtimeVersion)),
			lipgloss.NewStyle().Foreground(tui.ColorGreen).Render(
				fmt.Sprintf("● %d containers", m.containerCount)),
			tui.DimStyle.Render(TruncateSocketPath(m.socketPath)))
	}

	left := badge + " " + strings.Join(segments, sep)
	right := tui.DimStyle.Render("⟳ " + m.formatRefreshTime())

	// Right-align the refresh time; drop it if the bar is too narrow.
	if m.width > 0 {
		gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap >= 1 {
			return left + strings.Repeat(" ", gap) + right
		}
		return tui.ClipLine(left, m.width)
	}
	return left + " " + right
}

// formatRefreshTime formats the last refresh time as HH:MM:SS.
func (m StatusBarModel) formatRefreshTime() string {
	if m.lastRefresh.IsZero() {
		return "--:--:--"
	}
	return m.lastRefresh.Format("15:04:05")
}
