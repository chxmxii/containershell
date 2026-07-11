package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("containershell %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// Also expose version as a root flag: `containershell --version` / `-V`.
	// Pre-registering the flag with the -V shorthand stops Cobra from adding
	// its own default (which would try to claim -v, already used by --verbose).
	rootCmd.Version = Version
	rootCmd.Flags().BoolP("version", "V", false, "Print version information")
	rootCmd.SetVersionTemplate(fmt.Sprintf("containershell %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate))
}
