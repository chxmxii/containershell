package tui

import (
	"os"
	"path/filepath"
	"strings"
)

// themePrefPath returns the path of the persisted theme preference file
// (typically ~/.config/containershell/theme).
func themePrefPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "containershell", "theme"), nil
}

// SaveThemePref persists name as the user's theme preference.
func SaveThemePref(name string) error {
	path, err := themePrefPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(name+"\n"), 0o644)
}

// LoadThemePref returns the persisted theme name, or "" when none is saved.
func LoadThemePref() string {
	path, err := themePrefPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
