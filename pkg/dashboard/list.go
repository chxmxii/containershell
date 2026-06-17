package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/runtime"
)

// SortField represents the column by which containers are sorted.
type SortField int

const (
	SortName SortField = iota
	SortPod
	SortNamespace
	SortAge
	SortState
)

// Styles for the list panel.
var (
	listSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	listNormalStyle   = lipgloss.NewStyle()
	listDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	listHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
)

// ListModel is the container list panel sub-model.
type ListModel struct {
	containers []runtime.ContainerInfo
	filtered   []runtime.ContainerInfo
	cursor     int
	offset     int // scroll offset for viewport
	filter     string
	filterMode bool
	sortField  SortField
	height     int
	width      int
	err        error
	lastRefresh time.Time
}

// NewListModel creates a new ListModel with default state.
func NewListModel() ListModel {
	return ListModel{
		sortField: SortName,
	}
}

// Init implements tea.Model for ListModel.
func (m ListModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the list panel.
// Handles cursor navigation (Up/Down/k/j), filtering (inline and `/` mode), and Esc.
func (m ListModel) Update(msg tea.Msg) (ListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		keyStr := msg.String()

		// Handle filterMode-specific keys first
		if m.filterMode {
			switch keyStr {
			case "esc":
				// Clear filter and exit filterMode
				m.filter = ""
				m.filterMode = false
				m.reapplyFilter()
				return m, nil
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					m.reapplyFilter()
				}
				return m, nil
			case "enter":
				// Confirm filter, exit filterMode but keep filter text
				m.filterMode = false
				return m, nil
			case "up", "down", "k", "j":
				// Allow navigation even in filter mode
			default:
				// Type characters into filter
				if len(keyStr) == 1 && keyStr[0] >= 32 {
					m.filter += keyStr
					m.reapplyFilter()
					return m, nil
				}
				return m, nil
			}
		}

		switch keyStr {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.scrollIntoView()
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.scrollIntoView()
			}
		case "/":
			// Enter dedicated filter input mode
			m.filterMode = true
		case "S":
			// Cycle sort field and re-sort
			var selectedID string
			if m.cursor >= 0 && m.cursor < len(m.filtered) {
				selectedID = m.filtered[m.cursor].ID
			}

			m.sortField = NextSortField(m.sortField)
			m.filtered = SortContainers(m.filtered, m.sortField)

			// Restore selection
			if selectedID != "" {
				found := false
				for i, c := range m.filtered {
					if c.ID == selectedID {
						m.cursor = i
						found = true
						break
					}
				}
				if !found {
					m.cursor = 0
				}
			} else {
				m.cursor = 0
			}
			m.scrollIntoView()
		case "esc":
			// Clear filter when not in filterMode (inline filtering reset)
			if m.filter != "" {
				m.filter = ""
				m.reapplyFilter()
			}
		default:
			// Inline filtering: type alphanumeric chars to filter when not in filterMode
			if !m.filterMode && len(keyStr) == 1 && keyStr[0] >= 32 {
				m.filter += keyStr
				m.reapplyFilter()
			}
		}

	case containersLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.containers = msg.containers
		m.lastRefresh = time.Now()

		// Re-apply current filter to new container list
		m.reapplyFilter()
	}

	return m, nil
}

// reapplyFilter applies the current filter to the full container list, sorts, and clamps the cursor.
func (m *ListModel) reapplyFilter() {
	m.filtered = ApplyFilter(m.containers, m.filter)
	m.filtered = SortContainers(m.filtered, m.sortField)

	// Clamp cursor to new filtered list bounds
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.scrollIntoView()
}

