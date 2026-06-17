package debug

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/containershell/containershell/pkg/namespace"
	"github.com/containershell/containershell/pkg/runtime"
)

// Netstat shows network connections inside the container.
func Netstat(ctx context.Context, rt runtime.Runtime, containerID string) error {
	// Try exec in container first
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, []string{"ss", "-tulnp"}, 10)
	if err == nil && exitCode == 0 {
		fmt.Print(string(stdout))
		return nil
	}

	// Fallback: nsenter into network namespace and run ss from host
	return HostNsRun(ctx, rt, containerID, namespace.Net, "ss", "-tulnp")
}

// NetstatOutput returns the network connections of the container as a string.
func NetstatOutput(ctx context.Context, rt runtime.Runtime, containerID string) (string, error) {
	// Try exec in container first
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, []string{"ss", "-tulnp"}, 10)
	if err == nil && exitCode == 0 {
		return string(stdout), nil
	}

	// Fallback: nsenter into network namespace and capture ss output
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("cannot determine PID: %w", err)
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		"--net",
		"--", "ss", "-tulnp",
	}

	output, err := exec.CommandContext(ctx, "nsenter", nsenterArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("netstat via nsenter failed: %w", err)
	}
	return string(output), nil
}
