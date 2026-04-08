package debug

import (
	"context"
	"fmt"

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
