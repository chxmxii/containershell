package debug

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/runtime"
)

// Strace traces syscalls of a process inside the container.
func Strace(ctx context.Context, rt runtime.Runtime, containerID string, targetPid int, followForks bool) error {
	if _, err := exec.LookPath("strace"); err != nil {
		return fmt.Errorf("strace not found on host: %w", err)
	}

	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine container PID: %w", err)
	}

	straceTarget := int(pid)
	if targetPid > 0 {
		straceTarget = targetPid
	}

	args := []string{"-p", fmt.Sprintf("%d", straceTarget)}
	if followForks {
		args = append(args, "-f")
	}

	fmt.Fprintf(os.Stderr, "Tracing PID %d (Ctrl+C to stop)...\n", straceTarget)

	cmd := exec.CommandContext(ctx, "strace", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
