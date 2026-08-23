// Package tui provides the shared look and feel for containershell's
// terminal interfaces: switchable adaptive color themes, reusable styles,
// text formatting helpers, and a bordered panel renderer with embedded
// titles.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a complete named color palette. Every color adapts to light and
// dark terminal backgrounds.
type Theme struct {
	Name string

	Accent   lipgloss.AdaptiveColor
	Blue     lipgloss.AdaptiveColor
	Green    lipgloss.AdaptiveColor
	Yellow   lipgloss.AdaptiveColor
	Red      lipgloss.AdaptiveColor
	Dim      lipgloss.AdaptiveColor
	Border   lipgloss.AdaptiveColor
	Surface  lipgloss.AdaptiveColor // selection / badge background
	OnAccent lipgloss.AdaptiveColor // text on accent backgrounds
}

// themes is the built-in theme registry; the first entry is the default.
var themes = []Theme{
	{
		Name:     "catppuccin",
		Accent:   lipgloss.AdaptiveColor{Light: "#8839EF", Dark: "#CBA6F7"},
		Blue:     lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"},
		Green:    lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"},
		Yellow:   lipgloss.AdaptiveColor{Light: "#DF8E1D", Dark: "#F9E2AF"},
		Red:      lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#F38BA8"},
		Dim:      lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#6C7086"},
		Border:   lipgloss.AdaptiveColor{Light: "#ACB0BE", Dark: "#45475A"},
		Surface:  lipgloss.AdaptiveColor{Light: "#DCE0E8", Dark: "#313244"},
		OnAccent: lipgloss.AdaptiveColor{Light: "#EFF1F5", Dark: "#11111B"},
	},
	{
		Name:     "dracula",
		Accent:   lipgloss.AdaptiveColor{Light: "#644AC9", Dark: "#BD93F9"},
		Blue:     lipgloss.AdaptiveColor{Light: "#0E7490", Dark: "#8BE9FD"},
		Green:    lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#50FA7B"},
		Yellow:   lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#F1FA8C"},
		Red:      lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#FF5555"},
		Dim:      lipgloss.AdaptiveColor{Light: "#8A8F98", Dark: "#6272A4"},
		Border:   lipgloss.AdaptiveColor{Light: "#C0C4CC", Dark: "#44475A"},
		Surface:  lipgloss.AdaptiveColor{Light: "#E5E7EB", Dark: "#44475A"},
		OnAccent: lipgloss.AdaptiveColor{Light: "#F8F8F2", Dark: "#282A36"},
	},
	{
		Name:     "nord",
		Accent:   lipgloss.AdaptiveColor{Light: "#5E81AC", Dark: "#88C0D0"},
		Blue:     lipgloss.AdaptiveColor{Light: "#4C6A92", Dark: "#81A1C1"},
		Green:    lipgloss.AdaptiveColor{Light: "#4E7A3F", Dark: "#A3BE8C"},
		Yellow:   lipgloss.AdaptiveColor{Light: "#B08500", Dark: "#EBCB8B"},
		Red:      lipgloss.AdaptiveColor{Light: "#A0343F", Dark: "#BF616A"},
		Dim:      lipgloss.AdaptiveColor{Light: "#7B88A1", Dark: "#616E88"},
		Border:   lipgloss.AdaptiveColor{Light: "#C2C9D6", Dark: "#3B4252"},
		Surface:  lipgloss.AdaptiveColor{Light: "#D8DEE9", Dark: "#434C5E"},
		OnAccent: lipgloss.AdaptiveColor{Light: "#ECEFF4", Dark: "#2E3440"},
	},
	{
		Name:     "gruvbox",
		Accent:   lipgloss.AdaptiveColor{Light: "#AF3A03", Dark: "#FE8019"},
		Blue:     lipgloss.AdaptiveColor{Light: "#076678", Dark: "#83A598"},
		Green:    lipgloss.AdaptiveColor{Light: "#79740E", Dark: "#B8BB26"},
		Yellow:   lipgloss.AdaptiveColor{Light: "#B57614", Dark: "#FABD2F"},
		Red:      lipgloss.AdaptiveColor{Light: "#9D0006", Dark: "#FB4934"},
		Dim:      lipgloss.AdaptiveColor{Light: "#928374", Dark: "#928374"},
		Border:   lipgloss.AdaptiveColor{Light: "#BDAE93", Dark: "#504945"},
		Surface:  lipgloss.AdaptiveColor{Light: "#EBDBB2", Dark: "#3C3836"},
		OnAccent: lipgloss.AdaptiveColor{Light: "#FBF1C7", Dark: "#282828"},
	},
	{
		Name:     "tokyo-night",
		Accent:   lipgloss.AdaptiveColor{Light: "#7847BD", Dark: "#BB9AF7"},
		Blue:     lipgloss.AdaptiveColor{Light: "#2E7DE9", Dark: "#7AA2F7"},
		Green:    lipgloss.AdaptiveColor{Light: "#587539", Dark: "#9ECE6A"},
		Yellow:   lipgloss.AdaptiveColor{Light: "#8C6C3E", Dark: "#E0AF68"},
		Red:      lipgloss.AdaptiveColor{Light: "#F52A65", Dark: "#F7768E"},
		Dim:      lipgloss.AdaptiveColor{Light: "#848CB5", Dark: "#565F89"},
		Border:   lipgloss.AdaptiveColor{Light: "#A8AECB", Dark: "#3B4261"},
		Surface:  lipgloss.AdaptiveColor{Light: "#C4C8DA", Dark: "#292E42"},
		OnAccent: lipgloss.AdaptiveColor{Light: "#E9E9EC", Dark: "#1A1B26"},
	},
}

