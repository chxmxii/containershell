package shell

import (
	"context"
	"fmt"
	"strings"

	"github.com/containershell/containershell/pkg/runtime"
)

// Strategy is a single shell-access method.
type Strategy interface {
	// Name returns a human-readable name for this strategy.
	Name() string
	// Try attempts to get a shell into the container.
	// Returns nil on success (the shell session has ended), or an error explaining why it failed.
	Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error
}

// TierError records a failed strategy attempt.
type TierError struct {
	Tier     int
	Strategy string
	Err      error
}

func (e *TierError) Error() string {
	return fmt.Sprintf("tier %d (%s): %s", e.Tier, e.Strategy, e.Err)
}

// FallbackChain tries each strategy in order, returning the first success.
// If all fail, returns an aggregate error with all tier failures.
func FallbackChain(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, strategies []Strategy, verbose bool, logf func(string, ...any)) error {
	var failures []TierError

	for i, s := range strategies {
		tier := i + 1
		logf("Tier %d: Trying %s...", tier, s.Name())

		err := s.Try(ctx, rt, container, verbose)
		if err == nil {
			return nil
		}

		failure := TierError{Tier: tier, Strategy: s.Name(), Err: err}
		failures = append(failures, failure)
		logf("Tier %d: %s failed: %v", tier, s.Name(), err)
	}

	var sb strings.Builder
	sb.WriteString("all shell strategies failed:\n")
	for _, f := range failures {
		sb.WriteString(fmt.Sprintf("  %s\n", f.Error()))
	}
	sb.WriteString("\nTroubleshooting:\n")
	sb.WriteString("  - Tier 1 (exec): Ensure the container has /bin/sh or /bin/bash\n")
	sb.WriteString("  - Tier 2 (debug container): Ensure K8s API is accessible or docker/podman is available\n")
	sb.WriteString("  - Tier 3 (nsenter): Requires root or CAP_SYS_ADMIN on the host\n")
	return fmt.Errorf("%s", sb.String())
}

// DefaultStrategies returns the standard 3-tier chain.
func DefaultStrategies() []Strategy {
	return []Strategy{
		&ExecStrategy{},
		&DebugContainerStrategy{},
		&NsenterStrategy{},
	}
}

// Shells is the ordered list of shells to try.
var Shells = []string{"/bin/bash", "/bin/sh", "/bin/ash", "/bin/zsh"}
