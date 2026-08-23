package debug

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/namespace"
	"github.com/containershell/containershell/pkg/runtime"
)

// Tcpdump captures packets on the container's network namespace.
func Tcpdump(ctx context.Context, rt runtime.Runtime, containerID string, iface string, filter string, count int) error {
	if _, err := exec.LookPath("tcpdump"); err != nil {
		return fmt.Errorf("tcpdump not found on host: %w", err)
	}

	args := []string{}
	if iface != "" {
		args = append(args, "-i", iface)
	} else {
		args = append(args, "-i", "any")
	}
	if count > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", count))
	}
	if filter != "" {
		args = append(args, filter)
	}

	fmt.Fprintf(os.Stderr, "Capturing packets in container's network namespace (Ctrl+C to stop)...\n")
	if err := HostNsRun(ctx, rt, containerID, namespace.Net, "tcpdump", args...); err != nil {
		return fmt.Errorf("tcpdump in container netns failed: %w (requires root — try sudo)", err)
	}
	return nil
}
