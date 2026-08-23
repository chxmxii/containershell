package debug

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containershell/containershell/pkg/runtime"
)

// FS lists files in the container's filesystem. It prefers reading the
// container root through /proc/<pid>/root on the host (rich metadata, no
// binaries needed in the image), which requires root or process ownership;
// otherwise it falls back to running ls/find inside the container.
func FS(ctx context.Context, rt runtime.Runtime, containerID string, path string, recursive bool, pattern string) error {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	procErr := fsFromProc(pid, path, recursive, pattern)
	if procErr == nil {
		return nil
	}

	execErr := fsFromExec(ctx, rt, containerID, path, recursive, pattern)
	if execErr == nil {
		return nil
	}

	return fmt.Errorf("failed to list %s: via /proc (%v — needs root), via in-container ls/find (%v)",
		path, procErr, execErr)
}

// fsFromProc lists the path through the container root at /proc/<pid>/root.
func fsFromProc(pid uint32, path string, recursive bool, pattern string) error {
	rootFS := ContainerRootFS(pid)
	targetPath := filepath.Join(rootFS, path)

	if !recursive {
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return err
		}
		for _, e := range entries {
			info, _ := e.Info()
			if info != nil {
				fmt.Printf("%s %8d %s\n", info.Mode(), info.Size(), e.Name())
			} else {
				fmt.Println(e.Name())
			}
		}
		return nil
	}

	// Walking silently skips inaccessible entries, so verify the root of the
	// walk is readable at all before claiming success.
	if _, err := os.ReadDir(targetPath); err != nil {
		return err
	}

	return filepath.Walk(targetPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}
		relPath := strings.TrimPrefix(p, rootFS)
		if relPath == "" {
			relPath = "/"
		}
		if pattern != "" {
			matched, _ := filepath.Match(pattern, filepath.Base(p))
			if !matched {
				return nil
			}
		}
		fmt.Printf("%s %8d %s\n", info.Mode(), info.Size(), relPath)
		return nil
	})
}

// fsFromExec lists the path by running ls (or find, for recursive listings)
// inside the container.
func fsFromExec(ctx context.Context, rt runtime.Runtime, containerID string, path string, recursive bool, pattern string) error {
	var cmd []string
	if recursive {
		cmd = []string{"find", path}
		if pattern != "" {
			cmd = append(cmd, "-name", pattern)
		}
	} else {
		cmd = []string{"ls", "-la", path}
	}

	stdout, stderr, exitCode, err := rt.ExecSync(ctx, containerID, cmd, 10)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("%s exited %d: %s", cmd[0], exitCode, strings.TrimSpace(string(stderr)))
	}
	fmt.Print(string(stdout))
	return nil
}
