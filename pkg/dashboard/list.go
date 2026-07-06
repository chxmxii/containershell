package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

	// Compute a column layout that fits the panel width, then render the header
	// with it so the header and rows always stay aligned and never wrap.
	cols := m.columns()
	header := renderListRow(cols, "  ", "NAME", "POD", "NAMESPACE", "STATE", "AGE")
	b.WriteString(listDimStyle.Render(clipLine(header, m.width)))
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

		marker := "  "
		if i == m.cursor {
			marker = "▸ "
		}

		line := renderListRow(cols, marker,
			c.Name, c.PodName, c.Namespace, c.State, formatAge(time.Since(c.CreatedAt)))
		// Final guard: never exceed the panel width, so a row can never wrap
		// onto a second line and desync the scroll viewport.
		line = clipLine(line, m.width)

		if i == m.cursor {
			b.WriteString(listSelectedStyle.Render(line))
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

// Column width bounds. The name, pod, and namespace columns are flexible and
// grow toward their ideal width as space allows; state and age are fixed. When
// the panel is too narrow for every column, columns are dropped in reverse
// priority so the most useful fields (name, state, age) survive.
const (
	listMarkerWidth = 2 // "▸ " selection marker (or blank indent)
	listColGap      = 1 // single space between columns

	colNameMin, colNameIdeal = 6, 28
	colPodMin, colPodIdeal   = 6, 22
	colNsMin, colNsIdeal     = 6, 14
	colStateWidth            = 8
	colAgeWidth              = 6
)

// listColumns holds the rendered width of each list column. A width of 0 hides
// the column, either because there is no room or no container has that field.
type listColumns struct {
	name, pod, ns, state, age int
}

// columns computes the column layout for the current panel width and container
// set. Pod and namespace columns are only offered when at least one container
// populates them (e.g. Kubernetes-managed containers), so plain Docker/Podman
// lists give that space to the name instead.
func (m ListModel) columns() listColumns {
	hasPod, hasNs := false, false
	for i := range m.filtered {
		if m.filtered[i].PodName != "" {
			hasPod = true
		}
		if m.filtered[i].Namespace != "" {
			hasNs = true
		}
		if hasPod && hasNs {
			break
		}
	}
	return computeColumns(m.width, hasPod, hasNs)
}

// computeColumns allocates column widths that fit within a panel of the given
// content width. The returned widths (plus the marker and inter-column gaps)
// are guaranteed to never exceed width, so an assembled row cannot wrap.
func computeColumns(width int, hasPod, hasNs bool) listColumns {
	var cols listColumns

	budget := width - listMarkerWidth
	if budget < 1 {
		cols.name = 1 // degenerate width; the row is clipped when rendered
		return cols
	}

	// The name column is always present; start it at its minimum.
	cols.name = min(colNameMin, budget)
	used := cols.name

	// add reserves w columns plus a leading gap if it fits the remaining budget.
	add := func(w int) bool {
		need := listColGap + w
		if used+need > budget {
			return false
		}
		used += need
		return true
	}

	// Inclusion priority (name is already in): state, age, namespace, pod.
	if add(colStateWidth) {
		cols.state = colStateWidth
	}
	if add(colAgeWidth) {
		cols.age = colAgeWidth
	}
	if hasNs && add(colNsMin) {
		cols.ns = colNsMin
	}
	if hasPod && add(colPodMin) {
		cols.pod = colPodMin
	}

	// Distribute any leftover space to the flexible columns, up to their ideals.
	leftover := budget - used
	grow := func(cur *int, ideal int) {
		if *cur == 0 {
			return
		}
		if take := min(ideal-*cur, leftover); take > 0 {
			*cur += take
			leftover -= take
		}
	}
	grow(&cols.name, colNameIdeal)
	grow(&cols.pod, colPodIdeal)
	grow(&cols.ns, colNsIdeal)

	return cols
}

// renderListRow assembles one list line: the marker followed by each visible
// column, single-space separated. Values are padded or truncated to fit their
// column so all rows align.
func renderListRow(cols listColumns, marker, name, pod, ns, state, age string) string {
	parts := make([]string, 0, 5)
	parts = append(parts, fitCol(name, cols.name))
	if cols.pod > 0 {
		parts = append(parts, fitCol(pod, cols.pod))
	}
	if cols.ns > 0 {
		parts = append(parts, fitCol(ns, cols.ns))
	}
	if cols.state > 0 {
		parts = append(parts, fitCol(state, cols.state))
	}
	if cols.age > 0 {
		parts = append(parts, fitCol(age, cols.age))
	}
	return marker + strings.Join(parts, " ")
}

// fitCol pads or truncates s to exactly w display columns, accounting for wide
// runes and any ANSI escape sequences.
func fitCol(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// clipLine truncates s to at most w display columns to prevent line wrapping.
// A non-positive width returns s unchanged (the width is not yet known).
func clipLine(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Truncate(s, w, "")
}


