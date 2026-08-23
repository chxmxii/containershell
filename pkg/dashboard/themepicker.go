package dashboard

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/tui"
)

// ThemePickerModel is a centered modal for choosing a color theme. Moving the
// cursor previews the theme live; Enter keeps it (and persists the choice),
// Esc restores the theme that was active when the picker opened.
type ThemePickerModel struct {
	themes   []tui.Theme
	cursor   int
	original tui.Theme
	width    int
	height   int
}

// NewThemePickerModel opens a theme picker with the cursor on the active theme.
func NewThemePickerModel(width, height int) *ThemePickerModel {
	p := &ThemePickerModel{
		themes:   tui.Themes(),
		original: tui.CurrentTheme(),
		width:    width,
		height:   height,
	}
	for i, t := range p.themes {
		if t.Name == p.original.Name {
			p.cursor = i
			break
		}
	}
	return p
}

// SetDimensions updates the terminal dimensions used for centering.
func (p *ThemePickerModel) SetDimensions(width, height int) {
	p.width = width
	p.height = height
}

// Update handles keyboard input. It reports whether the picker should close.
func (p *ThemePickerModel) Update(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
			tui.Apply(p.themes[p.cursor])
		}
	case "down", "j":
		if p.cursor < len(p.themes)-1 {
			p.cursor++
			tui.Apply(p.themes[p.cursor])
		}
	case "enter":
		// Persisting the preference is best-effort; the theme is applied
		// for this session either way.
		_ = tui.SaveThemePref(p.themes[p.cursor].Name)
		return true
	case "esc", "q", "T":
		tui.Apply(p.original)
		return true
	}
	return false
}

// View renders the picker as a centered bordered box with one row per theme,
// each showing a swatch of that theme's own palette.
func (p *ThemePickerModel) View() string {
	var rows []string
	for i, t := range p.themes {
		swatch := themeSwatch(t)
		if i == p.cursor {
			rows = append(rows, tui.SelectedStyle.Render(" ▸ "+tui.FitCol(t.Name, 14))+" "+swatch)
		} else {
			rows = append(rows, "   "+tui.FitCol(t.Name, 14)+" "+swatch)
		}
	}

	boxWidth := 32
	if boxWidth > p.width {
		boxWidth = p.width
	}
	boxHeight := len(rows) + 2

	box := tui.Panel(boxWidth, boxHeight, "Theme", "↵ apply · esc cancel", true,
		strings.Join(rows, "\n"))
	return centerBox(box, p.width, p.height)
}

// themeSwatch renders sample dots in a theme's own colors, so every row
// previews its palette regardless of which theme is currently applied.
func themeSwatch(t tui.Theme) string {
	colors := []lipgloss.AdaptiveColor{t.Accent, t.Blue, t.Green, t.Yellow, t.Red}
	dots := make([]string, len(colors))
	for i, c := range colors {
		dots[i] = lipgloss.NewStyle().Foreground(c).Render("●")
	}
	return strings.Join(dots, " ")
}
