package main

import (
	"github.com/spf13/cobra"
)

var (
	socketPath  string
	runtimeType string
	containerID string
	podName     string
	namespace   string
	ctrName     string
	verbose     bool
)

var rootCmd = &cobra.Command{
	Use:   "containershell",
	Short: "Swiss Army knife for container debugging",
	Long: `ContainerShell guarantees shell access to any running container using a
3-tier fallback strategy (CRI exec -> debug container injection -> nsenter),
plus a full debugging toolkit.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&socketPath, "socket", "s", "", "Container runtime socket path or endpoint URL (auto-detected if not set)")
	rootCmd.PersistentFlags().StringVar(&socketPath, "cri-socket", "", "Deprecated: use --socket")
	rootCmd.PersistentFlags().MarkDeprecated("cri-socket", "use --socket instead")
	rootCmd.PersistentFlags().StringVarP(&runtimeType, "runtime", "r", "auto", "Container runtime: auto, cri, docker, podman")
	rootCmd.PersistentFlags().StringVar(&containerID, "container-id", "", "Target container ID")
	rootCmd.PersistentFlags().StringVar(&podName, "pod", "", "Target pod name (K8s-aware lookup)")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Target namespace (K8s-aware lookup)")
	rootCmd.PersistentFlags().StringVarP(&ctrName, "name", "n", "", "Target container name")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed fallback chain output")

	rootCmd.RegisterFlagCompletionFunc("socket", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"/run/containerd/containerd.sock\tcontainerd",
			"/var/run/crio/crio.sock\tCRI-O",
			"/var/run/docker.sock\tDocker",
			"/run/podman/podman.sock\tPodman",
		}, cobra.ShellCompDirectiveNoFileComp
	})
}
