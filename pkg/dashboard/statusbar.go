package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/runtime"
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

// Status bar segment styles: an accent app badge on an otherwise dark inset bar.
var (
	sbBadge   = lipgloss.NewStyle().Bold(true).Foreground(colOnAccent).Background(colAccent)
	sbSeg     = lipgloss.NewStyle().Foreground(colText).Background(colInset)
	sbRuntime = lipgloss.NewStyle().Bold(true).Foreground(colAccent2).Background(colInset)
	sbDim     = lipgloss.NewStyle().Foreground(colDim).Background(colInset)
	sbDanger  = lipgloss.NewStyle().Bold(true).Foreground(colDanger).Background(colInset)
)

// View renders the status bar as a single full-width row: an accent badge, the
// runtime and container count, and a right-aligned refresh clock.
func (m StatusBarModel) View() string {
	segs := []string{sbBadge.Render(" ◆ containershell ")}

	switch {
	case m.connecting:
		segs = append(segs, sbDim.Render(" connecting… "))
	case !m.connected:
		segs = append(segs,
			sbDanger.Render(" × disconnected "),
			sbSeg.Render(fmt.Sprintf(" %d containers (stale) ", m.containerCount)),
		)
	default:
		dot := lipgloss.NewStyle().Foreground(colRunning).Background(colInset).Render("●")
		segs = append(segs,
			sbSeg.Render(" ")+dot+sbRuntime.Render(fmt.Sprintf(" %s v%s ", m.runtimeName, m.runtimeVersion)),
			sbSeg.Render(fmt.Sprintf(" %d containers ", m.containerCount)),
			sbDim.Render(" "+TruncateSocketPath(m.socketPath)+" "),
		)
	}

	left := lipgloss.JoinHorizontal(lipgloss.Top, segs...)
	right := sbDim.Render(" ⟳ " + m.formatRefreshTime() + " ")

	if m.width <= 0 {
		return left
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		// Too narrow for the clock; clip the left segments to the width.
		return clipLine(left, m.width)
	}
	filler := sbSeg.Render(strings.Repeat(" ", gap))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, filler, right)
}

// formatRefreshTime formats the last refresh time as HH:MM:SS.
func (m StatusBarModel) formatRefreshTime() string {
	if m.lastRefresh.IsZero() {
		return "--:--:--"
	}
	return m.lastRefresh.Format("15:04:05")
}
