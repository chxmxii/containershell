package shell

import (
	"context"
	"fmt"
	"os"

	"github.com/containershell/containershell/pkg/runtime"
)

// ExecStrategy attempts to exec a shell directly inside the container via the runtime.
type ExecStrategy struct{}

func (s *ExecStrategy) Name() string { return "exec" }

func (s *ExecStrategy) Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error {
	// Probe which shells exist
	var availableShell string
	for _, shell := range Shells {
		_, stderr, exitCode, err := rt.ExecSync(ctx, container.ID, []string{"test", "-x", shell}, 5)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  probe %s: exec error: %v\n", shell, err)
			}
			continue
		}
		if exitCode == 0 {
			availableShell = shell
			break
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "  probe %s: not found (exit=%d, stderr=%s)\n", shell, exitCode, string(stderr))
		}
	}

	if availableShell == "" {
		return fmt.Errorf("no shell binary found in container (tried: %v)", Shells)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  using shell: %s\n", availableShell)
	}

	return rt.ExecInteractive(ctx, container.ID, []string{availableShell}, true)
}
