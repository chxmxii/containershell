package dashboard

import "testing"

func TestDefaultLayoutConfig(t *testing.T) {
	cfg := DefaultLayoutConfig()

	if cfg.MinWidth != 80 {
		t.Errorf("MinWidth = %d, want 80", cfg.MinWidth)
	}
	if cfg.MinHeight != 24 {
		t.Errorf("MinHeight = %d, want 24", cfg.MinHeight)
	}
	if cfg.ListWidthPct != 0.35 {
		t.Errorf("ListWidthPct = %f, want 0.35", cfg.ListWidthPct)
	}
	if cfg.MinListWidthPct != 0.30 {
		t.Errorf("MinListWidthPct = %f, want 0.30", cfg.MinListWidthPct)
	}
	if cfg.MaxListWidthPct != 0.50 {
		t.Errorf("MaxListWidthPct = %f, want 0.50", cfg.MaxListWidthPct)
	}
	if cfg.StatusBarHeight != 1 {
		t.Errorf("StatusBarHeight = %d, want 1", cfg.StatusBarHeight)
	}
	if cfg.ActionBarHeight != 1 {
		t.Errorf("ActionBarHeight = %d, want 1", cfg.ActionBarHeight)
	}
}

func TestComputeLayout_NormalMode(t *testing.T) {
	cfg := DefaultLayoutConfig()
	l := ComputeLayout(120, 40, cfg)

	if l.Mode != LayoutNormal {
		t.Errorf("Mode = %d, want LayoutNormal (%d)", l.Mode, LayoutNormal)
	}
	if !l.ShowActionBar {
		t.Error("ShowActionBar = false, want true")
	}
	if l.ListWidth+l.DetailWidth != 120 {
		t.Errorf("ListWidth(%d) + DetailWidth(%d) = %d, want 120", l.ListWidth, l.DetailWidth, l.ListWidth+l.DetailWidth)
	}
	// ListWidth should be 35% of 120 = 42, which is within [30%, 50%] = [36, 60]
	if l.ListWidth < 36 || l.ListWidth > 60 {
		t.Errorf("ListWidth = %d, want in range [36, 60]", l.ListWidth)
	}
	// ContentHeight = 40 - 1 (status) - 1 (action) = 38
	if l.ContentHeight != 38 {
		t.Errorf("ContentHeight = %d, want 38", l.ContentHeight)
	}
}

func TestComputeLayout_StackedMode(t *testing.T) {
	cfg := DefaultLayoutConfig()
	// Width < 80, height >= 24 => Stacked
	l := ComputeLayout(60, 30, cfg)

	if l.Mode != LayoutStacked {
		t.Errorf("Mode = %d, want LayoutStacked (%d)", l.Mode, LayoutStacked)
	}
	if l.ListWidth != 60 {
		t.Errorf("ListWidth = %d, want 60", l.ListWidth)
	}
	if l.DetailWidth != 0 {
		t.Errorf("DetailWidth = %d, want 0", l.DetailWidth)
	}
	if !l.ShowActionBar {
		t.Error("ShowActionBar = false, want true")
	}
}

func TestComputeLayout_TooSmallMode(t *testing.T) {
	cfg := DefaultLayoutConfig()
	// Width < 80 AND height < 24 => TooSmall
	l := ComputeLayout(50, 15, cfg)

	if l.Mode != LayoutTooSmall {
		t.Errorf("Mode = %d, want LayoutTooSmall (%d)", l.Mode, LayoutTooSmall)
	}
	if l.ListWidth != 50 {
		t.Errorf("ListWidth = %d, want 50", l.ListWidth)
	}
	if l.DetailWidth != 0 {
		t.Errorf("DetailWidth = %d, want 0", l.DetailWidth)
	}
}

func TestComputeLayout_HideActionBar(t *testing.T) {
	cfg := DefaultLayoutConfig()
	// Height < 10 => hide action bar
	l := ComputeLayout(120, 8, cfg)

	if l.ShowActionBar {
		t.Error("ShowActionBar = true, want false for height < 10")
	}
	// ContentHeight = 8 - 1 (status) = 7
	if l.ContentHeight != 7 {
		t.Errorf("ContentHeight = %d, want 7", l.ContentHeight)
	}
}

func TestComputeLayout_ShowActionBarAtThreshold(t *testing.T) {
	cfg := DefaultLayoutConfig()
	// Height == 10 => show action bar
	l := ComputeLayout(120, 10, cfg)

	if !l.ShowActionBar {
		t.Error("ShowActionBar = false, want true for height == 10")
	}
	// ContentHeight = 10 - 1 (status) - 1 (action) = 8
	if l.ContentHeight != 8 {
		t.Errorf("ContentHeight = %d, want 8", l.ContentHeight)
	}
}

func TestComputeLayout_ListWidthClamping(t *testing.T) {
	cfg := DefaultLayoutConfig()
	// Set ListWidthPct very low to test min clamping
	cfg.ListWidthPct = 0.10
	l := ComputeLayout(100, 30, cfg)

	minExpected := int(float64(100) * cfg.MinListWidthPct) // 30
	if l.ListWidth < minExpected {
		t.Errorf("ListWidth = %d, want >= %d (MinListWidthPct)", l.ListWidth, minExpected)
	}

	// Set ListWidthPct very high to test max clamping
	cfg.ListWidthPct = 0.80
	l = ComputeLayout(100, 30, cfg)

	maxExpected := int(float64(100) * cfg.MaxListWidthPct) // 50
	if l.ListWidth > maxExpected {
		t.Errorf("ListWidth = %d, want <= %d (MaxListWidthPct)", l.ListWidth, maxExpected)
	}
}

func TestComputeLayout_ZeroHeight(t *testing.T) {
	cfg := DefaultLayoutConfig()
	l := ComputeLayout(100, 0, cfg)

	if l.ContentHeight != 0 {
		t.Errorf("ContentHeight = %d, want 0 for zero height", l.ContentHeight)
	}
}

func TestComputeLayout_ExactMinWidth(t *testing.T) {
	cfg := DefaultLayoutConfig()
	// Width == 80 (exactly MinWidth) => Normal mode
	l := ComputeLayout(80, 30, cfg)

	if l.Mode != LayoutNormal {
		t.Errorf("Mode = %d, want LayoutNormal (%d) at exactly MinWidth", l.Mode, LayoutNormal)
	}
	if l.ListWidth+l.DetailWidth != 80 {
		t.Errorf("ListWidth(%d) + DetailWidth(%d) = %d, want 80", l.ListWidth, l.DetailWidth, l.ListWidth+l.DetailWidth)
	}
}
