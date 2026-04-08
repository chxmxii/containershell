package debug

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/namespace"
	"github.com/containershell/containershell/pkg/runtime"
)

// NsRun executes a command inside the container's namespaces using nsenter.
func NsRun(ctx context.Context, rt runtime.Runtime, containerID string, cmd string, args ...string) error {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount", "--uts", "--ipc", "--net", "--pid",
		"--", cmd,
	}
	nsenterArgs = append(nsenterArgs, args...)

	c := exec.CommandContext(ctx, "nsenter", nsenterArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// NsRunOutput executes a command inside the container's namespaces and returns stdout.
func NsRunOutput(ctx context.Context, rt runtime.Runtime, containerID string, cmd string, args ...string) ([]byte, error) {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("cannot determine PID: %w", err)
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount", "--uts", "--ipc", "--net", "--pid",
		"--", cmd,
	}
	nsenterArgs = append(nsenterArgs, args...)

	return exec.CommandContext(ctx, "nsenter", nsenterArgs...).CombinedOutput()
}

// HostNsRun runs a command on the host but in the container's specific namespace.
func HostNsRun(ctx context.Context, rt runtime.Runtime, containerID string, nsType namespace.Type, cmd string, args ...string) error {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	nsFlag := ""
	switch nsType {
	case namespace.Net:
		nsFlag = "--net"
	case namespace.PID:
		nsFlag = "--pid"
	case namespace.Mnt:
		nsFlag = "--mount"
	case namespace.UTS:
		nsFlag = "--uts"
	case namespace.IPC:
		nsFlag = "--ipc"
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		nsFlag,
		"--", cmd,
	}
	nsenterArgs = append(nsenterArgs, args...)

	c := exec.CommandContext(ctx, "nsenter", nsenterArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// ContainerRootFS returns the path to the container's root filesystem on the host.
func ContainerRootFS(pid uint32) string {
	return fmt.Sprintf("/proc/%d/root", pid)
}

// runHostCmd runs a command on the host.
func runHostCmd(ctx context.Context, cmd string, args ...string) error {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
