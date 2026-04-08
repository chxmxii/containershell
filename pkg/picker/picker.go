package picker

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/runtime"
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	normalStyle   = lipgloss.NewStyle()
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	filterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type model struct {
	containers []runtime.ContainerInfo
	filtered   []runtime.ContainerInfo
	cursor     int
	filter     string
	selected   *runtime.ContainerInfo
	quitting   bool
}

func initialModel(containers []runtime.ContainerInfo) model {
	return model{
		containers: containers,
		filtered:   containers,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}

		case "enter":
			if len(m.filtered) > 0 {
				m.selected = &m.filtered[m.cursor]
			}
			return m, tea.Quit

		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.applyFilter()
			}

		default:
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m.applyFilter()
			}
		}
	}
	return m, nil
}

func (m *model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.containers
	} else {
		lower := strings.ToLower(m.filter)
		m.filtered = nil
		for _, c := range m.containers {
			searchStr := strings.ToLower(fmt.Sprintf("%s %s %s %s %s",
				c.Name, c.PodName, c.Namespace, c.Image, c.ID))
			if strings.Contains(searchStr, lower) {
				m.filtered = append(m.filtered, c)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = maxInt(0, len(m.filtered)-1)
	}
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("ContainerShell — Select a container"))
	b.WriteString("\n")

	if m.filter != "" {
		b.WriteString(filterStyle.Render(fmt.Sprintf("Filter: %s", m.filter)))
	} else {
		b.WriteString(dimStyle.Render("Type to filter, ↑/↓ to navigate, Enter to select, Esc to cancel"))
	}
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render(fmt.Sprintf("  %-20s %-25s %-15s %-40s %-14s",
		"NAME", "POD", "NAMESPACE", "IMAGE", "AGE")))
	b.WriteString("\n")

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("  No containers match the filter"))
		b.WriteString("\n")
	}

	for i, c := range m.filtered {
		age := formatAge(time.Since(c.CreatedAt))
		image := truncate(c.Image, 40)
		name := truncate(c.Name, 20)
		pod := truncate(c.PodName, 25)
		ns := truncate(c.Namespace, 15)

		line := fmt.Sprintf("  %-20s %-25s %-15s %-40s %-14s", name, pod, ns, image, age)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render("▸ " + line[2:]))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Pick launches the interactive container picker and returns the selected container.
// Returns nil if the user cancels.
func Pick(containers []runtime.ContainerInfo) (*runtime.ContainerInfo, error) {
	if len(containers) == 0 {
		return nil, fmt.Errorf("no running containers found")
	}

	// If only one container, select it automatically
	if len(containers) == 1 {
		return &containers[0], nil
	}

	m := initialModel(containers)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("picker failed: %w", err)
	}

	final := result.(model)
	if final.quitting && final.selected == nil {
		return nil, fmt.Errorf("cancelled by user")
	}

	return final.selected, nil
}
