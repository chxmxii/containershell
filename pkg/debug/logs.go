package debug

import (
	"context"

	"github.com/containershell/containershell/pkg/runtime"
)

// Logs streams container logs from the log file path reported by the runtime.
func Logs(ctx context.Context, rt runtime.Runtime, containerID string, follow bool, tail int64) error {
	return rt.ContainerLogs(ctx, containerID, follow, tail)
}
