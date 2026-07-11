package main

import (
	"context"
	"fmt"

	"github.com/containershell/containershell/pkg/debug"
	"github.com/containershell/containershell/pkg/runtime"
	"github.com/spf13/cobra"
)

// withContainer is a helper that connects to the runtime and resolves the container for any debug command.
func withContainer(fn func(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		rt, err := connectRuntime(ctx)
		if err != nil {
			return err
		}
		defer rt.Close()

		container, err := resolveOrPick(ctx, rt)
		if err != nil {
			return err
		}

		if verbose {
			fmt.Printf("Target: %s (id=%s)\n", container.Name, container.ID[:12])
		}

		return fn(ctx, rt, container)
	}
}

var (
	logFollow bool
	logTail   int64

	tcpdumpIface  string
	tcpdumpFilter string
	tcpdumpCount  int

	stracePid   int
	straceForks bool

	cpSrc       string
	cpDst       string
	cpDirection string

	portfwLocal  int
	portfwRemote int

	fsPath      string
	fsRecursive bool
	fsPattern   string
)

func init() {
	// logs
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream container logs",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.Logs(ctx, rt, c.ID, logFollow, logTail)
		}),
	}
	logsCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().Int64VarP(&logTail, "tail", "t", 100, "Number of lines from the end")
	rootCmd.AddCommand(logsCmd)

	// inspect
	rootCmd.AddCommand(&cobra.Command{
		Use:   "inspect",
		Short: "Show container metadata, config, mounts, and env",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.Inspect(ctx, rt, c.ID)
		}),
	})

	// top
	rootCmd.AddCommand(&cobra.Command{
		Use:   "top",
		Short: "List processes inside the container",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.Top(ctx, rt, c.ID)
		}),
	})

	// netstat
	rootCmd.AddCommand(&cobra.Command{
		Use:   "netstat",
		Short: "Show network connections and listeners",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.Netstat(ctx, rt, c.ID)
		}),
	})

	// tcpdump
	tcpdumpCmd := &cobra.Command{
		Use:   "tcpdump",
		Short: "Capture packets on the container's network",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.Tcpdump(ctx, rt, c.ID, tcpdumpIface, tcpdumpFilter, tcpdumpCount)
		}),
	}
	tcpdumpCmd.Flags().StringVarP(&tcpdumpIface, "interface", "i", "", "Network interface (default: any)")
	tcpdumpCmd.Flags().StringVarP(&tcpdumpFilter, "filter", "f", "", "BPF filter expression")
	tcpdumpCmd.Flags().IntVarP(&tcpdumpCount, "count", "c", 0, "Stop after N packets")
	rootCmd.AddCommand(tcpdumpCmd)

	// strace
	straceCmd := &cobra.Command{
		Use:   "strace",
		Short: "Trace syscalls of a process in the container",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.Strace(ctx, rt, c.ID, stracePid, straceForks)
		}),
	}
	straceCmd.Flags().IntVarP(&stracePid, "pid", "p", 0, "PID to trace (default: container init)")
	straceCmd.Flags().BoolVarP(&straceForks, "follow-forks", "f", false, "Follow child processes")
	rootCmd.AddCommand(straceCmd)

	// cp
	cpCmd := &cobra.Command{
		Use:   "cp",
		Short: "Copy files in/out of the container",
		Long:  "Copy files between host and container filesystem.\n\nExamples:\n  containershell cp --dir from --src /etc/hosts --dst ./hosts\n  containershell cp --dir to --src ./config.yaml --dst /app/config.yaml",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			if cpDirection == "from" {
				return debug.CopyFromContainer(ctx, rt, c.ID, cpSrc, cpDst)
			}
			return debug.CopyToContainer(ctx, rt, c.ID, cpSrc, cpDst)
		}),
	}
	cpCmd.Flags().StringVar(&cpDirection, "dir", "from", "Direction: 'from' (container->host) or 'to' (host->container)")
	cpCmd.Flags().StringVar(&cpSrc, "src", "", "Source path")
	cpCmd.Flags().StringVar(&cpDst, "dst", "", "Destination path")
	cpCmd.MarkFlagRequired("src")
	cpCmd.MarkFlagRequired("dst")
	rootCmd.AddCommand(cpCmd)

	// portfw
	portfwCmd := &cobra.Command{
		Use:   "portfw",
		Short: "Forward a local port to a port in the container",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.PortForward(ctx, rt, c.ID, portfwLocal, portfwRemote)
		}),
	}
	portfwCmd.Flags().IntVarP(&portfwLocal, "local", "l", 0, "Local port")
	portfwCmd.Flags().IntVarP(&portfwRemote, "remote", "R", 0, "Remote port in container")
	portfwCmd.MarkFlagRequired("local")
	portfwCmd.MarkFlagRequired("remote")
	rootCmd.AddCommand(portfwCmd)

	// env
	rootCmd.AddCommand(&cobra.Command{
		Use:   "env",
		Short: "Dump container environment variables",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.Env(ctx, rt, c.ID)
		}),
	})

	// fs
	fsCmd := &cobra.Command{
		Use:   "fs",
		Short: "Browse/search the container filesystem",
		RunE: withContainer(func(ctx context.Context, rt runtime.Runtime, c *runtime.ContainerInfo) error {
			return debug.FS(ctx, rt, c.ID, fsPath, fsRecursive, fsPattern)
		}),
	}
	fsCmd.Flags().StringVarP(&fsPath, "path", "p", "/", "Path to list")
	fsCmd.Flags().BoolVarP(&fsRecursive, "recursive", "R", false, "Recursive listing")
	fsCmd.Flags().StringVar(&fsPattern, "match", "", "Filename glob pattern to filter")
	rootCmd.AddCommand(fsCmd)
}
