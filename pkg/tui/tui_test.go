package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFitColExpandsTabsAndStripsCR(t *testing.T) {
	got := FitCol("ab\tcd\r", 12)
	if strings.ContainsAny(got, "\t\r") {
		t.Fatalf("control characters survived: %q", got)
	}
	if got != "ab      cd  " {
		t.Fatalf("tab not expanded to 8-column stop: %q", got)
	}
	if w := ansi.StringWidth(got); w != 12 {
		t.Fatalf("expected width 12, got %d (%q)", w, got)
	}
}

func TestClipLineSanitizesWithoutWidth(t *testing.T) {
	got := ClipLine("a\tb\r", 0)
	if strings.ContainsAny(got, "\t\r") {
		t.Fatalf("control characters survived: %q", got)
	}
}

func TestApplyByName(t *testing.T) {
	t.Cleanup(func() { Apply(Themes()[0]) })

	if !ApplyByName("Nord") {
		t.Fatal("expected case-insensitive match for Nord")
	}
	if CurrentTheme().Name != "nord" {
		t.Fatalf("expected nord active, got %s", CurrentTheme().Name)
	}
	if ColorAccent != CurrentTheme().Accent {
		t.Fatal("palette vars not updated by Apply")
	}
	if ApplyByName("no-such-theme") {
		t.Fatal("expected unknown theme to be rejected")
	}
	if CurrentTheme().Name != "nord" {
		t.Fatal("unknown theme must not change the active theme")
	}
}

func TestThemeNamesMatchThemes(t *testing.T) {
	names := ThemeNames()
	themes := Themes()
	if len(names) != len(themes) || len(names) < 2 {
		t.Fatalf("inconsistent registry: %d names, %d themes", len(names), len(themes))
	}
	for i := range themes {
		if names[i] != themes[i].Name {
			t.Fatalf("name order mismatch at %d", i)
		}
	}
}
