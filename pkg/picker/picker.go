package picker

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/containershell/containershell/pkg/runtime"
)

var (
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("99")).Foreground(lipgloss.Color("231")).Bold(true)
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	filterStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// stateColor maps a container state to its semantic color.
func stateColor(state string) lipgloss.Color {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running", "up":
		return lipgloss.Color("42") // green
	case "paused":
		return lipgloss.Color("214") // amber
	case "exited", "stopped", "dead", "removing":
		return lipgloss.Color("203") // red
	case "created", "configured", "restarting":
		return lipgloss.Color("39") // blue
	default:
		return lipgloss.Color("244")
	}
}

// statusDot renders a ● colored by container state.
func statusDot(state string) string {
	return lipgloss.NewStyle().Foreground(stateColor(state)).Render("●")
}

type model struct {
	containers []runtime.ContainerInfo
	filtered   []runtime.ContainerInfo
	cursor     int
	offset     int // scroll offset for the viewport
	filter     string
	width      int
	height     int
	selected   *runtime.ContainerInfo
	quitting   bool
}

func initialModel(containers []runtime.ContainerInfo) model {
	return model{
		containers: containers,
		filtered:   containers,
		// Sensible defaults so the first frame (before the terminal reports its
		// size) is already bounded and cannot overflow the screen.
		width:  80,
		height: 24,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollIntoView()

	case tea.KeyMsg:
		switch msg.String() {
		// Only Esc/Ctrl-C cancel. `q` is intentionally NOT a quit key so it can
		// be typed into the filter (the old picker swallowed q/j/k).
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
				m.scrollIntoView()
			}

		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.scrollIntoView()
			}

		case "enter":
			if len(m.filtered) > 0 {
				sel := m.filtered[m.cursor]
				m.selected = &sel
			}
			m.quitting = true
			return m, tea.Quit

		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.applyFilter()
			}

		default:
			// Type printable characters into the filter.
			if s := msg.String(); len(s) == 1 && s[0] >= 32 {
				m.filter += s
				m.applyFilter()
			}
		}
	}
	return m, nil
}

// applyFilter re-filters the container list against the current filter text and
// keeps the cursor and viewport within bounds.
func (m *model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.containers
	} else {
		lower := strings.ToLower(m.filter)
		m.filtered = nil
		for _, c := range m.containers {
			hay := strings.ToLower(fmt.Sprintf("%s %s %s %s %s",
				c.Name, c.PodName, c.Namespace, c.Image, c.ID))
			if strings.Contains(hay, lower) {
				m.filtered = append(m.filtered, c)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.scrollIntoView()
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render(clipLine("ContainerShell — Select a container", m.width)))
	b.WriteString("\n")

	if m.filter != "" {
		b.WriteString(filterStyle.Render(clipLine("Filter: "+m.filter+"█", m.width)))
	} else {
		b.WriteString(dimStyle.Render(clipLine("Type to filter · ↑/↓ or Ctrl-N/P to move · Enter to select · Esc to cancel", m.width)))
	}
	b.WriteString("\n\n")

	// Compute a column layout that fits the terminal width, and render the
	// header with it so header and rows stay aligned and never wrap.
	cols := m.columns()
	header := renderRow(cols, "  ", "NAME", "POD", "NAMESPACE", "IMAGE", "AGE")
	b.WriteString(dimStyle.Render(clipLine(header, m.width)))
	b.WriteString("\n")

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render(clipLine("  No containers match the filter", m.width)))
		return b.String()
	}

	visible := m.visibleRows()
	end := m.offset + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.offset; i < end; i++ {
		c := m.filtered[i]
		age := formatAge(time.Since(c.CreatedAt))

		if i == m.cursor {
			// Selected: ▸ marker and a full-width accent highlight.
			line := renderRow(cols, "▸ ", c.Name, c.PodName, c.Namespace, c.Image, age)
			b.WriteString(selectedStyle.Render(padLine(line, m.width)))
		} else {
			// Unselected: a state-colored ● marker. Hard-clip so a row never
			// wraps onto a second line and desyncs the cursor.
			body := renderRow(cols, "", c.Name, c.PodName, c.Namespace, c.Image, age)
			b.WriteString(clipLine(statusDot(c.State)+" "+normalStyle.Render(body), m.width))
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render(clipLine(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.filtered)), m.width)))

	return b.String()
}

