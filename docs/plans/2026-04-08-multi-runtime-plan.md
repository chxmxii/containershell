# Multi-Runtime Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:executing-plans to implement this plan task-by-task.

**Goal:** Add Docker and Podman (rootful + rootless) support alongside existing CRI, with automatic runtime detection.

**Architecture:** A `runtime.Runtime` interface abstracts container operations. Two backends implement it: CRI (gRPC) and Docker (Engine SDK, also serves Podman via compat API). Auto-detection probes sockets and returns the first working runtime.

**Tech Stack:** Go 1.25, `github.com/docker/docker/client` (Docker SDK), existing `k8s.io/cri-api` (CRI), `github.com/spf13/cobra` (CLI)

---

### Task 1: Create runtime interface and shared types

**Files:**
- Create: `pkg/runtime/runtime.go`

**Step 1: Create the runtime interface file**

```go
package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Runtime abstracts container runtime operations across CRI, Docker, and Podman.
type Runtime interface {
	// ListContainers returns all running containers.
	ListContainers(ctx context.Context) ([]ContainerInfo, error)

	// Resolve finds a single container matching the given options.
	Resolve(ctx context.Context, opts ResolveOptions) (*ContainerInfo, error)

	// ContainerStatus returns detailed info for a single container by ID.
	ContainerStatus(ctx context.Context, containerID string) (*ContainerInfo, error)

	// ExecInteractive creates an interactive exec session with TTY support.
	// Handles streaming internally (stdin/stdout/stderr).
	ExecInteractive(ctx context.Context, containerID string, cmd []string, tty bool) error

	// ExecSync runs a command synchronously and returns output.
	ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) (stdout []byte, stderr []byte, exitCode int32, err error)

	// ContainerPid returns the host PID of the container's init process.
	ContainerPid(ctx context.Context, containerID string) (uint32, error)

	// ContainerLogs streams container logs to stdout.
	ContainerLogs(ctx context.Context, containerID string, follow bool, tail int64) error

	// RuntimeInfo returns metadata about the connected runtime.
	RuntimeInfo(ctx context.Context) (*RuntimeInfo, error)

	// Close releases resources.
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
```

**Step 2: Verify it compiles**

Run: `cd /tmp/sides && go build ./pkg/runtime/`
Expected: Success (no consumers yet)

**Step 3: Commit**

```bash
git add pkg/runtime/runtime.go
git commit -m "feat: add runtime.Runtime interface for multi-runtime support"
```

---

### Task 2: Create CRI backend

Refactor existing `pkg/cri/` into a new `pkg/runtime/cri/` that implements the `runtime.Runtime` interface.

**Files:**
- Create: `pkg/runtime/cri/cri.go`

**Step 1: Create CRI backend implementing runtime.Runtime**

The CRI backend wraps the existing gRPC client logic. Key changes:
- `ExecInteractive` replaces separate `Exec()` + `streamExec()` — handles SPDY streaming internally.
- `ContainerLogs` gets log path from CRI status response and tails it.
- `Resolve` delegates to `ResolveByFilter`.

