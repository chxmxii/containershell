package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/containershell/containershell/pkg/dashboard"
	"github.com/containershell/containershell/pkg/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	refreshInterval time.Duration
	themeName       string
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Launch the interactive TUI dashboard",
	Long: `Launch a full-screen interactive dashboard for browsing containers,
viewing details, launching shells, and running debug commands.

The dashboard supports all container runtimes (CRI, Docker, Podman) and
provides the same 3-tier shell fallback strategy as the shell command.`,
	Aliases: []string{"ui", "tui"},
	RunE:    runDashboard,
}

func init() {
	dashboardCmd.Flags().DurationVar(&refreshInterval, "refresh", 5*time.Second,
		"Container list refresh interval (range: 1s-60s)")
	dashboardCmd.Flags().StringVar(&themeName, "theme", "",
		"Color theme: "+strings.Join(tui.ThemeNames(), ", ")+" (default: saved preference; press T in the dashboard to switch)")
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	// Validate refresh interval
	if refreshInterval < time.Second || refreshInterval > 60*time.Second {
		return fmt.Errorf("refresh interval must be between 1s and 60s, got %s", refreshInterval)
	}

	// Apply the theme flag (overrides any saved preference)
	if themeName != "" && !tui.ApplyByName(themeName) {
		return fmt.Errorf("unknown theme %q (available: %s)", themeName, strings.Join(tui.ThemeNames(), ", "))
	}

	// Validate TTY
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("dashboard requires an interactive terminal (TTY)")
	}

	// Connect to runtime
	ctx := context.Background()
	rt, err := connectRuntime(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to container runtime: %w", err)
	}
	defer rt.Close()

	// Get runtime info for status bar
	rtInfo, err := rt.RuntimeInfo(ctx)
	if err != nil {
		// Non-fatal: dashboard can work without runtime info in status bar
		rtInfo = nil
	}

	// Build config
	cfg := dashboard.Config{
		RefreshInterval: refreshInterval,
		RuntimeType:     runtimeType,
		SocketPath:      socketPath,
		Verbose:         verbose,
	}

	// Create model
	model := dashboard.NewModel(cfg, rt, rtInfo)

	// Launch TUI
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("dashboard error: %w", err)
	}

	return nil
}