// ApplyFilter performs case-insensitive substring matching against container name,
// pod name, namespace, and image fields. Returns only containers where at least one
// field contains the filter string. If filter is empty, all containers are returned.
func ApplyFilter(containers []runtime.ContainerInfo, filter string) []runtime.ContainerInfo {
	if filter == "" {
		// Return a copy to avoid mutation issues
		result := make([]runtime.ContainerInfo, len(containers))
		copy(result, containers)
		return result
	}

	lowerFilter := strings.ToLower(filter)
	var result []runtime.ContainerInfo

	for _, c := range containers {
		if strings.Contains(strings.ToLower(c.Name), lowerFilter) ||
			strings.Contains(strings.ToLower(c.PodName), lowerFilter) ||
			strings.Contains(strings.ToLower(c.Namespace), lowerFilter) ||
			strings.Contains(strings.ToLower(c.Image), lowerFilter) {
			result = append(result, c)
		}
	}

	return result
}

// View renders the container list panel.
func (m ListModel) View() string {
	var b strings.Builder

	// Render header
	b.WriteString(listHeaderStyle.Render(fmt.Sprintf("Containers [sort: %s]", SortFieldLabel(m.sortField))))
	b.WriteString("\n")

	// Show filter prompt when filterMode is active, or show inline filter indicator
	if m.filterMode {
		b.WriteString(listDimStyle.Render(fmt.Sprintf("  Filter: %s█", m.filter)))
		b.WriteString("\n")
	} else if m.filter != "" {
		b.WriteString(listDimStyle.Render(fmt.Sprintf("  [filter: %s]", m.filter)))
		b.WriteString("\n")
	}

	// Render column headers
	header := fmt.Sprintf("  %-20s %-20s %-12s %-10s %-8s",
		"NAME", "POD", "NAMESPACE", "STATE", "AGE")
	b.WriteString(listDimStyle.Render(header))
	b.WriteString("\n")

	// Handle error state
	if m.err != nil {
		b.WriteString(listDimStyle.Render(fmt.Sprintf("  Error: %s", m.err.Error())))
		b.WriteString("\n")
		return b.String()
	}

	// Handle empty list
	if len(m.filtered) == 0 {
		if m.filter != "" {
			b.WriteString(listDimStyle.Render("  No containers match the filter"))
		} else {
			b.WriteString(listDimStyle.Render("  No running containers found"))
		}
		b.WriteString("\n")
		return b.String()
	}

	// Calculate visible rows (subtract header lines from available height)
	visibleRows := m.visibleRows()

	// Render visible container rows
	end := m.offset + visibleRows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.offset; i < end; i++ {
		c := m.filtered[i]
		age := formatAge(time.Since(c.CreatedAt))
		name := truncateStr(c.Name, 20)
		pod := truncateStr(c.PodName, 20)
		ns := truncateStr(c.Namespace, 12)
		state := truncateStr(c.State, 10)

		line := fmt.Sprintf("  %-20s %-20s %-12s %-10s %-8s", name, pod, ns, state, age)

		if i == m.cursor {
			b.WriteString(listSelectedStyle.Render("▸ " + line[2:]))
		} else {
			b.WriteString(listNormalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// SelectedContainer returns the currently selected container, or nil if none.
func (m ListModel) SelectedContainer() *runtime.ContainerInfo {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	c := m.filtered[m.cursor]
	return &c
}

// SetDimensions updates the panel dimensions.
func (m *ListModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
	m.scrollIntoView()
}

// visibleRows returns the number of container rows that fit in the viewport.
// Subtracts 2 for the header line and column header line, plus 1 if filter prompt is shown.
func (m ListModel) visibleRows() int {
	rows := m.height - 2
	if m.filterMode || m.filter != "" {
		rows-- // extra line for filter prompt/indicator
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// scrollIntoView adjusts the offset so the cursor is visible within the viewport.
func (m *ListModel) scrollIntoView() {
	visible := m.visibleRows()

	// If cursor is above the visible area, scroll up
	if m.cursor < m.offset {
		m.offset = m.cursor
	}

	// If cursor is below the visible area, scroll down
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}

	// Clamp offset to valid range
	maxOffset := len(m.filtered) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// formatAge formats a duration as a human-readable age string.
// Rules: <1min → "Xs", <1hr → "Xm", <24h → "XhYm", >=24h → "XdYh"
func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
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

// truncateStr truncates a string to n characters, adding "..." if truncated.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}


