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
	"golang.org/x/term"
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

// connectRuntime resolves and connects to a container runtime for an
// interactive command, prompting to choose when several runtimes are available.
func connectRuntime(ctx context.Context) (runtime.Runtime, error) {
	return connectRuntimeInteractive(ctx, true)
}

// connectRuntimeInteractive resolves the runtime endpoint (see resolveRuntime)
// and connects to it. When interactive is false the runtime selector is never
// shown — used by shell completion, which must not block on a prompt.
func connectRuntimeInteractive(_ context.Context, interactive bool) (runtime.Runtime, error) {
	sock, rtType, err := resolveRuntime(interactive)
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

// resolveRuntime picks which runtime endpoint to connect to, applying the
// selection priority:
//  1. an explicit --socket or --runtime
//  2. the sole auto-detected runtime
//  3. an interactive selector when several are detected on a TTY
//
// A non-interactive caller with several runtimes falls back to the
// highest-priority one rather than prompting.
func resolveRuntime(interactive bool) (string, runtime.RuntimeType, error) {
	// Explicit selection always wins.
	if socketPath != "" || runtime.RuntimeType(runtimeType) != runtime.RuntimeAuto {
		return runtime.DetectSocket(socketPath, runtime.RuntimeType(runtimeType))
	}

	detected := runtime.DetectAllRuntimes()
	switch len(detected) {
	case 0:
		// Reuse DetectSocket's descriptive "not found" error.
		return runtime.DetectSocket("", runtime.RuntimeAuto)
	case 1:
		return detected[0].Endpoint, detected[0].RuntimeType, nil
	}

	if interactive && term.IsTerminal(int(os.Stdin.Fd())) {
		choice, err := picker.PickRuntime(detected)
		if err != nil {
			return "", "", err
		}
		return choice.Endpoint, choice.RuntimeType, nil
	}

	// Ambiguous but non-interactive: use the highest-priority runtime.
	return detected[0].Endpoint, detected[0].RuntimeType, nil
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
	rt, err := connectRuntimeInteractive(ctx, false)
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
