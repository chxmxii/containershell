package main

import (
	"context"
	"fmt"
	"os"

	"github.com/containershell/containershell/pkg/picker"
	"github.com/containershell/containershell/pkg/runtime"
	cri_rt "github.com/containershell/containershell/pkg/runtime/cri"
	docker_rt "github.com/containershell/containershell/pkg/runtime/docker"
	"github.com/containershell/containershell/pkg/shell"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Get an interactive shell in a container (3-tier fallback)",
	Long: `Attempts to get a shell into the target container using:
  Tier 1: CRI exec (if container has a shell binary)
  Tier 2: Debug container injection (K8s ephemeral or CRI-level)
  Tier 3: nsenter (direct namespace entry from host)`,
	RunE:              runShell,
	ValidArgsFunction: completeContainerIDs,
}

func init() {
	rootCmd.AddCommand(shellCmd)
	// Make shell the default command when no subcommand is given
	rootCmd.RunE = runShell
}

func connectRuntime(ctx context.Context) (runtime.Runtime, error) {
	sock, rtType, err := runtime.DetectSocket(socketPath, runtime.RuntimeType(runtimeType))
	if err != nil {
		return nil, err
	}

	switch rtType {
	case runtime.RuntimeCRI:
		return cri_rt.New(sock)
	case runtime.RuntimeDocker, runtime.RuntimePodman:
		name := "docker"
		if rtType == runtime.RuntimePodman {
			name = "podman"
		}
		return docker_rt.New(sock, name)
	default:
		return nil, fmt.Errorf("unknown runtime type: %s", rtType)
	}
}

func runShell(cmd *cobra.Command, args []string) error {
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
		fmt.Fprintf(os.Stderr, "Target: %s (pod=%s, ns=%s, id=%s)\n",
			container.Name, container.PodName, container.Namespace, container.ID)
	}

	logf := func(format string, args ...any) {
		if verbose {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	return shell.FallbackChain(ctx, rt, container, shell.DefaultStrategies(), verbose, logf)
}

// resolveOrPick resolves the target container from flags, or launches the interactive picker.
func resolveOrPick(ctx context.Context, rt runtime.Runtime) (*runtime.ContainerInfo, error) {
	if containerID != "" || podName != "" || namespace != "" || ctrName != "" {
		return rt.Resolve(ctx, runtime.ResolveOptions{
			ContainerID: containerID,
			Name:        ctrName,
			PodName:     podName,
			Namespace:   namespace,
		})
	}

	containers, err := rt.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	return picker.Pick(containers)
}

// completeContainerIDs provides dynamic shell completions for container IDs.
func completeContainerIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := context.Background()
	rt, err := connectRuntime(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer rt.Close()

	containers, err := rt.ListContainers(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, c := range containers {
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}
		desc := c.Name
		if c.PodName != "" {
			desc = fmt.Sprintf("%s (pod=%s, ns=%s)", c.Name, c.PodName, c.Namespace)
		}
		completions = append(completions, fmt.Sprintf("%s\t%s", id, desc))
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
