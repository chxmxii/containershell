package debug

import (
	"context"
	"fmt"

	"github.com/containershell/containershell/pkg/runtime"
)

// Top shows processes running in the container.
func Top(ctx context.Context, rt runtime.Runtime, containerID string) error {
	// Try exec first
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, []string{"ps", "aux"}, 10)
	if err == nil && exitCode == 0 {
		fmt.Print(string(stdout))
		return nil
	}

	// Fallback: use nsenter
	output, err := NsRunOutput(ctx, rt, containerID, "ps", "aux")
	if err == nil {
		fmt.Print(string(output))
		return nil
	}

	// Last resort: read from /proc
	pid, pidErr := rt.ContainerPid(ctx, containerID)
	if pidErr != nil {
		return fmt.Errorf("all methods failed: exec (%v), nsenter (%v), pid lookup (%v)", err, err, pidErr)
	}

	fmt.Printf("PID\tCMD\n")
	fmt.Printf("(Container init PID on host: %d — use /proc/%d/root/proc for detailed listing)\n", pid, pid)
	return nil
}
