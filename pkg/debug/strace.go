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

	var args []string
	if targetPid > 0 {
		// A specific PID was requested; trace just that one.
		args = []string{"-p", fmt.Sprintf("%d", targetPid)}
		if followForks {
			args = append(args, "-f")
		}
		fmt.Fprintf(os.Stderr, "Tracing PID %d (Ctrl+C to stop)...\n", targetPid)
	} else {
		// Default: trace the container's whole process tree. The init process
		// is usually idle (blocked in sigsuspend/wait), so attaching to it
		// alone shows nothing — the activity is in its children. -f keeps
		// following anything forked after we attach.
		targets, err := ProcTreePids(int(pid))
		if err != nil {
			targets = []int{int(pid)}
		}
		args = []string{"-f"}
		for _, t := range targets {
			args = append(args, "-p", fmt.Sprintf("%d", t))
		}
		fmt.Fprintf(os.Stderr, "Tracing %d process(es) %v (Ctrl+C to stop)...\n", len(targets), targets)
	}

	cmd := exec.CommandContext(ctx, "strace", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
