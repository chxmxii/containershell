package shell

import (
	"context"
	"fmt"
	"os"

	"github.com/containershell/containershell/pkg/runtime"
	"github.com/containershell/containershell/pkg/namespace"
)

// NsenterStrategy enters the container's namespaces directly using nsenter.
type NsenterStrategy struct{}

func (s *NsenterStrategy) Name() string { return "nsenter" }

func (s *NsenterStrategy) Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error {
	// Ensure nsenter is available
	nsenterPath, err := namespace.FindNsenter()
	if err != nil {
		return fmt.Errorf("nsenter not available: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "  nsenter found at: %s\n", nsenterPath)
	}

	// Get the container's host PID
	pid, err := rt.ContainerPid(ctx, container.ID)
	if err != nil {
		// Fallback: try to find PID via cgroup
		if verbose {
			fmt.Fprintf(os.Stderr, "  CRI PID lookup failed: %v, trying cgroup fallback...\n", err)
		}
		pid, err = namespace.ReadPidFromProc(container.ID)
		if err != nil {
			return fmt.Errorf("cannot determine container PID: %w", err)
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Target PID: %d\n", pid)
	}

	// Validate PID and namespace access
	if err := namespace.ValidatePid(pid); err != nil {
		return err
	}

	nsTypes := namespace.AllTypes()
	if err := namespace.NamespacesAccessible(pid, nsTypes); err != nil {
		return fmt.Errorf("insufficient privileges: %w (try running as root)", err)
	}

	// Determine which shell to use — probe via /proc/PID/root filesystem
	shell := "/bin/sh"
	for _, candidate := range Shells {
		procPath := fmt.Sprintf("/proc/%d/root%s", pid, candidate)
		if info, err := os.Stat(procPath); err == nil && !info.IsDir() {
			shell = candidate
			break
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Entering namespaces [%v] with shell %s\n", nsTypes, shell)
	}

	cmd := namespace.NsenterCmd(pid, nsTypes, shell)
	return cmd.Run()
}