```go
package cri

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/containershell/containershell/pkg/runtime"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

// Runtime implements runtime.Runtime for CRI-compatible runtimes (containerd, CRI-O).
type Runtime struct {
	conn    *grpc.ClientConn
	runtime runtimeapi.RuntimeServiceClient
	image   runtimeapi.ImageServiceClient
	socket  string
}

// New connects to a CRI runtime at the given socket path.
func New(socketPath string) (*Runtime, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CRI at %s: %w", socketPath, err)
	}

	return &Runtime{
		conn:    conn,
		runtime: runtimeapi.NewRuntimeServiceClient(conn),
		image:   runtimeapi.NewImageServiceClient(conn),
		socket:  socketPath,
	}, nil
}

func (r *Runtime) Close() error { return r.conn.Close() }

func (r *Runtime) RuntimeInfo(ctx context.Context) (*runtime.RuntimeInfo, error) {
	resp, err := r.runtime.Version(ctx, &runtimeapi.VersionRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime version: %w", err)
	}
	return &runtime.RuntimeInfo{
		Name:       resp.RuntimeName,
		Version:    resp.RuntimeVersion,
		SocketPath: r.socket,
	}, nil
}

func (r *Runtime) ListContainers(ctx context.Context) ([]runtime.ContainerInfo, error) {
	resp, err := r.runtime.ListContainers(ctx, &runtimeapi.ListContainersRequest{
		Filter: &runtimeapi.ContainerFilter{
			State: &runtimeapi.ContainerStateValue{
				State: runtimeapi.ContainerState_CONTAINER_RUNNING,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	containers := make([]runtime.ContainerInfo, 0, len(resp.Containers))
	for _, c := range resp.Containers {
		containers = append(containers, toCtrInfo(c))
	}
	return containers, nil
}

func (r *Runtime) Resolve(ctx context.Context, opts runtime.ResolveOptions) (*runtime.ContainerInfo, error) {
	return runtime.ResolveByFilter(ctx, r, opts)
}

func (r *Runtime) ContainerStatus(ctx context.Context, containerID string) (*runtime.ContainerInfo, error) {
	resp, err := r.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container status for %s: %w", containerID, err)
	}

	s := resp.Status
	info := &runtime.ContainerInfo{
		ID:          s.Id,
		Image:       s.ImageRef,
		State:       s.State.String(),
		Labels:      s.Labels,
		Annotations: s.Annotations,
		CreatedAt:   time.Unix(0, s.CreatedAt),
	}
	if s.Metadata != nil {
		info.Name = s.Metadata.Name
	}
	applyK8sLabels(info, s.Labels)
	return info, nil
}

func (r *Runtime) ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	resp, err := r.runtime.ExecSync(ctx, &runtimeapi.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     timeout,
	})
	if err != nil {
		return nil, nil, -1, fmt.Errorf("exec failed in container %s: %w", containerID, err)
	}
	return resp.Stdout, resp.Stderr, resp.ExitCode, nil
}

func (r *Runtime) ExecInteractive(ctx context.Context, containerID string, cmd []string, tty bool) error {
	resp, err := r.runtime.Exec(ctx, &runtimeapi.ExecRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Tty:         tty,
		Stdin:       true,
		Stdout:      true,
		Stderr:      !tty,
	})
	if err != nil {
		return fmt.Errorf("exec request failed for container %s: %w", containerID, err)
	}
	return streamExec(resp.Url)
}

func (r *Runtime) ContainerPid(ctx context.Context, containerID string) (uint32, error) {
	resp, err := r.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get container status: %w", err)
	}

	infoMap := resp.Info
	if pidStr, ok := infoMap["pid"]; ok {
		var pid uint32
		if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil && pid > 0 {
			return pid, nil
		}
	}

	if infoJSON, ok := infoMap["info"]; ok {
		var pid uint32
		if _, err := fmt.Sscanf(extractPidFromJSON(infoJSON), "%d", &pid); err == nil && pid > 0 {
			return pid, nil
		}
	}

	return 0, fmt.Errorf("could not determine host PID for container %s", containerID)
}

func (r *Runtime) ContainerLogs(ctx context.Context, containerID string, follow bool, tail int64) error {
	resp, err := r.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	logPath := resp.GetStatus().GetLogPath()
	if logPath == "" {
		return fmt.Errorf("no log path found for container %s", containerID)
	}

	fmt.Printf("Log file: %s\n", logPath)
	args := []string{}
	if follow {
		args = append(args, "-f")
	}
	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}
	args = append(args, logPath)

	cmd := exec.CommandContext(ctx, "tail", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// streamExec connects to the CRI streaming exec URL via SPDY.
func streamExec(execURL string) error {
	u, err := url.Parse(execURL)
	if err != nil {
		return fmt.Errorf("invalid exec URL: %w", err)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	transport, upgrader, err := spdy.RoundTripperFor(nil)
	if err != nil {
		return fmt.Errorf("failed to create SPDY transport: %w", err)
	}

	executor, err := remotecommand.NewSPDYExecutorForTransports(transport, upgrader, http.MethodPost, u)
	if err != nil {
		return fmt.Errorf("failed to create SPDY executor: %w", err)
	}

	return executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})
}

// --- helpers ---

func toCtrInfo(c *runtimeapi.Container) runtime.ContainerInfo {
	info := runtime.ContainerInfo{
		ID:          c.Id,
		Image:       c.ImageRef,
		State:       c.State.String(),
		Labels:      c.Labels,
		Annotations: c.Annotations,
		CreatedAt:   time.Unix(0, c.CreatedAt),
	}
	applyK8sLabels(&info, c.Labels)
	if c.Metadata != nil && info.Name == "" {
		info.Name = c.Metadata.Name
	}
	return info
}

func applyK8sLabels(info *runtime.ContainerInfo, labels map[string]string) {
	if name, ok := labels["io.kubernetes.container.name"]; ok {
		info.Name = name
	}
	if pod, ok := labels["io.kubernetes.pod.name"]; ok {
		info.PodName = pod
	}
	if ns, ok := labels["io.kubernetes.pod.namespace"]; ok {
		info.Namespace = ns
	}
}

func extractPidFromJSON(jsonStr string) string {
	key := `"pid":`
	idx := 0
	for {
		i := indexOf(jsonStr[idx:], key)
		if i < 0 {
			return ""
		}
		idx += i + len(key)
		for idx < len(jsonStr) && (jsonStr[idx] == ' ' || jsonStr[idx] == '\t') {
			idx++
		}
		start := idx
		for idx < len(jsonStr) && jsonStr[idx] >= '0' && jsonStr[idx] <= '9' {
			idx++
		}
		if idx > start {
			return jsonStr[start:idx]
		}
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
```

