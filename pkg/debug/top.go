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

	// Last resort: walk the host /proc process tree (works without root and
	// without a ps binary in the image).
	out, err := topFromProc(ctx, rt, containerID, err)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// TopOutput returns the process list of the container as a string.
func TopOutput(ctx context.Context, rt runtime.Runtime, containerID string) (string, error) {
	// Try exec first
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, []string{"ps", "aux"}, 10)
	if err == nil && exitCode == 0 {
		return string(stdout), nil
	}

	// Fallback: use nsenter
	output, err := NsRunOutput(ctx, rt, containerID, "ps", "aux")
	if err == nil {
		return string(output), nil
	}

	// Last resort: walk the host /proc process tree (works without root and
	// without a ps binary in the image).
	return topFromProc(ctx, rt, containerID, err)
}

// topFromProc resolves the container's init PID and lists its process tree
// from the host /proc. nsErr is the earlier nsenter failure, kept for context
// in the combined error.
func topFromProc(ctx context.Context, rt runtime.Runtime, containerID string, nsErr error) (string, error) {
	pid, pidErr := rt.ContainerPid(ctx, containerID)
	if pidErr != nil {
		return "", fmt.Errorf("all methods failed: no ps in image, nsenter (%v — needs root), pid lookup (%v)", nsErr, pidErr)
	}
	out, procErr := ProcTop(int(pid))
	if procErr != nil {
		return "", fmt.Errorf("all methods failed: no ps in image, nsenter (%v — needs root), /proc walk (%v)", nsErr, procErr)
	}
	return out, nil
}
