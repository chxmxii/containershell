package dashboard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/tui"
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

// viewportHeight returns the number of visible content lines within the
// overlay border. The title lives in the border itself, so only the top and
// bottom border rows are subtracted.
func (o *OverlayModel) viewportHeight() int {
	h := o.overlayHeight() - 2
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

// View renders the overlay as a centered bordered box. The title (with FOLLOW
// and loading badges) is embedded in the top border and the scroll position in
// the bottom border.
func (o *OverlayModel) View() string {
	boxWidth := o.overlayWidth()
	boxHeight := o.overlayHeight()
	vpHeight := o.viewportHeight()

	title := o.title
	if o.follow {
		title += " · FOLLOW"
	}
	if o.loading {
		title += " · Loading…"
	}

	// Build visible content
	var contentLines []string
	switch {
	case o.err != nil:
		contentLines = []string{tui.ErrStyle.Render("✗ " + o.err.Error())}
	case o.loading && len(o.lines) == 0:
		contentLines = []string{tui.DimStyle.Render("… loading")}
	default:
		start := o.scroll
		end := min(start+vpHeight, len(o.lines))
		if start < len(o.lines) {
			contentLines = o.lines[start:end]
		}
	}

	// Indent content one column from the border; Panel clips each line.
	// Build a fresh slice: contentLines may alias o.lines, and mutating it in
	// place would prepend another space on every render, walking the text out
	// of the box.
	indented := make([]string, len(contentLines))
	for i, line := range contentLines {
		indented[i] = " " + line
	}

	box := tui.Panel(boxWidth, boxHeight, title, o.scrollIndicator(), true,
		strings.Join(indented, "\n"))

	return centerBox(box, o.width, o.height)
}

// scrollIndicator reports the scroll position for the bottom border:
// empty when everything fits, otherwise "top", "NN%", or "bot".
func (o *OverlayModel) scrollIndicator() string {
	max := o.maxScroll()
	switch {
	case max == 0:
		return ""
	case o.scroll == 0:
		return "top"
	case o.scroll >= max:
		return "bot"
	default:
		return fmt.Sprintf("%d%%", o.scroll*100/max)
	}
}

// centerBox pads a rendered box with blank lines and left spacing so it sits
// centered in a width×height terminal.
func centerBox(box string, width, height int) string {
	lines := strings.Split(box, "\n")

	padTop := (height - len(lines)) / 2
	padLeft := (width - lipgloss.Width(box)) / 2
	if padTop < 0 {
		padTop = 0
	}
	if padLeft < 0 {
		padLeft = 0
	}

	var result strings.Builder
	result.WriteString(strings.Repeat("\n", padTop))
	leftPad := strings.Repeat(" ", padLeft)
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(leftPad)
		result.WriteString(line)
	}
	return result.String()
}
