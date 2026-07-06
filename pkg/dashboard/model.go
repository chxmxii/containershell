package dashboard

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/debug"
	"github.com/containershell/containershell/pkg/runtime"
	"github.com/containershell/containershell/pkg/shell"
)

// FocusPanel indicates which panel currently receives keyboard input.
type FocusPanel int

const (
	FocusList   FocusPanel = iota
	FocusDetail
)

// Model is the root bubbletea model for the TUI dashboard.
type Model struct {
	// Sub-models
	list      ListModel
	detail    DetailModel
	statusBar StatusBarModel
	actionBar ActionBarModel
	overlay   *OverlayModel // nil when no overlay is active

	// State
	focus  FocusPanel
	width  int
	height int
	rt     runtime.Runtime
	rtInfo *runtime.RuntimeInfo
	err    error

	// Config
	refreshInterval time.Duration
	verbose         bool
}

// NewModel creates a new dashboard Model with the given config and runtime.
func NewModel(cfg Config, rt runtime.Runtime, rtInfo *runtime.RuntimeInfo) Model {
	m := Model{
		list:            NewListModel(),
		detail:          NewDetailModel(),
		statusBar:       NewStatusBarModel(),
		actionBar:       NewActionBarModel(),
		focus:           FocusList,
		rt:              rt,
		rtInfo:          rtInfo,
		refreshInterval: cfg.RefreshInterval,
		verbose:         cfg.Verbose,
	}
	if rtInfo != nil {
		m.statusBar.SetRuntimeInfo(rtInfo)
	}
	return m
}

// Init implements tea.Model. It returns a batch of initial commands:
// fetch runtime info, load containers, and start the polling tick.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.fetchRuntimeInfoCmd(),
		m.fetchContainersCmd(),
		m.tickCmd(),
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model. It routes messages based on focus and overlay state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recomputeLayout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		return m, tea.Batch(m.fetchContainersCmd(), m.tickCmd())

	case containersLoadedMsg:
		return m.handleContainersLoaded(msg), nil

	case runtimeInfoMsg:
		return m.handleRuntimeInfo(msg), nil

	case debugOutputMsg:
		return m.handleDebugOutput(msg), nil

	case shellDoneMsg:
		// TUI is automatically restored after tea.Exec returns.
		// Nothing to do here.
		return m, nil

	case logLineMsg:
		if m.overlay != nil {
			m.overlay.AppendLine(string(msg))
		}
		return m, nil
	}

	return m, nil
}

// handleKey processes key messages, routing to overlay or focused panel.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// If overlay is open, route keys to it first
	if m.overlay != nil {
		overlay, cmd, shouldClose := m.overlay.Update(msg)
		m.overlay = overlay
		if shouldClose {
			m.overlay = nil
			m.updateActionBarContext()
		}
		return m, cmd
	}

	// Global keys (no overlay open)
	switch keyStr {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.cycleFocus()
		return m, nil

	case "?":
		m.openHelpOverlay()
		return m, nil

	case "enter", "s":
		return m.handleShellKey()

	case "l":
		return m.handleLogsKey()

	case "i":
		return m.handleDebugKey("Inspect", debug.InspectOutput)

	case "t":
		return m.handleDebugKey("Processes (top)", debug.TopOutput)

	case "e":
		return m.handleDebugKey("Environment", debug.EnvOutput)

	case "n":
		return m.handleDebugKey("Network (netstat)", debug.NetstatOutput)

	case "r":
		// Immediate refresh
		return m, m.fetchContainersCmd()

	default:
		// Route to focused panel
		return m.routeToFocusedPanel(msg)
	}
}

// cycleFocus toggles focus between List and Detail panels.
func (m *Model) cycleFocus() {
	switch m.focus {
	case FocusList:
		m.focus = FocusDetail
	case FocusDetail:
		m.focus = FocusList
	}
	m.updateActionBarContext()
}

