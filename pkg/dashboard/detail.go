package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/runtime"
	"github.com/containershell/containershell/pkg/tui"
)

// DetailView represents which view is active in the detail panel.
type DetailView int

const (
	ViewInfo    DetailView = iota
	ViewEnv
	ViewTop
	ViewNetstat
)

// Styles for the detail panel.
var (
	detailKeyStyle       = tui.KeyStyle
	detailValueStyle     = lipgloss.NewStyle()
	detailDimStyle       = tui.DimStyle
	detailErrStyle       = tui.ErrStyle
	detailTabActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorOnAccent).Background(tui.ColorAccent)
	detailTabStyle       = tui.DimStyle
)

// DetailModel is the detail panel sub-model that displays container info and debug command output.
type DetailModel struct {
	container *runtime.ContainerInfo
	view      DetailView
	content   string
	loading   bool
	err       error
	scroll    int
	height    int
	width     int
}

// NewDetailModel creates a new DetailModel with default state.
func NewDetailModel() DetailModel {
	return DetailModel{
		view: ViewInfo,
	}
}

// SetContainer updates the displayed container and resets to the info view.
func (m *DetailModel) SetContainer(c *runtime.ContainerInfo) {
	m.container = c
	m.view = ViewInfo
	m.content = ""
	m.loading = false
	m.err = nil
	m.scroll = 0
}

// SetDimensions updates the panel dimensions.
func (m *DetailModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
	// Clamp scroll after resize
	m.clampScroll()
}

// Update handles messages for the detail panel.
func (m DetailModel) Update(msg tea.Msg) (DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		keyStr := msg.String()
		switch keyStr {
		case "1":
			m.view = ViewEnv
			m.content = ""
			m.loading = true
			m.err = nil
			m.scroll = 0
		case "2":
			m.view = ViewTop
			m.content = ""
			m.loading = true
			m.err = nil
			m.scroll = 0
		case "3":
			m.view = ViewNetstat
			m.content = ""
			m.loading = true
			m.err = nil
			m.scroll = 0
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
			m.clampScroll()
		}

	case debugOutputMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.content = ""
		} else {
			m.err = nil
			m.content = msg.content
		}
		m.scroll = 0
	}

	return m, nil
}

// View renders the detail panel.
func (m DetailModel) View() string {
	var b strings.Builder

	// Tab bar: the active view is highlighted; 1/2/3 switch tabs.
	b.WriteString(clipLine(m.renderTabs(), m.width))
	b.WriteString("\n")

	// No container selected
	if m.container == nil {
		b.WriteString(detailDimStyle.Render("  ◇ select a container from the list"))
		b.WriteString("\n")
		return b.String()
	}

	// Loading state
	if m.loading {
		b.WriteString(detailDimStyle.Render("  … loading"))
		b.WriteString("\n")
		return b.String()
	}

	// Error state
	if m.err != nil {
		cmdName := m.viewCommandName()
		errLine := fmt.Sprintf("  ✗ %s: %s", cmdName, m.err.Error())
		b.WriteString(detailErrStyle.Render(clipLine(errLine, m.width)))
		b.WriteString("\n")
		return b.String()
	}

	// Render content based on view
	var content string
	if m.view == ViewInfo {
		content = m.renderInfoView()
	} else {
		content = m.content
	}

	// Apply scroll to content
	lines := strings.Split(content, "\n")
	viewportHeight := m.viewportHeight()

	// Clamp scroll within content bounds
	maxScroll := len(lines) - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}

	// Render visible lines
	start := m.scroll
	end := start + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}

	for i := start; i < end; i++ {
		// Clip each line to the panel width so long lines (e.g. wide top/netstat
		// output) cannot wrap and desync the scroll viewport.
		b.WriteString(clipLine(lines[i], m.width))
		b.WriteString("\n")
	}

	return b.String()
}

// renderTabs builds the tab bar shown at the top of the detail panel.
func (m DetailModel) renderTabs() string {
	tabs := []struct {
		view  DetailView
		label string
	}{
		{ViewInfo, "info"},
		{ViewEnv, "1 env"},
		{ViewTop, "2 top"},
		{ViewNetstat, "3 net"},
	}

	parts := make([]string, 0, len(tabs))
	for _, t := range tabs {
		if t.view == m.view {
			parts = append(parts, detailTabActiveStyle.Render(" "+t.label+" "))
		} else {
			parts = append(parts, detailTabStyle.Render(" "+t.label+" "))
		}
	}
	return strings.Join(parts, " ")
}

// renderInfoView builds the default container info key-value display.
func (m DetailModel) renderInfoView() string {
	c := m.container
	if c == nil {
		return ""
	}

	var b strings.Builder

	// Display container ID (truncate to 12 chars for readability)
	id := c.ID
	if len(id) > 12 {
		id = id[:12]
	}

	writeField(&b, "ID", id)
	writeField(&b, "Name", c.Name)
	writeField(&b, "Image", c.Image)
	writeField(&b, "State", tui.StateDot(c.State))
	writeField(&b, "Pod", c.PodName)
	writeField(&b, "Namespace", c.Namespace)
	writeField(&b, "Created", c.CreatedAt.Format(time.DateTime))
	writeField(&b, "PID", fmt.Sprintf("%d", c.Pid))

	// Labels
	if len(c.Labels) > 0 {
		b.WriteString(detailKeyStyle.Render("Labels:"))
		b.WriteString("\n")
		for k, v := range c.Labels {
			b.WriteString("  ")
			b.WriteString(detailValueStyle.Render(fmt.Sprintf("%s: %s", k, v)))
			b.WriteString("\n")
		}
	} else {
		writeField(&b, "Labels", "(none)")
	}

	return b.String()
}

// writeField writes a formatted key-value line to the builder.
func writeField(b *strings.Builder, key, value string) {
	b.WriteString(detailKeyStyle.Render(fmt.Sprintf("%-10s", key+":")))
	b.WriteString(" ")
	b.WriteString(detailValueStyle.Render(value))
	b.WriteString("\n")
}

// viewportHeight returns the number of content lines visible in the panel.
// Subtracts 1 for the header line.
func (m DetailModel) viewportHeight() int {
	h := m.height - 1
	if h < 1 {
		h = 1
	}
	return h
}

// clampScroll ensures scroll is within valid bounds.
func (m *DetailModel) clampScroll() {
	if m.scroll < 0 {
		m.scroll = 0
	}

	// Get content lines count
	var content string
	if m.view == ViewInfo && m.container != nil {
		content = m.renderInfoView()
	} else {
		content = m.content
	}

	if content == "" {
		m.scroll = 0
		return
	}

	lines := strings.Split(content, "\n")
	maxScroll := len(lines) - m.viewportHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

// viewCommandName returns a human-readable name for the current debug view.
func (m DetailModel) viewCommandName() string {
	switch m.view {
	case ViewEnv:
		return "env"
	case ViewTop:
		return "top"
	case ViewNetstat:
		return "netstat"
	default:
		return "info"
	}
}
