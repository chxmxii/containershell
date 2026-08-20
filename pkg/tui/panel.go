package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Panel renders content inside a rounded border of exactly width×height cells.
// The title is embedded in the top border (╭─ Title ────╮) and the footer, if
// any, is embedded right-aligned in the bottom border (╰──── footer ─╯).
// Focused panels get an accent border; unfocused panels a muted one.
// Content lines are clipped to the inner width and the panel is padded to the
// full height, so the result never overflows the given box.
func Panel(width, height int, title, footer string, focused bool, content string) string {
	if width < 2 || height < 2 {
		return ""
	}
	inner := width - 2

	borderColor := ColorBorder
	titleStyle := DimStyle.Bold(true)
	if focused {
		borderColor = ColorAccent
		titleStyle = TitleStyle
	}
	edge := lipgloss.NewStyle().Foreground(borderColor)

	var b strings.Builder
	b.WriteString(topEdge(inner, title, edge, titleStyle))
	b.WriteString("\n")

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for row := 0; row < height-2; row++ {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		b.WriteString(edge.Render("│"))
		b.WriteString(FitCol(line, inner))
		b.WriteString(edge.Render("│"))
		b.WriteString("\n")
	}

	b.WriteString(bottomEdge(inner, footer, edge))
	return b.String()
}

// topEdge builds "╭─ title ────╮" clipped to inner columns between the corners.
func topEdge(inner int, title string, edge, titleStyle lipgloss.Style) string {
	if title != "" {
		// "─ " + title + " " needs 3 columns beyond the title itself.
		t := ansi.Truncate(title, max(0, inner-3), "…")
		if tw := ansi.StringWidth(t); tw > 0 && inner-tw-3 >= 0 {
			return edge.Render("╭─ ") + titleStyle.Render(t) +
				edge.Render(" "+strings.Repeat("─", inner-tw-3)+"╮")
		}
	}
	return edge.Render("╭" + strings.Repeat("─", inner) + "╮")
}

// bottomEdge builds "╰──── footer ─╯" with the footer right-aligned.
func bottomEdge(inner int, footer string, edge lipgloss.Style) string {
	if footer != "" {
		f := ansi.Truncate(footer, max(0, inner-3), "…")
		if fw := ansi.StringWidth(f); fw > 0 && inner-fw-3 >= 0 {
			return edge.Render("╰"+strings.Repeat("─", inner-fw-3)+" ") + DimStyle.Render(f) +
				edge.Render(" ─╯")
		}
	}
	return edge.Render("╰" + strings.Repeat("─", inner) + "╯")
}
