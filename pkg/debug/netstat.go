package debug

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/containershell/containershell/pkg/runtime"
)

// Netstat prints the container's network connections and listeners.
func Netstat(ctx context.Context, rt runtime.Runtime, containerID string) error {
	out, err := NetstatOutput(ctx, rt, containerID)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// NetstatOutput returns the container's network connections and listeners.
//
// It tries, in order:
//  1. ss inside the container (via the runtime exec API)
//  2. netstat inside the container
//  3. ss on the host, in the container's network namespace
//  4. netstat on the host, in the container's network namespace
//  5. a direct parse of the kernel /proc/<pid>/net socket tables
//
// The final step needs no ss/netstat binary in the container or on the host, so
// it produces output even for distroless images and hosts without iproute2.
func NetstatOutput(ctx context.Context, rt runtime.Runtime, containerID string) (string, error) {
	// 1 & 2: run a socket-listing tool inside the container.
	for _, tool := range [][]string{{"ss", "-tulnp"}, {"netstat", "-tulnp"}} {
		if out, ok := execInContainer(ctx, rt, containerID, tool); ok {
			return out, nil
		}
	}

	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("cannot determine PID: %w", err)
	}

	// 3 & 4: run the host's tool inside the container's network namespace.
	for _, tool := range [][]string{{"ss", "-tulnp"}, {"netstat", "-tulnp"}} {
		if out, ok := nsenterNet(ctx, pid, tool); ok {
			return out, nil
		}
	}

	// 5: parse the kernel socket tables directly — always available.
	out, perr := netstatFromProc(pid)
	if perr != nil {
		return "", fmt.Errorf("netstat: ss/netstat unavailable and reading /proc/%d/net failed: %w", pid, perr)
	}
	return out, nil
}

// execInContainer runs cmd through the runtime exec API, returning its stdout
// and true only when it exits 0 with non-empty output.
func execInContainer(ctx context.Context, rt runtime.Runtime, containerID string, cmd []string) (string, bool) {
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, cmd, 10)
	if err == nil && exitCode == 0 && len(stdout) > 0 {
		return string(stdout), true
	}
	return "", false
}

// nsenterNet runs cmd on the host inside pid's network namespace, returning its
// combined output and true on success with non-empty output.
func nsenterNet(ctx context.Context, pid uint32, cmd []string) (string, bool) {
	args := append([]string{fmt.Sprintf("--target=%d", pid), "--net", "--"}, cmd...)
	out, err := exec.CommandContext(ctx, "nsenter", args...).CombinedOutput()
	if err == nil && len(out) > 0 {
		return string(out), true
	}
	return "", false
}