**Step 2: Verify it compiles**

Run: `cd /tmp/sides && go build ./pkg/runtime/cri/`
Expected: Success

**Step 3: Commit**

```bash
git add pkg/runtime/cri/cri.go
git commit -m "feat: add CRI backend implementing runtime.Runtime interface"
```

---

### Task 3: Create Docker/Podman backend

**Files:**
- Create: `pkg/runtime/docker/docker.go`

**Step 1: Add Docker SDK dependency**

Run: `cd /tmp/sides && go get github.com/docker/docker@latest`

**Step 2: Create Docker backend**

```go
package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/containershell/containershell/pkg/runtime"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"golang.org/x/term"
)

// Runtime implements runtime.Runtime for Docker Engine and Podman (via compat API).
type Runtime struct {
	client     *client.Client
	socketPath string
	name       string // "docker" or "podman"
}

// New creates a Docker/Podman runtime client connected to the given socket.
func New(socketPath string, name string) (*Runtime, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socketPath),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s at %s: %w", name, socketPath, err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("failed to ping %s at %s: %w", name, socketPath, err)
	}

	return &Runtime{client: cli, socketPath: socketPath, name: name}, nil
}

func (r *Runtime) Close() error { return r.client.Close() }

func (r *Runtime) RuntimeInfo(ctx context.Context) (*runtime.RuntimeInfo, error) {
	info, err := r.client.ServerVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}
	return &runtime.RuntimeInfo{
		Name:       r.name,
		Version:    info.Version,
		SocketPath: r.socketPath,
	}, nil
}

func (r *Runtime) ListContainers(ctx context.Context) ([]runtime.ContainerInfo, error) {
	containers, err := r.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]runtime.ContainerInfo, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}
		info := runtime.ContainerInfo{
			ID:        c.ID,
			Name:      name,
			Image:     c.Image,
			State:     c.State,
			Labels:    c.Labels,
			CreatedAt: time.Unix(c.Created, 0),
		}
		// Extract K8s metadata from labels (when running under K8s with Docker/Podman)
		if k8sName, ok := c.Labels["io.kubernetes.container.name"]; ok {
			info.Name = k8sName
		}
		if pod, ok := c.Labels["io.kubernetes.pod.name"]; ok {
			info.PodName = pod
		}
		if ns, ok := c.Labels["io.kubernetes.pod.namespace"]; ok {
			info.Namespace = ns
		}
		result = append(result, info)
	}
	return result, nil
}

func (r *Runtime) Resolve(ctx context.Context, opts runtime.ResolveOptions) (*runtime.ContainerInfo, error) {
	return runtime.ResolveByFilter(ctx, r, opts)
}

func (r *Runtime) ContainerStatus(ctx context.Context, containerID string) (*runtime.ContainerInfo, error) {
	inspect, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerID, err)
	}

	name := inspect.Name
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	var createdAt time.Time
	if t, err := time.Parse(time.RFC3339Nano, inspect.Created); err == nil {
		createdAt = t
	}

	info := &runtime.ContainerInfo{
		ID:        inspect.ID,
		Name:      name,
		Image:     inspect.Config.Image,
		State:     inspect.State.Status,
		Labels:    inspect.Config.Labels,
		CreatedAt: createdAt,
	}

	if inspect.State.Pid > 0 {
		info.Pid = uint32(inspect.State.Pid)
	}

	// K8s labels
	if k8sName, ok := inspect.Config.Labels["io.kubernetes.container.name"]; ok {
		info.Name = k8sName
	}
	if pod, ok := inspect.Config.Labels["io.kubernetes.pod.name"]; ok {
		info.PodName = pod
	}
	if ns, ok := inspect.Config.Labels["io.kubernetes.pod.namespace"]; ok {
		info.Namespace = ns
	}

	return info, nil
}

func (r *Runtime) ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}
	execResp, err := r.client.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return nil, nil, -1, fmt.Errorf("exec create failed in container %s: %w", containerID, err)
	}

	resp, err := r.client.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, nil, -1, fmt.Errorf("exec attach failed: %w", err)
	}
	defer resp.Close()

	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return nil, nil, -1, fmt.Errorf("reading exec output failed: %w", err)
	}

	inspectResp, err := r.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return output, nil, -1, fmt.Errorf("exec inspect failed: %w", err)
	}

	return output, nil, int32(inspectResp.ExitCode), nil
}

func (r *Runtime) ExecInteractive(ctx context.Context, containerID string, cmd []string, tty bool) error {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          tty,
	}
	execResp, err := r.client.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return fmt.Errorf("exec create failed: %w", err)
	}

	resp, err := r.client.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{Tty: tty})
	if err != nil {
		return fmt.Errorf("exec attach failed: %w", err)
	}
	defer resp.Close()

	if tty {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)

		// Handle resize
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		defer signal.Stop(sigCh)

		go func() {
			for range sigCh {
				w, h, err := term.GetSize(int(os.Stdin.Fd()))
				if err == nil {
					r.client.ContainerExecResize(ctx, execResp.ID, container.ResizeOptions{
						Height: uint(h),
						Width:  uint(w),
					})
				}
			}
		}()

		// Set initial size
		if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			r.client.ContainerExecResize(ctx, execResp.ID, container.ResizeOptions{
				Height: uint(h),
				Width:  uint(w),
			})
		}
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(os.Stdout, resp.Reader)
		errCh <- err
	}()
	go func() {
		io.Copy(resp.Conn, os.Stdin)
		resp.CloseWrite()
	}()

	return <-errCh
}

func (r *Runtime) ContainerPid(ctx context.Context, containerID string) (uint32, error) {
	inspect, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return 0, fmt.Errorf("failed to inspect container %s: %w", containerID, err)
	}

	if inspect.State.Pid <= 0 {
		return 0, fmt.Errorf("container %s has no running PID", containerID)
	}

	return uint32(inspect.State.Pid), nil
}

func (r *Runtime) ContainerLogs(ctx context.Context, containerID string, follow bool, tail int64) error {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       strconv.FormatInt(tail, 10),
	}
	reader, err := r.client.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(os.Stdout, reader)
	return err
}
```