// updateActionBarContext sets the action bar shortcuts based on current state.
func (m *Model) updateActionBarContext() {
	if m.overlay != nil {
		m.actionBar.SetContext(ContextOverlay)
		return
	}
	switch m.focus {
	case FocusList:
		m.actionBar.SetContext(ContextList)
	case FocusDetail:
		m.actionBar.SetContext(ContextDetail)
	}
}

// routeToFocusedPanel sends key messages to the currently focused sub-model.
func (m Model) routeToFocusedPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.focus {
	case FocusList:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		// Update detail panel with selected container
		if sel := m.list.SelectedContainer(); sel != nil {
			m.detail.SetContainer(sel)
		}
		return m, cmd
	case FocusDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleShellKey launches an interactive shell session for the selected container.
func (m Model) handleShellKey() (tea.Model, tea.Cmd) {
	container := m.list.SelectedContainer()
	if container == nil {
		return m, nil
	}
	return m, m.shellCmd(container)
}

// handleLogsKey opens the log overlay for the selected container.
func (m Model) handleLogsKey() (tea.Model, tea.Cmd) {
	container := m.list.SelectedContainer()
	if container == nil {
		return m, nil
	}

	overlay := NewOverlayModel("Logs: "+container.Name, m.width, m.height)
	overlay.SetLoading(true)
	m.overlay = overlay
	m.updateActionBarContext()

	return m, m.fetchLogsCmd(container.ID)
}

// handleDebugKey opens a debug overlay and dispatches the corresponding command.
func (m Model) handleDebugKey(title string, fn func(ctx context.Context, rt runtime.Runtime, containerID string) (string, error)) (tea.Model, tea.Cmd) {
	container := m.list.SelectedContainer()
	if container == nil {
		return m, nil
	}

	overlay := NewOverlayModel(title+": "+container.Name, m.width, m.height)
	overlay.SetLoading(true)
	m.overlay = overlay
	m.updateActionBarContext()

	return m, m.debugCmd(title, fn, container.ID)
}

// openHelpOverlay opens the help overlay displaying all keyboard shortcuts.
func (m *Model) openHelpOverlay() {
	overlay := NewOverlayModel("Help", m.width, m.height)
	overlay.SetContent(m.buildHelpContent())
	m.overlay = overlay
	m.actionBar.SetContext(ContextHelp)
}

// buildHelpContent generates the help text listing all keyboard shortcuts.
func (m Model) buildHelpContent() string {
	var b strings.Builder

	b.WriteString("Container List Panel:\n")
	b.WriteString("  ↑/k        Move selection up\n")
	b.WriteString("  ↓/j        Move selection down\n")
	b.WriteString("  Enter/s    Shell into selected container\n")
	b.WriteString("  l          View container logs\n")
	b.WriteString("  i          Inspect container metadata\n")
	b.WriteString("  t          View process list (top)\n")
	b.WriteString("  e          View environment variables\n")
	b.WriteString("  n          View network connections (netstat)\n")
	b.WriteString("  /          Activate filter input mode\n")
	b.WriteString("  S          Cycle sort field\n")
	b.WriteString("  r          Refresh container list\n")
	b.WriteString("  Tab        Switch focus panel\n")
	b.WriteString("\n")

	b.WriteString("Detail Panel:\n")
	b.WriteString("  ↑/k        Scroll up\n")
	b.WriteString("  ↓/j        Scroll down\n")
	b.WriteString("  1          Show environment variables\n")
	b.WriteString("  2          Show process list\n")
	b.WriteString("  3          Show network connections\n")
	b.WriteString("  Tab        Switch focus panel\n")
	b.WriteString("\n")

	b.WriteString("Overlay:\n")
	b.WriteString("  ↑/k        Scroll up\n")
	b.WriteString("  ↓/j        Scroll down\n")
	b.WriteString("  PgUp/PgDn  Page scroll\n")
	b.WriteString("  g/G        Jump to top/bottom\n")
	b.WriteString("  f          Toggle follow mode (logs)\n")
	b.WriteString("  Esc/q      Close overlay\n")
	b.WriteString("\n")

	b.WriteString("Global:\n")
	b.WriteString("  ?          Show this help\n")
	b.WriteString("  q/Ctrl+C   Quit dashboard\n")

	return b.String()
}

// handleContainersLoaded processes the result of a ListContainers call.
func (m Model) handleContainersLoaded(msg containersLoadedMsg) Model {
	// Forward to list model to update containers
	m.list, _ = m.list.Update(msg)

	if msg.err == nil {
		m.statusBar.SetContainerCount(len(msg.containers))
		m.statusBar.SetLastRefresh(time.Now())
	}

	// Update detail panel selection if cursor changed
	if sel := m.list.SelectedContainer(); sel != nil {
		m.detail.SetContainer(sel)
	}

	return m
}

// handleRuntimeInfo processes the runtime info response.
func (m Model) handleRuntimeInfo(msg runtimeInfoMsg) Model {
	if msg.err != nil {
		m.statusBar.SetDisconnected()
		return m
	}
	m.rtInfo = msg.info
	m.statusBar.SetRuntimeInfo(msg.info)
	return m
}

// handleDebugOutput processes the result of a debug command execution.
func (m Model) handleDebugOutput(msg debugOutputMsg) Model {
	if m.overlay != nil {
		m.overlay.SetLoading(false)
		if msg.err != nil {
			m.overlay.SetError(msg.err)
		} else {
			m.overlay.SetContent(msg.content)
		}
	}
	return m
}

// recomputeLayout recalculates panel dimensions after a resize.
func (m *Model) recomputeLayout() {
	layout := ComputeLayout(m.width, m.height, DefaultLayoutConfig())

	m.statusBar.SetDimensions(m.width)
	m.actionBar.SetDimensions(m.width)

	// The list and detail panels are rendered inside a single-cell border in
	// renderContent, which consumes 2 columns and 2 rows. Hand each panel its
	// actual inner content area so its viewport math matches what is visible;
	// otherwise the panels render more rows/columns than fit and overflow.
	const border = 2
	listW := max(0, layout.ListWidth-border)
	detailW := max(0, layout.DetailWidth-border)
	contentH := max(0, layout.ContentHeight-border)
	m.list.SetDimensions(listW, contentH)
	m.detail.SetDimensions(detailW, contentH)

	if m.overlay != nil {
		m.overlay.SetDimensions(m.width, m.height)
	}
}

// View implements tea.Model. It composes the final view from all sub-models.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	layout := ComputeLayout(m.width, m.height, DefaultLayoutConfig())

	// If terminal is too small, show size requirement message
	if layout.Mode == LayoutTooSmall {
		return m.renderTooSmall()
	}

	var sections []string

	// Status bar at top
	sections = append(sections, m.statusBar.View())

	// Content area: list and detail panels
	contentView := m.renderContent(layout)
	sections = append(sections, contentView)

	// Action bar at bottom (if shown)
	if layout.ShowActionBar {
		sections = append(sections, m.actionBar.View())
	}

	view := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// If overlay is active, render it on top
	if m.overlay != nil {
		view = m.renderWithOverlay(view)
	}

	return view
}

