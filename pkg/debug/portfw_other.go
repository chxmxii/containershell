//go:build !linux

package debug

import (
	"context"
	"fmt"

	"github.com/containershell/containershell/pkg/runtime"
)

// PortForward requires Linux network namespaces and is not supported on
// this platform.
func PortForward(ctx context.Context, rt runtime.Runtime, containerID string, localPort, remotePort int) error {
	return fmt.Errorf("port forwarding requires Linux network namespaces and is not supported on this platform")
}