**Step 3: Run `go mod tidy` and verify compilation**

Run: `cd /tmp/sides && go mod tidy && go build ./pkg/runtime/docker/`
Expected: Success (may need to resolve Docker SDK API differences)

**Step 4: Commit**

```bash
git add pkg/runtime/docker/ go.mod go.sum
git commit -m "feat: add Docker/Podman backend implementing runtime.Runtime"
```

---

### Task 4: Create runtime auto-detection

**Files:**
- Create: `pkg/runtime/detect.go`

**Step 1: Create detection logic**

```go
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
	Name        string // human-readable name
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

	// Rootless sockets
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		// Fallback: /run/user/<uid>
		xdgRuntime = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	candidates = append(candidates,
		SocketCandidate{filepath.Join(xdgRuntime, "podman", "podman.sock"), RuntimePodman, "Podman (rootless)"},
		SocketCandidate{filepath.Join(xdgRuntime, "docker.sock"), RuntimeDocker, "Docker (rootless)"},
	)

	return candidates
}

// DetectSocket finds the first available socket, optionally filtered by runtime type.
// If socketPath is non-empty, it validates that path exists and returns it.
// If runtimeType is not "auto", only sockets of that type are considered.
func DetectSocket(socketPath string, runtimeType RuntimeType) (string, RuntimeType, error) {
	if socketPath != "" {
		if _, err := os.Stat(socketPath); err != nil {
			return "", "", fmt.Errorf("specified socket not found: %s: %w", socketPath, err)
		}
		// Infer runtime type from path if auto
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
		return RuntimeDocker // default assumption for unknown sockets
	}
}
```

