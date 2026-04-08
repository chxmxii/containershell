package debug

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/containershell/containershell/pkg/runtime"
)

// CopyFromContainer copies a file from the container to the host.
func CopyFromContainer(ctx context.Context, rt runtime.Runtime, containerID string, srcPath string, dstPath string) error {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	hostPath := fmt.Sprintf("/proc/%d/root%s", pid, srcPath)
	src, err := os.Open(hostPath)
	if err != nil {
		return fmt.Errorf("failed to open %s in container: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", dstPath, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	fmt.Printf("Copied %d bytes: container:%s -> %s\n", n, srcPath, dstPath)
	return nil
}

// CopyToContainer copies a file from the host to the container.
func CopyToContainer(ctx context.Context, rt runtime.Runtime, containerID string, srcPath string, dstPath string) error {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	hostPath := fmt.Sprintf("/proc/%d/root%s", pid, dstPath)

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := os.Create(hostPath)
	if err != nil {
		return fmt.Errorf("failed to create %s in container: %w", dstPath, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	fmt.Printf("Copied %d bytes: %s -> container:%s\n", n, srcPath, dstPath)
	return nil
}