// renderTooSmall renders the terminal-too-small message.
func (m Model) renderTooSmall() string {
	msg := fmt.Sprintf("Terminal too small (%dx%d). Minimum: 80x24.", m.width, m.height)
	style := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center)
	return style.Render(msg)
}

// renderContent renders the main content area (list + detail panels).
func (m Model) renderContent(layout Layout) string {
	// Styles for panel borders
	activeBorder := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(layout.ListWidth - 2).
		Height(layout.ContentHeight - 2)

	inactiveBorder := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(layout.ListWidth - 2).
		Height(layout.ContentHeight - 2)

	// Panel Views terminate each line with a newline; trim the trailing one so a
	// panel that exactly fills its viewport does not spill a phantom blank line
	// past the border and push the layout beyond the terminal height.
	listView := strings.TrimSuffix(m.list.View(), "\n")
	detailView := strings.TrimSuffix(m.detail.View(), "\n")

	switch layout.Mode {
	case LayoutStacked:
		// Single panel: just the list at full width
		listStyle := activeBorder.Width(m.width - 2)
		return listStyle.Render(listView)

	default: // LayoutNormal
		// List panel
		var listPanel string
		if m.focus == FocusList {
			listStyle := activeBorder.Width(layout.ListWidth - 2)
			listPanel = listStyle.Render(listView)
		} else {
			listStyle := inactiveBorder.Width(layout.ListWidth - 2)
			listPanel = listStyle.Render(listView)
		}

		// Detail panel
		var detailPanel string
		if m.focus == FocusDetail {
			detailStyle := activeBorder.Width(layout.DetailWidth - 2)
			detailPanel = detailStyle.Render(detailView)
		} else {
			detailStyle := inactiveBorder.Width(layout.DetailWidth - 2)
			detailPanel = detailStyle.Render(detailView)
		}

		return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, detailPanel)
	}
}