**Step 2: Verify compilation**

Run: `cd /tmp/sides && go build ./pkg/runtime/`
Expected: Success

**Step 3: Commit**

```bash
git add pkg/runtime/detect.go
git commit -m "feat: add multi-runtime socket auto-detection with rootless support"
```

---

### Task 5: Update shell strategies to use runtime.Runtime

**Files:**
- Modify: `pkg/shell/strategy.go`
- Modify: `pkg/shell/exec.go`
- Modify: `pkg/shell/debug.go`
- Modify: `pkg/shell/nsenter.go`

**Step 1: Update strategy.go interface**

Change import from `pkg/cri` to `pkg/runtime`. Change `Strategy.Try` signature and `FallbackChain` to accept `runtime.Runtime`.

```go
// Old: import "github.com/containershell/containershell/pkg/cri"
// New: import "github.com/containershell/containershell/pkg/runtime"

// Old: Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error
// New: Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error

// Old: FallbackChain(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, ...)
// New: FallbackChain(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, ...)
```

**Step 2: Update exec.go**

Replace `client.ExecSync()` calls with `rt.ExecSync()` and `client.Exec()` + `streamExec()` with `rt.ExecInteractive()`.

```go
// Old: func (s *CRIExecStrategy) Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error
// New: func (s *CRIExecStrategy) Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error

// Remove streamExec function and SPDY imports — now handled by runtime backend.
// Replace: client.ExecSync(ctx, container.ID, ...) -> rt.ExecSync(ctx, container.ID, ...)
// Replace: client.Exec(ctx, ...) + streamExec(url) -> rt.ExecInteractive(ctx, container.ID, cmd, true)
```

Rename `CRIExecStrategy` to `ExecStrategy` since it's now runtime-agnostic.

**Step 3: Update debug.go**

Replace `*cri.Client` with `runtime.Runtime` in both method signature and internal calls.

```go
// Old: func (s *DebugContainerStrategy) Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error
// New: func (s *DebugContainerStrategy) Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error
// Replace: client.ContainerPid() -> rt.ContainerPid()
```

**Step 4: Update nsenter.go**

Same pattern: replace `*cri.Client` with `runtime.Runtime`.

```go
// Old: func (s *NsenterStrategy) Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error
// New: func (s *NsenterStrategy) Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error
// Replace: client.ContainerPid() -> rt.ContainerPid()
```

**Step 5: Verify compilation**

Run: `cd /tmp/sides && go build ./pkg/shell/`
Expected: May fail until consumers are updated, but the package itself should compile.

**Step 6: Commit**

