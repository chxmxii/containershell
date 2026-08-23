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

// SortField represents the column by which containers are sorted.
type SortField int

const (
	SortName SortField = iota
	SortPod
	SortNamespace
	SortAge
	SortState
)

// Styles are read from pkg/tui at render time (never cached in package vars)
// so a theme switch takes effect immediately.

// ListModel is the container list panel sub-model.
type ListModel struct {
	containers  []runtime.ContainerInfo
	filtered    []runtime.ContainerInfo
	cursor      int
	offset      int // scroll offset for viewport
	filter      string
	filterMode  bool
	sortField   SortField
	height      int
	width       int
	err         error
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
			case "up", "down":
				// Allow navigation even in filter mode
			default:
				// Type characters into the filter. Rapid input can arrive as a
				// single multi-rune KeyMsg, so append every rune, not just
				// single-character strings.
				if msg.Type == tea.KeyRunes && !msg.Alt {
					m.filter += string(msg.Runes)
					m.reapplyFilter()
				} else if keyStr == " " {
					m.filter += " "
					m.reapplyFilter()
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
			m.cycleSort()
		case "esc":
			// Clear filter when not in filterMode (inline filtering reset)
			if m.filter != "" {
				m.filter = ""
				m.reapplyFilter()
			}
		default:
			// Inline filtering: type printable chars to filter when not in filterMode
			if !m.filterMode && msg.Type == tea.KeyRunes && !msg.Alt {
				m.filter += string(msg.Runes)
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

// cycleSort advances to the next sort field, re-sorts, and keeps the current
// selection (by container ID) under the cursor when it survives the re-sort.
func (m *ListModel) cycleSort() {
	var selectedID string
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		selectedID = m.filtered[m.cursor].ID
	}

	m.sortField = NextSortField(m.sortField)
	m.filtered = SortContainers(m.filtered, m.sortField)

	m.cursor = 0
	for i, c := range m.filtered {
		if selectedID != "" && c.ID == selectedID {
			m.cursor = i
			break
		}
	}
	m.scrollIntoView()
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

// View renders the container list panel. The panel title lives in the
// surrounding border (drawn by the root model), so the body is just the
// optional filter line, the column header, and the rows.
func (m ListModel) View() string {
	var b strings.Builder

	// Show filter prompt when filterMode is active, or show inline filter indicator
	if m.filterMode {
		b.WriteString(tui.FilterStyle.Render(clipLine(fmt.Sprintf("  / %s█", m.filter), m.width)))
		b.WriteString("\n")
	} else if m.filter != "" {
		b.WriteString(tui.DimStyle.Render(clipLine(fmt.Sprintf("  / %s  (esc clears)", m.filter), m.width)))
		b.WriteString("\n")
	}

	// Compute a column layout that fits the panel width, then render the header
	// with it so the header and rows always stay aligned and never wrap.
	cols := m.columns()
	header := renderListRow(cols, "  ",
		m.headerLabel("NAME", SortName), m.headerLabel("POD", SortPod),
		m.headerLabel("NAMESPACE", SortNamespace), m.headerLabel("STATE", SortState),
		m.headerLabel("AGE", SortAge))
	b.WriteString(tui.ColHeadStyle.Render(clipLine(header, m.width)))
	b.WriteString("\n")

	// Handle error state
	if m.err != nil {
		b.WriteString(tui.ErrStyle.Render(clipLine(fmt.Sprintf("  ✗ %s", m.err.Error()), m.width)))
		b.WriteString("\n")
		return b.String()
	}

	// Handle empty list
	if len(m.filtered) == 0 {
		if m.filter != "" {
			b.WriteString(tui.DimStyle.Render("  ∅ no containers match the filter"))
		} else {
			b.WriteString(tui.DimStyle.Render("  ∅ no running containers found"))
		}
		b.WriteString("\n")
		return b.String()
	}

	// Render visible container rows
	end := m.offset + m.visibleRows()
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.offset; i < end; i++ {
		c := m.filtered[i]
		age := formatAge(time.Since(c.CreatedAt))

		if i == m.cursor {
			// Selected row: plain text under a single full-width highlight so the
			// background bar stays unbroken; pad to the panel width.
			line := renderListRow(cols, "▸ ", c.Name, c.PodName, c.Namespace, "● "+c.State, age)
			b.WriteString(tui.SelectedStyle.Render(fitCol(line, m.width)))
		} else {
			// Normal row: state cell carries its own color, secondary cells are dim.
			// Each cell is fitted before styling, and the assembled line is clipped
			// (ANSI-aware) so a row can never wrap and desync the scroll viewport.
			b.WriteString(clipLine(m.renderStyledRow(cols, c, age), m.width))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// headerLabel marks the active sort column with a ▾ indicator.
func (m ListModel) headerLabel(label string, f SortField) string {
	if m.sortField == f {
		return label + " ▾"
	}
	return label
}

// renderStyledRow assembles one unselected list line with per-cell styling.
// It mirrors renderListRow's layout exactly so rows align with the header.
func (m ListModel) renderStyledRow(cols listColumns, c runtime.ContainerInfo, age string) string {
	parts := make([]string, 0, 5)
	parts = append(parts, fitCol(c.Name, cols.name))
	if cols.pod > 0 {
		parts = append(parts, tui.DimStyle.Render(fitCol(c.PodName, cols.pod)))
	}
	if cols.ns > 0 {
		parts = append(parts, tui.DimStyle.Render(fitCol(c.Namespace, cols.ns)))
	}
	if cols.state > 0 {
		stateStyle := lipgloss.NewStyle().Foreground(tui.StateColor(c.State))
		parts = append(parts, stateStyle.Render(fitCol("● "+c.State, cols.state)))
	}
	if cols.age > 0 {
		parts = append(parts, tui.DimStyle.Render(fitCol(age, cols.age)))
	}
	return "  " + strings.Join(parts, " ")
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
// Subtracts 1 for the column header line, plus 1 if the filter prompt is shown.
func (m ListModel) visibleRows() int {
	rows := m.height - 1
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
func formatAge(d time.Duration) string { return tui.FormatAge(d) }

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
	colStateWidth            = 10 // fits "● running" with its status dot
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
func fitCol(s string, w int) string { return tui.FitCol(s, w) }

// clipLine truncates s to at most w display columns to prevent line wrapping.
// A non-positive width returns s unchanged (the width is not yet known).
func clipLine(s string, w int) string { return tui.ClipLine(s, w) }
