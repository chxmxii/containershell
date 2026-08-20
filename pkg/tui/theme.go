// Package tui provides the shared look and feel for containershell's
// terminal interfaces: an adaptive color palette, reusable styles, text
// formatting helpers, and a bordered panel renderer with embedded titles.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette. Every color adapts to light and dark terminal backgrounds.
var (
	ColorAccent   = lipgloss.AdaptiveColor{Light: "#8839EF", Dark: "#CBA6F7"} // mauve
	ColorBlue     = lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"}
	ColorGreen    = lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"}
	ColorYellow   = lipgloss.AdaptiveColor{Light: "#DF8E1D", Dark: "#F9E2AF"}
	ColorRed      = lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#F38BA8"}
	ColorDim      = lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#6C7086"}
	ColorBorder   = lipgloss.AdaptiveColor{Light: "#ACB0BE", Dark: "#45475A"}
	ColorSurface  = lipgloss.AdaptiveColor{Light: "#DCE0E8", Dark: "#313244"} // selection / badge background
	ColorOnAccent = lipgloss.AdaptiveColor{Light: "#EFF1F5", Dark: "#11111B"} // text on accent backgrounds
)

// Shared styles.
var (
	TitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	HeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(ColorBlue)
	DimStyle      = lipgloss.NewStyle().Foreground(ColorDim)
	ErrStyle      = lipgloss.NewStyle().Foreground(ColorRed)
	KeyStyle      = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	SelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Background(ColorSurface)
	BadgeStyle    = lipgloss.NewStyle().Bold(true).Foreground(ColorOnAccent).Background(ColorAccent)
)

// Logo is the product glyph shown in badges and headers.
const Logo = "⬢"

// StateColor maps a container state to its indicator color.
func StateColor(state string) lipgloss.TerminalColor {
	switch strings.ToLower(state) {
	case "running", "up":
		return ColorGreen
	case "created", "paused", "restarting":
		return ColorYellow
	case "exited", "stopped", "dead":
		return ColorRed
	default:
		return ColorDim
	}
}

// StateDot renders a colored "● state" label.
func StateDot(state string) string {
	return lipgloss.NewStyle().Foreground(StateColor(state)).Render("● " + state)
}