```bash
git add pkg/shell/
git commit -m "refactor: update shell strategies to use runtime.Runtime interface"
```

---

### Task 6: Update debug package to use runtime.Runtime

**Files:**
- Modify: `pkg/debug/helpers.go`
- Modify: `pkg/debug/logs.go`
- Modify: `pkg/debug/inspect.go`
- Modify: `pkg/debug/top.go`
- Modify: `pkg/debug/env.go`
- Modify: `pkg/debug/netstat.go`
- Modify: `pkg/debug/tcpdump.go`
- Modify: `pkg/debug/strace.go`
- Modify: `pkg/debug/cp.go`
- Modify: `pkg/debug/portfw.go`
- Modify: `pkg/debug/fs.go`

**Step 1: Update helpers.go**

Replace all `*cri.Client` parameters with `runtime.Runtime`:

```go
// Old: import "github.com/containershell/containershell/pkg/cri"
// New: import "github.com/containershell/containershell/pkg/runtime"

// NsRun: func NsRun(ctx, client *cri.Client, ...) -> func NsRun(ctx, rt runtime.Runtime, ...)
// Replace: client.ContainerPid(ctx, containerID) -> rt.ContainerPid(ctx, containerID)
// Same for NsRunOutput and HostNsRun
```

**Step 2: Update logs.go**

Replace CRI-specific `ContainerStatusRaw()` call with `rt.ContainerLogs()`:

```go
// Old:
// func Logs(ctx, client *cri.Client, containerID string, follow bool, tail int64) error {
//     resp, err := client.ContainerStatusRaw(ctx, containerID)
//     logPath := resp.GetStatus().GetLogPath()
//     ... tail logPath ...
// }

// New:
func Logs(ctx context.Context, rt runtime.Runtime, containerID string, follow bool, tail int64) error {
    return rt.ContainerLogs(ctx, containerID, follow, tail)
}
```

**Step 3: Update remaining debug files**

All follow the same pattern — replace `*cri.Client` with `runtime.Runtime` and `client.Xyz()` with `rt.Xyz()`:
- `inspect.go`: `client.ContainerStatus()` -> `rt.ContainerStatus()`
- `top.go`: `client.ExecSync()` -> `rt.ExecSync()`, NsRun calls updated
- `env.go`: `client.ExecSync()` -> `rt.ExecSync()`, `client.ContainerPid()` -> `rt.ContainerPid()`
- `netstat.go`: `client.ExecSync()` -> `rt.ExecSync()`
- `tcpdump.go`: no direct client calls (uses HostNsRun which is updated)
- `strace.go`: `client.ContainerPid()` -> `rt.ContainerPid()`
- `cp.go`: `client.ContainerPid()` -> `rt.ContainerPid()`
- `portfw.go`: `client.ContainerPid()` -> `rt.ContainerPid()`
- `fs.go`: `client.ContainerPid()` -> `rt.ContainerPid()`

**Step 4: Verify compilation**

Run: `cd /tmp/sides && go build ./pkg/debug/`

**Step 5: Commit**

```bash
git add pkg/debug/
git commit -m "refactor: update debug package to use runtime.Runtime interface"
```

---

### Task 7: Update picker and CLI commands

**Files:**
- Modify: `pkg/picker/picker.go`
- Modify: `cmd/containershell/root.go`
- Modify: `cmd/containershell/shell.go`
- Modify: `cmd/containershell/debug_cmds.go`

**Step 1: Update picker.go**

Replace all `cri.ContainerInfo` references with `runtime.ContainerInfo`:

```go
// Old: import "github.com/containershell/containershell/pkg/cri"
// New: import "github.com/containershell/containershell/pkg/runtime"

// All cri.ContainerInfo -> runtime.ContainerInfo
// func Pick(containers []cri.ContainerInfo) -> func Pick(containers []runtime.ContainerInfo)
```

**Step 2: Update root.go**

Add `--runtime` flag, rename `--cri-socket` to `--socket`:

