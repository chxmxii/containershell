package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// FormatAge formats a duration as a human-readable age string.
// Rules: <1min → "Xs", <1hr → "Xm", <24h → "XhYm", >=24h → "XdYh"
func FormatAge(d time.Duration) string {
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

// sanitize strips carriage returns and expands tabs to 8-column stops so raw
// command output (inspect, top, netstat, logs) cannot move the cursor and
// escape a panel border when rendered.
func sanitize(s string) string {
	if !strings.ContainsAny(s, "\t\r") {
		return s
	}
	s = strings.ReplaceAll(s, "\r", "")
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	parts := strings.Split(s, "\t")
	for i, part := range parts {
		b.WriteString(part)
		col += ansi.StringWidth(part)
		if i < len(parts)-1 {
			pad := 8 - col%8
			b.WriteString(strings.Repeat(" ", pad))
			col += pad
		}
	}
	return b.String()
}

// FitCol pads or truncates s to exactly w display columns, accounting for wide
// runes and any ANSI escape sequences.
func FitCol(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(sanitize(s), w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// ClipLine truncates s to at most w display columns to prevent line wrapping.
// A non-positive width returns s unchanged (the width is not yet known).
func ClipLine(s string, w int) string {
	s = sanitize(s)
	if w <= 0 {
		return s
	}
	return ansi.Truncate(s, w, "")
}