// pickerChrome is the number of non-row lines View always renders: the title,
// the filter/help line, a blank spacer, the column header, and the footer.
const pickerChrome = 5

// visibleRows returns how many container rows fit in the current viewport.
func (m model) visibleRows() int {
	rows := m.height - pickerChrome
	if rows < 1 {
		rows = 1
	}
	return rows
}

// scrollIntoView adjusts the scroll offset so the cursor stays on screen.
func (m *model) scrollIntoView() {
	visible := m.visibleRows()

	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}

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

// Column width bounds. name and image are the flexible columns that grow toward
// their ideal as space allows; age is fixed. pod/namespace are only offered when
// a container populates them (Kubernetes), so plain Docker/Podman lists give
// that room to the name and image instead.
const (
	pickerMarker = 2 // "▸ " selection marker (or blank indent)
	pickerGap    = 1 // single space between columns

	pNameMin, pNameIdeal = 6, 24
	pPodMin, pPodIdeal   = 6, 20
	pNsMin, pNsIdeal     = 6, 14
	pImgMin, pImgIdeal   = 10, 40
	pAgeWidth            = 6
)

type pickerColumns struct {
	name, pod, ns, image, age int
}

func (m model) columns() pickerColumns {
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

// computeColumns allocates column widths that fit within the given terminal
// width. The returned widths plus the marker and inter-column gaps never exceed
// width, so an assembled row cannot wrap.
func computeColumns(width int, hasPod, hasNs bool) pickerColumns {
	var cols pickerColumns

	budget := width - pickerMarker
	if budget < 1 {
		cols.name = 1 // degenerate; the row is clipped when rendered
		return cols
	}

	cols.name = min(pNameMin, budget)
	used := cols.name

	add := func(w int) bool {
		need := pickerGap + w
		if used+need > budget {
			return false
		}
		used += need
		return true
	}

	// Inclusion priority (name is already in): age, image, namespace, pod.
	if add(pAgeWidth) {
		cols.age = pAgeWidth
	}
	if add(pImgMin) {
		cols.image = pImgMin
	}
	if hasNs && add(pNsMin) {
		cols.ns = pNsMin
	}
	if hasPod && add(pPodMin) {
		cols.pod = pPodMin
	}

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
	grow(&cols.name, pNameIdeal)
	grow(&cols.image, pImgIdeal)
	grow(&cols.pod, pPodIdeal)
	grow(&cols.ns, pNsIdeal)

	return cols
}

// renderRow assembles one line: the marker followed by each visible column,
// single-space separated, each padded/truncated to its column width.
func renderRow(cols pickerColumns, marker, name, pod, ns, image, age string) string {
	parts := make([]string, 0, 5)
	parts = append(parts, fitCol(name, cols.name))
	if cols.pod > 0 {
		parts = append(parts, fitCol(pod, cols.pod))
	}
	if cols.ns > 0 {
		parts = append(parts, fitCol(ns, cols.ns))
	}
	if cols.image > 0 {
		parts = append(parts, fitCol(image, cols.image))
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
func clipLine(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Truncate(s, w, "")
}

// padLine clips s to w display columns and pads it with spaces to exactly w, so
// a background highlight fills the full row width.
func padLine(s string, w int) string {
	if w <= 0 {
		return s
	}
	s = ansi.Truncate(s, w, "")
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// Pick launches the interactive container picker and returns the selected container.
// Returns an error if the user cancels or there are no containers.
func Pick(containers []runtime.ContainerInfo) (*runtime.ContainerInfo, error) {
	if len(containers) == 0 {
		return nil, fmt.Errorf("no running containers found")
	}

	// If only one container, select it automatically.
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
	if final.selected == nil {
		return nil, fmt.Errorf("cancelled by user")
	}

	return final.selected, nil
}