```go
var (
    socketPath  string
    runtimeType string
    containerID string
    podName     string
    namespace   string
    ctrName     string
    verbose     bool
)

func init() {
    rootCmd.PersistentFlags().StringVar(&socketPath, "socket", "", "Container runtime socket path (auto-detected if not set)")
    rootCmd.PersistentFlags().StringVar(&socketPath, "cri-socket", "", "Deprecated: use --socket instead")
    rootCmd.PersistentFlags().MarkDeprecated("cri-socket", "use --socket instead")
    rootCmd.PersistentFlags().StringVar(&runtimeType, "runtime", "auto", "Container runtime: auto, cri, docker, podman")
    // ... rest unchanged ...
}
```

**Step 3: Update shell.go**

Replace CRI client creation with runtime detection:

```go
import (
    "github.com/containershell/containershell/pkg/runtime"
    cri_rt "github.com/containershell/containershell/pkg/runtime/cri"
    docker_rt "github.com/containershell/containershell/pkg/runtime/docker"
)

func runShell(cmd *cobra.Command, args []string) error {
    ctx := context.Background()

    rt, err := connectRuntime(ctx)
    if err != nil {
        return err
    }
    defer rt.Close()

    container, err := resolveOrPick(ctx, rt)
    if err != nil {
        return err
    }

    // ... verbose output ...

    return shell.FallbackChain(ctx, rt, container, shell.DefaultStrategies(), verbose, logf)
}

func connectRuntime(ctx context.Context) (runtime.Runtime, error) {
    sock, rtType, err := runtime.DetectSocket(socketPath, runtime.RuntimeType(runtimeType))
    if err != nil {
        return nil, err
    }

    switch rtType {
    case runtime.RuntimeCRI:
        return cri_rt.New(sock)
    case runtime.RuntimeDocker, runtime.RuntimePodman:
        name := "docker"
        if rtType == runtime.RuntimePodman {
            name = "podman"
        }
        return docker_rt.New(sock, name)
    default:
        return nil, fmt.Errorf("unknown runtime type: %s", rtType)
    }
}

func resolveOrPick(ctx context.Context, rt runtime.Runtime) (*runtime.ContainerInfo, error) {
    // Same logic but using rt instead of client
}
```

**Step 4: Update debug_cmds.go**

Replace `withContainer` helper to use `runtime.Runtime`:

```go
func withContainer(fn func(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo) error) func(cmd *cobra.Command, args []string) error {
    return func(cmd *cobra.Command, args []string) error {
        ctx := context.Background()
        rt, err := connectRuntime(ctx)
        if err != nil {
            return err
        }
        defer rt.Close()

        container, err := resolveOrPick(ctx, rt)
        if err != nil {
            return err
        }

        return fn(ctx, rt, container)
    }
}

// Update all debug command closures: client -> rt
// debug.Logs(ctx, client, ...) -> debug.Logs(ctx, rt, ...)
// etc.
```

**Step 5: Verify full build**

Run: `cd /tmp/sides && go build ./...`

**Step 6: Commit**

```bash
git add pkg/picker/ cmd/containershell/
git commit -m "refactor: update picker and CLI to use runtime.Runtime interface"
```

---

### Task 8: Remove old pkg/cri and final cleanup

**Files:**
- Delete: `pkg/cri/client.go`
- Delete: `pkg/cri/detect.go`
- Delete: `pkg/cri/resolver.go`
- Delete: `pkg/cri/types.go`

**Step 1: Remove old package**

Run: `rm -rf /tmp/sides/pkg/cri`

**Step 2: Verify no remaining imports**

Run: `grep -r '"github.com/containershell/containershell/pkg/cri"' /tmp/sides/`
Expected: No matches

**Step 3: Full build + vet**

Run: `cd /tmp/sides && go build ./... && go vet ./...`

**Step 4: Commit**

```bash
git add -A
git commit -m "refactor: remove old pkg/cri in favor of pkg/runtime/cri"
```

**Step 5: Test manually (if runtime available)**

```bash
# Docker
containershell --runtime docker

# Podman rootless
containershell --runtime podman

# Auto-detect
containershell

# Verbose
containershell -v
```
