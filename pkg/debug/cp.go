package debug

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/containershell/containershell/pkg/runtime"
)

// containerAbs anchors a container path at / so it can be joined onto the
// /proc/<pid>/root prefix safely.
func containerAbs(p string) string {
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// CopyFromContainer copies a file from the container to the host. It prefers
// reading through /proc/<pid>/root on the host (needs root or process
// ownership) and falls back to cat inside the container.
func CopyFromContainer(ctx context.Context, rt runtime.Runtime, containerID string, srcPath string, dstPath string) error {
	srcPath = containerAbs(srcPath)
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	procErr := copyFromViaProc(pid, srcPath, dstPath)
	if procErr == nil {
		return nil
	}

	if execErr := copyFromViaExec(ctx, rt, containerID, srcPath, dstPath); execErr != nil {
		return fmt.Errorf("failed to copy container:%s: via /proc (%v — needs root), via in-container cat (%v)",
			srcPath, procErr, execErr)
	}
	return nil
}

// copyFromViaProc reads the source through the container root at /proc.
func copyFromViaProc(pid uint32, srcPath, dstPath string) error {
	src, err := os.Open(ContainerRootFS(pid) + srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return err
	}
	fmt.Printf("Copied %d bytes: container:%s -> %s\n", n, srcPath, dstPath)
	return nil
}

// copyFromViaExec reads the source with cat inside the container.
func copyFromViaExec(ctx context.Context, rt runtime.Runtime, containerID, srcPath, dstPath string) error {
	stdout, stderr, exitCode, err := rt.ExecSync(ctx, containerID, []string{"cat", srcPath}, 30)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("cat exited %d: %s", exitCode, strings.TrimSpace(string(stderr)))
	}
	if err := os.WriteFile(dstPath, stdout, 0o644); err != nil {
		return err
	}
	fmt.Printf("Copied %d bytes: container:%s -> %s\n", len(stdout), srcPath, dstPath)
	return nil
}

// CopyToContainer copies a file from the host to the container, preferring
// /proc/<pid>/root and falling back to base64-chunked writes through the
// container's shell.
func CopyToContainer(ctx context.Context, rt runtime.Runtime, containerID string, srcPath string, dstPath string) error {
	dstPath = containerAbs(dstPath)
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	procErr := copyToViaProc(pid, srcPath, dstPath)
	if procErr == nil {
		return nil
	}

	if execErr := copyToViaExec(ctx, rt, containerID, srcPath, dstPath); execErr != nil {
		return fmt.Errorf("failed to copy to container:%s: via /proc (%v — needs root), via in-container shell (%v)",
			dstPath, procErr, execErr)
	}
	return nil
}

// copyToViaProc writes the destination through the container root at /proc.
func copyToViaProc(pid uint32, srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(ContainerRootFS(pid) + dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return err
	}
	fmt.Printf("Copied %d bytes: %s -> container:%s\n", n, srcPath, dstPath)
	return nil
}

// copyToViaExec writes the destination by piping base64 chunks through the
// container's shell. Chunks stay well under ARG_MAX so arbitrary file sizes
// work; the image must provide sh and base64 (busybox does).
func copyToViaExec(ctx context.Context, rt runtime.Runtime, containerID, srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	const chunkSize = 48 * 1024
	quotedDst := "'" + strings.ReplaceAll(dstPath, "'", `'\''`) + "'"

	for off := 0; ; off += chunkSize {
		end := off + chunkSize
		if end > len(data) {
			end = len(data)
		}
		redir := ">>"
		if off == 0 {
			redir = ">" // truncate on the first chunk
		}
		b64 := base64.StdEncoding.EncodeToString(data[off:end])
		script := fmt.Sprintf("printf %%s %s | base64 -d %s %s", b64, redir, quotedDst)
		if off == 0 && b64 == "" {
			script = fmt.Sprintf(": %s %s", redir, quotedDst) // empty source file
		}

		_, stderr, exitCode, err := rt.ExecSync(ctx, containerID, []string{"sh", "-c", script}, 30)
		if err != nil {
			return err
		}
		if exitCode != 0 {
			return fmt.Errorf("write chunk at %d exited %d: %s", off, exitCode, strings.TrimSpace(string(stderr)))
		}
		if end == len(data) {
			break
		}
	}

	fmt.Printf("Copied %d bytes: %s -> container:%s\n", len(data), srcPath, dstPath)
	return nil
}