// renderWithOverlay composites the overlay on top of the main view.
func (m Model) renderWithOverlay(baseView string) string {
	if m.overlay == nil {
		return baseView
	}

	overlayView := m.overlay.View()

	// The overlay View() already handles centering, so we can use it directly.
	// For a proper overlay effect, we return the overlay rendered at full terminal size.
	// The base view is effectively hidden behind the overlay.
	return overlayView
}

// --- Commands ---

// tickCmd returns a command that sends a tickMsg after the configured refresh interval.
func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchContainersCmd returns a command that fetches the container list from the runtime.
func (m Model) fetchContainersCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		containers, err := m.rt.ListContainers(ctx)
		return containersLoadedMsg{containers: containers, err: err}
	}
}

// fetchRuntimeInfoCmd returns a command that fetches runtime info.
func (m Model) fetchRuntimeInfoCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		info, err := m.rt.RuntimeInfo(ctx)
		return runtimeInfoMsg{info: info, err: err}
	}
}

// debugCmd returns a command that executes a debug function and returns the output.
func (m Model) debugCmd(title string, fn func(ctx context.Context, rt runtime.Runtime, containerID string) (string, error), containerID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		content, err := fn(ctx, m.rt, containerID)
		return debugOutputMsg{title: title, content: content, err: err}
	}
}

// fetchLogsCmd returns a command that retrieves the last 100 log lines for a container.
func (m Model) fetchLogsCmd(containerID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		content, err := debug.LogsOutput(ctx, m.rt, containerID, 100)
		return debugOutputMsg{title: "Logs", content: content, err: err}
	}
}

// shellCmd returns a command that suspends the TUI and runs the shell fallback chain.
func (m Model) shellCmd(container *runtime.ContainerInfo) tea.Cmd {
	rt := m.rt
	verbose := m.verbose
	c := container

	return tea.Exec(&shellExecCmd{
		rt:        rt,
		container: c,
		verbose:   verbose,
		stderr:    io.Discard,
	}, func(err error) tea.Msg {
		return shellDoneMsg{err: err}
	})
}

// shellExecCmd implements tea.ExecCommand for running the shell fallback chain.
type shellExecCmd struct {
	rt        runtime.Runtime
	container *runtime.ContainerInfo
	verbose   bool
	stderr    io.Writer
}

// Run executes the shell fallback chain.
func (s *shellExecCmd) Run() error {
	ctx := context.Background()
	logf := func(format string, args ...any) {}
	if s.verbose {
		logf = func(format string, args ...any) {
			fmt.Fprintf(s.stderr, format+"\n", args...)
		}
	}
	return shell.FallbackChain(ctx, s.rt, s.container, shell.DefaultStrategies(), s.verbose, logf)
}

// SetStdin implements tea.ExecCommand.
func (s *shellExecCmd) SetStdin(r io.Reader) {}

// SetStdout implements tea.ExecCommand.
func (s *shellExecCmd) SetStdout(w io.Writer) {}

// SetStderr implements tea.ExecCommand.
func (s *shellExecCmd) SetStderr(w io.Writer) { s.stderr = w }