// Palette. These variables always reflect the active theme; Apply rewrites
// them together with the derived styles below, so never cache either across
// a theme change.
var (
	ColorAccent   lipgloss.AdaptiveColor
	ColorBlue     lipgloss.AdaptiveColor
	ColorGreen    lipgloss.AdaptiveColor
	ColorYellow   lipgloss.AdaptiveColor
	ColorRed      lipgloss.AdaptiveColor
	ColorDim      lipgloss.AdaptiveColor
	ColorBorder   lipgloss.AdaptiveColor
	ColorSurface  lipgloss.AdaptiveColor
	ColorOnAccent lipgloss.AdaptiveColor
)

// Shared styles, derived from the active theme by Apply.
var (
	TitleStyle     lipgloss.Style
	HeaderStyle    lipgloss.Style
	DimStyle       lipgloss.Style
	ErrStyle       lipgloss.Style
	KeyStyle       lipgloss.Style
	SelectedStyle  lipgloss.Style
	BadgeStyle     lipgloss.Style
	FilterStyle    lipgloss.Style
	ColHeadStyle   lipgloss.Style
	TabActiveStyle lipgloss.Style
)

var current Theme

func init() { Apply(themes[0]) }

// Apply makes t the active theme, rewriting every palette color and style.
func Apply(t Theme) {
	current = t

	ColorAccent = t.Accent
	ColorBlue = t.Blue
	ColorGreen = t.Green
	ColorYellow = t.Yellow
	ColorRed = t.Red
	ColorDim = t.Dim
	ColorBorder = t.Border
	ColorSurface = t.Surface
	ColorOnAccent = t.OnAccent

	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	HeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorBlue)
	DimStyle = lipgloss.NewStyle().Foreground(ColorDim)
	ErrStyle = lipgloss.NewStyle().Foreground(ColorRed)
	KeyStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	SelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Background(ColorSurface)
	BadgeStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorOnAccent).Background(ColorAccent)
	FilterStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
	ColHeadStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorDim)
	TabActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorOnAccent).Background(ColorAccent)
}

// ApplyByName activates the named theme (case-insensitive) and reports
// whether the name matched a built-in theme.
func ApplyByName(name string) bool {
	for _, t := range themes {
		if strings.EqualFold(t.Name, name) {
			Apply(t)
			return true
		}
	}
	return false
}

// CurrentTheme returns the active theme.
func CurrentTheme() Theme { return current }

// Themes returns the built-in themes in display order.
func Themes() []Theme { return append([]Theme(nil), themes...) }

// ThemeNames returns the built-in theme names in display order.
func ThemeNames() []string {
	names := make([]string, len(themes))
	for i, t := range themes {
		names[i] = t.Name
	}
	return names
}

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
