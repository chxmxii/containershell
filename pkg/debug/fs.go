package debug

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containershell/containershell/pkg/runtime"
)

// FS lists files in the container's filesystem.
func FS(ctx context.Context, rt runtime.Runtime, containerID string, path string, recursive bool, pattern string) error {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	rootFS := ContainerRootFS(pid)
	targetPath := filepath.Join(rootFS, path)

	if !recursive {
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
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
