package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/runtime"
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
	detailHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	detailKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	detailValueStyle  = lipgloss.NewStyle()
	detailDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	detailErrStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
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

	// Header
	title := "Detail"
	switch m.view {
	case ViewInfo:
		title = "Container Info"
	case ViewEnv:
		title = "Environment"
	case ViewTop:
		title = "Processes"
	case ViewNetstat:
		title = "Network"
	}
	b.WriteString(detailHeaderStyle.Render(title))
	b.WriteString("\n")

	// No container selected
	if m.container == nil {
		b.WriteString(detailDimStyle.Render("  Select a container from the list"))
		b.WriteString("\n")
		return b.String()
	}

	// Loading state
	if m.loading {
		b.WriteString(detailDimStyle.Render("  Loading..."))
		b.WriteString("\n")
		return b.String()
	}

	// Error state
	if m.err != nil {
		cmdName := m.viewCommandName()
		errLine := fmt.Sprintf("  Error (%s): %s", cmdName, m.err.Error())
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
	writeField(&b, "State", c.State)
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
