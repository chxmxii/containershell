package dashboard

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// OverlayModel represents a centered modal overlay for displaying debug output,
// logs, or help content on top of the main dashboard layout.
type OverlayModel struct {
	title   string
	content string
	lines   []string
	scroll  int
	height  int
	width   int
	follow  bool
	logChan chan string
	loading bool
	err     error
}

// NewOverlayModel creates a new OverlayModel with the given title and dimensions.
func NewOverlayModel(title string, width, height int) *OverlayModel {
	return &OverlayModel{
		title:  title,
		width:  width,
		height: height,
	}
}

// SetContent sets the overlay content and splits it into lines for scrolling.
func (o *OverlayModel) SetContent(content string) {
	o.content = content
	if content == "" {
		o.lines = nil
	} else {
		o.lines = strings.Split(content, "\n")
	}
	o.scroll = 0
}

// SetLoading sets the loading state of the overlay.
func (o *OverlayModel) SetLoading(loading bool) {
	o.loading = loading
}

// SetError sets the error state of the overlay.
func (o *OverlayModel) SetError(err error) {
	o.err = err
}

// SetDimensions updates the overlay dimensions (e.g., on terminal resize).
func (o *OverlayModel) SetDimensions(width, height int) {
	o.width = width
	o.height = height
	// Re-clamp scroll after resize
	o.clampScroll()
}

// ToggleFollow toggles the follow mode for log streaming.
func (o *OverlayModel) ToggleFollow() {
	o.follow = !o.follow
	if o.follow {
		// When enabling follow, scroll to bottom
		o.scrollToBottom()
	}
}

// AppendLine appends a new line to the overlay content (used for log streaming).
func (o *OverlayModel) AppendLine(line string) {
	o.lines = append(o.lines, line)
	o.content = strings.Join(o.lines, "\n")
	if o.follow {
		o.scrollToBottom()
	}
}

// viewportHeight returns the number of visible content lines within the overlay border.
// It accounts for the border (2 rows) and the title bar (1 row).
func (o *OverlayModel) viewportHeight() int {
	// Total overlay box height minus border top/bottom (2) and title row (1)
	h := o.overlayHeight() - 3
	if h < 1 {
		return 1
	}
	return h
}

// overlayHeight returns the height of the overlay box (capped at 80% of terminal height).
func (o *OverlayModel) overlayHeight() int {
	h := o.height * 80 / 100
	if h < 5 {
		h = 5
	}
	return h
}

// overlayWidth returns the width of the overlay box (capped at 80% of terminal width).
func (o *OverlayModel) overlayWidth() int {
	w := o.width * 80 / 100
	if w < 20 {
		w = 20
	}
	return w
}

// maxScroll returns the maximum scroll offset.
func (o *OverlayModel) maxScroll() int {
	max := len(o.lines) - o.viewportHeight()
	if max < 0 {
		return 0
	}
	return max
}

// clampScroll ensures the scroll position is within valid bounds.
func (o *OverlayModel) clampScroll() {
	max := o.maxScroll()
	if o.scroll > max {
		o.scroll = max
	}
	if o.scroll < 0 {
		o.scroll = 0
	}
}

// scrollToBottom scrolls to the bottom of the content.
func (o *OverlayModel) scrollToBottom() {
	o.scroll = o.maxScroll()
}

// Update handles keyboard input for the overlay. Returns the updated model,
// a command, and whether the overlay should be closed.
func (o *OverlayModel) Update(msg tea.Msg) (*OverlayModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return o, nil, true

		case "up", "k":
			o.scroll--
			o.follow = false
			o.clampScroll()

		case "down", "j":
			o.scroll++
			o.clampScroll()

		case "pgup":
			o.scroll -= o.viewportHeight()
			o.follow = false
			o.clampScroll()

		case "pgdown":
			o.scroll += o.viewportHeight()
			o.clampScroll()

		case "g":
			o.scroll = 0
			o.follow = false

		case "G":
			o.scrollToBottom()

		case "f":
			o.ToggleFollow()
		}
	}

	return o, nil, false
}

// View renders the overlay as a centered bordered box with title and scrollable content.
func (o *OverlayModel) View() string {
	boxWidth := o.overlayWidth()

	// Build the title bar
	titleText := " " + o.title + " "
	if o.follow {
		titleText += "[FOLLOW] "
	}
	if o.loading {
		titleText += "[Loading...] "
	}

	// Inner content width (box width minus border left/right)
	innerWidth := boxWidth - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	// Build visible content
	var contentLines []string
	vpHeight := o.viewportHeight()

	if o.err != nil {
		errMsg := "Error: " + o.err.Error()
		contentLines = []string{errMsg}
	} else if o.loading && len(o.lines) == 0 {
		contentLines = []string{"Loading..."}
	} else {
		// Get the visible slice of lines based on scroll position
		start := o.scroll
		end := start + vpHeight
		if end > len(o.lines) {
			end = len(o.lines)
		}
		if start < len(o.lines) {
			contentLines = o.lines[start:end]
		}
	}

	// Truncate lines to fit inner width and pad to fill viewport
	var renderedLines []string
	for _, line := range contentLines {
		if len(line) > innerWidth {
			line = line[:innerWidth]
		}
		renderedLines = append(renderedLines, line)
	}
	// Pad remaining viewport lines with empty strings
	for len(renderedLines) < vpHeight {
		renderedLines = append(renderedLines, "")
	}

	contentBlock := strings.Join(renderedLines, "\n")

	// Style the overlay box with a rounded border
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(innerWidth).
		Height(vpHeight + 1). // +1 for title row
		Padding(0, 1)

	// Build title line (padded to fill width)
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62"))

	titleLine := titleStyle.Render(titleText)

	// Combine title and content
	body := titleLine + "\n" + contentBlock

	box := borderStyle.Render(body)

	// Center the box in the terminal
	boxRenderedHeight := lipgloss.Height(box)
	boxRenderedWidth := lipgloss.Width(box)

	// Calculate padding for centering
	padTop := (o.height - boxRenderedHeight) / 2
	padLeft := (o.width - boxRenderedWidth) / 2

	if padTop < 0 {
		padTop = 0
	}
	if padLeft < 0 {
		padLeft = 0
	}

	// Build centered output
	var result strings.Builder
	// Add top padding
	for i := 0; i < padTop; i++ {
		result.WriteString("\n")
	}
	// Add each line of the box with left padding
	leftPad := strings.Repeat(" ", padLeft)
	for i, line := range strings.Split(box, "\n") {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(leftPad)
		result.WriteString(line)
	}

	return result.String()
}
