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

// View renders the status bar as a single row.
func (m StatusBarModel) View() string {
	style := lipgloss.NewStyle().
		Bold(true).
		Reverse(true)

	var content string

	switch {
	case m.connecting:
		content = "[Connecting...]"
	case !m.connected:
		refreshStr := m.formatRefreshTime()
		content = fmt.Sprintf("[Disconnected] ⚠ | %d containers (stale) | Last: %s",
			m.containerCount, refreshStr)
	default:
		socket := TruncateSocketPath(m.socketPath)
		refreshStr := m.formatRefreshTime()
		content = fmt.Sprintf("[%s v%s] %s | %d containers | Last: %s",
			m.runtimeName, m.runtimeVersion, socket,
			m.containerCount, refreshStr)
	}

	// Pad content to fill the full width
	if m.width > 0 && len(content) < m.width {
		content = content + strings.Repeat(" ", m.width-len(content))
	}

	return style.Render(content)
}

// formatRefreshTime formats the last refresh time as HH:MM:SS.
func (m StatusBarModel) formatRefreshTime() string {
	if m.lastRefresh.IsZero() {
		return "--:--:--"
	}
	return m.lastRefresh.Format("15:04:05")
}
