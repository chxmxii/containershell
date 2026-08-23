package dashboard

// LayoutMode represents the rendering mode for the dashboard layout.
type LayoutMode int

const (
	// LayoutNormal is a multi-panel layout with list and detail side by side.
	LayoutNormal LayoutMode = iota
	// LayoutStacked is a single-panel layout showing only the list (width < MinWidth).
	LayoutStacked
	// LayoutTooSmall indicates the terminal is too small to render any panel layout.
	LayoutTooSmall
)

// LayoutConfig holds the configuration parameters for computing the dashboard layout.
type LayoutConfig struct {
	MinWidth        int     // Minimum width for multi-panel layout (default: 80)
	MinHeight       int     // Minimum height for full layout (default: 24)
	ListWidthPct    float64 // Default list panel width as fraction of terminal width (default: 0.35)
	MinListWidthPct float64 // Minimum list panel width fraction (default: 0.30)
	MaxListWidthPct float64 // Maximum list panel width fraction (default: 0.50)
	StatusBarHeight int     // Height of the status bar in rows (default: 1)
	ActionBarHeight int     // Height of the action bar in rows (default: 1)
}

// DefaultLayoutConfig returns a LayoutConfig with the standard default values.
func DefaultLayoutConfig() LayoutConfig {
	return LayoutConfig{
		MinWidth:        80,
		MinHeight:       24,
		ListWidthPct:    0.35,
		MinListWidthPct: 0.30,
		MaxListWidthPct: 0.50,
		StatusBarHeight: 1,
		ActionBarHeight: 1,
	}
}

// Layout represents the computed layout dimensions for the dashboard.
type Layout struct {
	Mode          LayoutMode // The layout mode based on terminal dimensions
	ListWidth     int        // Width of the list panel in columns
	DetailWidth   int        // Width of the detail panel in columns
	ContentHeight int        // Available height for panel content
	ShowActionBar bool       // Whether the action bar is visible
}

// ComputeLayout is a pure function that calculates the dashboard layout
// based on terminal dimensions and configuration. It has no side effects.
func ComputeLayout(width, height int, cfg LayoutConfig) Layout {
	var l Layout

	// Determine if action bar should be shown (height >= 10)
	l.ShowActionBar = height >= 10

	// Determine layout mode based on dimensions
	switch {
	case width < cfg.MinWidth && height < cfg.MinHeight:
		// Terminal is too small in both dimensions
		l.Mode = LayoutTooSmall
		l.ListWidth = width
		l.DetailWidth = 0
	case width < cfg.MinWidth:
		// Width too narrow for multi-panel, use stacked (list only)
		l.Mode = LayoutStacked
		l.ListWidth = width
		l.DetailWidth = 0
	default:
		// Normal multi-panel layout
		l.Mode = LayoutNormal
		l.ListWidth = int(float64(width) * cfg.ListWidthPct)

		// Clamp list width to [MinListWidthPct, MaxListWidthPct] of width
		minListWidth := int(float64(width) * cfg.MinListWidthPct)
		maxListWidth := int(float64(width) * cfg.MaxListWidthPct)
		if l.ListWidth < minListWidth {
			l.ListWidth = minListWidth
		}
		if l.ListWidth > maxListWidth {
			l.ListWidth = maxListWidth
		}

		// Detail panel takes the remainder
		l.DetailWidth = width - l.ListWidth
	}

	// Compute content height: subtract status bar and optionally action bar
	l.ContentHeight = height - cfg.StatusBarHeight
	if l.ShowActionBar {
		l.ContentHeight -= cfg.ActionBarHeight
	}

	// Ensure content height is non-negative
	if l.ContentHeight < 0 {
		l.ContentHeight = 0
	}

	return l
}
