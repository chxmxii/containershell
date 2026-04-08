package debug

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containershell/containershell/pkg/runtime"
)

// Env dumps environment variables of the container's init process.
func Env(ctx context.Context, rt runtime.Runtime, containerID string) error {
	// Try exec first
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, []string{"env"}, 10)
	if err == nil && exitCode == 0 {
		fmt.Print(string(stdout))
		return nil
	}

	// Fallback: read from /proc/<pid>/environ
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return fmt.Errorf("failed to read environ: %w", err)
	}

	envs := strings.Split(string(data), "\x00")
	for _, e := range envs {
		if e != "" {
			fmt.Println(e)
		}
	}
	return nil
}
