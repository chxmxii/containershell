package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Runtime abstracts container runtime operations across CRI, Docker, and Podman.
type Runtime interface {
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
	Resolve(ctx context.Context, opts ResolveOptions) (*ContainerInfo, error)
	ContainerStatus(ctx context.Context, containerID string) (*ContainerInfo, error)
	ExecInteractive(ctx context.Context, containerID string, cmd []string, tty bool) error
	ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) (stdout []byte, stderr []byte, exitCode int32, err error)
	ContainerPid(ctx context.Context, containerID string) (uint32, error)
	ContainerLogs(ctx context.Context, containerID string, follow bool, tail int64) error
	RuntimeInfo(ctx context.Context) (*RuntimeInfo, error)
	Close() error
}

// ContainerInfo holds metadata about a running container.
type ContainerInfo struct {
	ID          string
	Name        string
	Image       string
	State       string
	PodName     string
	PodID       string
	Namespace   string
	CreatedAt   time.Time
	Pid         uint32
	Labels      map[string]string
	Annotations map[string]string
}

// RuntimeInfo holds metadata about the detected container runtime.
type RuntimeInfo struct {
	Name       string // "containerd", "cri-o", "docker", "podman"
	Version    string
	SocketPath string
}

// ResolveOptions specifies how to find a container.
type ResolveOptions struct {
	ContainerID string
	Name        string
	PodName     string
	Namespace   string
}

// ResolveByFilter is a shared implementation of container resolution via list+filter.
// Backends that don't have native resolve can use this.
func ResolveByFilter(ctx context.Context, rt Runtime, opts ResolveOptions) (*ContainerInfo, error) {
	if opts.ContainerID != "" {
		return rt.ContainerStatus(ctx, opts.ContainerID)
	}

	containers, err := rt.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	var matches []ContainerInfo
	for _, ctr := range containers {
		if opts.Name != "" && !strings.Contains(ctr.Name, opts.Name) {
			continue
		}
		if opts.PodName != "" && !strings.Contains(ctr.PodName, opts.PodName) {
			continue
		}
		if opts.Namespace != "" && ctr.Namespace != opts.Namespace {
			continue
		}
		matches = append(matches, ctr)
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no running container found matching %s", describeOpts(opts))
	case 1:
		return &matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			id := m.ID
			if len(id) > 12 {
				id = id[:12]
			}
			names[i] = fmt.Sprintf("  %s (pod=%s, ns=%s, id=%s)", m.Name, m.PodName, m.Namespace, id)
		}
		return nil, fmt.Errorf("multiple containers match %s:\n%s\nUse --container-id to be specific", describeOpts(opts), strings.Join(names, "\n"))
	}
}

func describeOpts(opts ResolveOptions) string {
	parts := []string{}
	if opts.Name != "" {
		parts = append(parts, fmt.Sprintf("name=%q", opts.Name))
	}
	if opts.PodName != "" {
		parts = append(parts, fmt.Sprintf("pod=%q", opts.PodName))
	}
	if opts.Namespace != "" {
		parts = append(parts, fmt.Sprintf("namespace=%q", opts.Namespace))
	}
	if len(parts) == 0 {
		return "(no filters)"
	}
	return strings.Join(parts, ", ")
}
