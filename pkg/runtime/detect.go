package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

// RuntimeType identifies a container runtime backend.
type RuntimeType string

const (
	RuntimeAuto   RuntimeType = "auto"
	RuntimeCRI    RuntimeType = "cri"
	RuntimeDocker RuntimeType = "docker"
	RuntimePodman RuntimeType = "podman"
)

// SocketCandidate represents a runtime socket to probe.
type SocketCandidate struct {
	Path        string
	RuntimeType RuntimeType
	Name        string
}

// DefaultSocketCandidates returns the ordered list of sockets to probe.
func DefaultSocketCandidates() []SocketCandidate {
	candidates := []SocketCandidate{
		{"/run/containerd/containerd.sock", RuntimeCRI, "containerd"},
		{"/var/run/containerd/containerd.sock", RuntimeCRI, "containerd"},
		{"/var/run/crio/crio.sock", RuntimeCRI, "CRI-O"},
		{"/run/crio/crio.sock", RuntimeCRI, "CRI-O"},
		{"/var/run/docker.sock", RuntimeDocker, "Docker"},
		{"/run/podman/podman.sock", RuntimePodman, "Podman"},
	}

	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		xdgRuntime = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	candidates = append(candidates,
		SocketCandidate{filepath.Join(xdgRuntime, "podman", "podman.sock"), RuntimePodman, "Podman (rootless)"},
		SocketCandidate{filepath.Join(xdgRuntime, "docker.sock"), RuntimeDocker, "Docker (rootless)"},
	)

	return candidates
}

// DetectSocket finds the first available socket, optionally filtered by runtime type.
func DetectSocket(socketPath string, runtimeType RuntimeType) (string, RuntimeType, error) {
	if socketPath != "" {
		if _, err := os.Stat(socketPath); err != nil {
			return "", "", fmt.Errorf("specified socket not found: %s: %w", socketPath, err)
		}
		if runtimeType == RuntimeAuto {
			runtimeType = inferRuntimeType(socketPath)
		}
		return socketPath, runtimeType, nil
	}

	for _, c := range DefaultSocketCandidates() {
		if runtimeType != RuntimeAuto && c.RuntimeType != runtimeType {
			continue
		}
		if _, err := os.Stat(c.Path); err == nil {
			return c.Path, c.RuntimeType, nil
		}
	}

	return "", "", fmt.Errorf("no container runtime socket found (tried CRI, Docker, Podman paths; use --socket to specify)")
}

func inferRuntimeType(socketPath string) RuntimeType {
	base := filepath.Base(socketPath)
	switch {
	case base == "containerd.sock" || base == "crio.sock":
		return RuntimeCRI
	case base == "docker.sock":
		return RuntimeDocker
	case base == "podman.sock":
		return RuntimePodman
	default:
		return RuntimeDocker
	}
}
