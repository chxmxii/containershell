// Package docker implements runtime.Runtime using the Docker Engine API.
// It works with both Docker Engine and Podman (via its Docker-compatible socket).
package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	rt "github.com/containershell/containershell/pkg/runtime"
)

var _ rt.Runtime = (*Runtime)(nil)

// Runtime implements runtime.Runtime via the Docker Engine API.
type Runtime struct {
	cli        *client.Client
	socketPath string
	name       string
}

// New connects to a Docker-compatible daemon at the given Unix socket.
// name should be "docker" or "podman" depending on the detected engine.
func New(socketPath, name string) (*Runtime, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socketPath),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("docker ping %s: %w", socketPath, err)
	}

	return &Runtime{
		cli:        cli,
		socketPath: socketPath,
		name:       name,
	}, nil
}

// Close releases the underlying Docker client connection.
func (r *Runtime) Close() error {
	return r.cli.Close()
}

// ListContainers returns all running containers.
func (r *Runtime) ListContainers(ctx context.Context) ([]rt.ContainerInfo, error) {
	containers, err := r.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}

	result := make([]rt.ContainerInfo, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		info := rt.ContainerInfo{
			ID:        c.ID,
			Name:      name,
			Image:     c.Image,
			State:     string(c.State),
			Labels:    c.Labels,
			CreatedAt: time.Unix(c.Created, 0),
		}

		// Extract Kubernetes labels if present.
		if v, ok := c.Labels["io.kubernetes.container.name"]; ok {
			info.Name = v
		}
		if v, ok := c.Labels["io.kubernetes.pod.name"]; ok {
			info.PodName = v
		}
		if v, ok := c.Labels["io.kubernetes.pod.uid"]; ok {
			info.PodID = v
		}
		if v, ok := c.Labels["io.kubernetes.pod.namespace"]; ok {
			info.Namespace = v
		}

		result = append(result, info)
	}

	return result, nil
}

// ContainerStatus returns detailed information about a container.
func (r *Runtime) ContainerStatus(ctx context.Context, containerID string) (*rt.ContainerInfo, error) {
	inspect, err := r.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("container inspect %s: %w", containerID, err)
	}

	name := strings.TrimPrefix(inspect.Name, "/")
	createdAt, _ := time.Parse(time.RFC3339Nano, inspect.Created)

	info := &rt.ContainerInfo{
		ID:        inspect.ID,
		Name:      name,
		Image:     inspect.Image,
		CreatedAt: createdAt,
	}

	if inspect.State != nil {
		info.State = string(inspect.State.Status)
		info.Pid = uint32(inspect.State.Pid)
	}

	if inspect.Config != nil {
		info.Labels = inspect.Config.Labels
		info.Image = inspect.Config.Image // prefer the human-readable image reference
	}

	// Extract Kubernetes labels if present.
	if info.Labels != nil {
		if v, ok := info.Labels["io.kubernetes.container.name"]; ok {
			info.Name = v
		}
		if v, ok := info.Labels["io.kubernetes.pod.name"]; ok {
			info.PodName = v
		}
		if v, ok := info.Labels["io.kubernetes.pod.uid"]; ok {
			info.PodID = v
		}
		if v, ok := info.Labels["io.kubernetes.pod.namespace"]; ok {
			info.Namespace = v
		}
	}

	return info, nil
}

// ExecSync runs a command in the container and returns captured output.
func (r *Runtime) ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := r.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return nil, nil, -1, fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := r.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, nil, -1, fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader); err != nil {
		return nil, nil, -1, fmt.Errorf("exec read output: %w", err)
	}

	inspectResp, err := r.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), -1, fmt.Errorf("exec inspect: %w", err)
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), int32(inspectResp.ExitCode), nil
}

// ExecInteractive runs an interactive exec session in the container.
func (r *Runtime) ExecInteractive(ctx context.Context, containerID string, cmd []string, tty bool) error {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          tty,
	}

	execResp, err := r.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := r.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{
		Tty: tty,
	})
	if err != nil {
		return fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	if tty {
		return r.handleTTYSession(ctx, execResp.ID, attachResp)
	}

	return r.handleNonTTYSession(attachResp)
}

// handleTTYSession manages an interactive TTY exec session with raw terminal
// mode and window resize handling (SIGWINCH).
func (r *Runtime) handleTTYSession(ctx context.Context, execID string, resp types.HijackedResponse) error {
	// Set terminal to raw mode.
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Perform initial resize.
	r.resizeExecTTY(ctx, execID, fd)

	// Handle SIGWINCH for terminal resize.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGWINCH)
	defer signal.Stop(sigCh)

	go func() {
		for range sigCh {
			r.resizeExecTTY(ctx, execID, fd)
		}
	}()

	// Stream I/O bidirectionally.
	outputDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(os.Stdout, resp.Reader)
		outputDone <- err
	}()

	inputDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(resp.Conn, os.Stdin)
		resp.CloseWrite()
		inputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return fmt.Errorf("output stream: %w", err)
		}
	case <-inputDone:
		// Wait briefly for any remaining output after stdin closes.
		select {
		case <-outputDone:
		case <-time.After(500 * time.Millisecond):
		}
	}

	return nil
}

// handleNonTTYSession manages a non-TTY exec session, demultiplexing stdout/stderr.
func (r *Runtime) handleNonTTYSession(resp types.HijackedResponse) error {
	outputDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(os.Stdout, os.Stderr, resp.Reader)
		outputDone <- err
	}()

	go func() {
		io.Copy(resp.Conn, os.Stdin)
		resp.CloseWrite()
	}()

	if err := <-outputDone; err != nil {
		return fmt.Errorf("output stream: %w", err)
	}
	return nil
}

// resizeExecTTY sends the current terminal dimensions to the exec session.
func (r *Runtime) resizeExecTTY(ctx context.Context, execID string, fd int) {
	w, h, err := term.GetSize(fd)
	if err != nil {
		return
	}
	_ = r.cli.ContainerExecResize(ctx, execID, container.ResizeOptions{
		Height: uint(h),
		Width:  uint(w),
	})
}

// ContainerPid returns the host PID of the container's init process.
func (r *Runtime) ContainerPid(ctx context.Context, containerID string) (uint32, error) {
	inspect, err := r.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return 0, fmt.Errorf("container inspect %s: %w", containerID, err)
	}

	if inspect.State == nil || inspect.State.Pid == 0 {
		return 0, fmt.Errorf("container %s: no PID available (not running?)", containerID)
	}

	return uint32(inspect.State.Pid), nil
}

// ContainerLogs streams container logs to stdout.
func (r *Runtime) ContainerLogs(ctx context.Context, containerID string, follow bool, tail int64) error {
	tailStr := "all"
	if tail > 0 {
		tailStr = fmt.Sprintf("%d", tail)
	}

	reader, err := r.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tailStr,
	})
	if err != nil {
		return fmt.Errorf("container logs %s: %w", containerID, err)
	}
	defer reader.Close()

	// Use StdCopy to demultiplex the log stream. If the container uses a TTY,
	// StdCopy gracefully falls back to a simple copy.
	if _, err := stdcopy.StdCopy(os.Stdout, os.Stderr, reader); err != nil {
		return fmt.Errorf("read logs: %w", err)
	}

	return nil
}

// Resolve finds a container matching the given options.
func (r *Runtime) Resolve(ctx context.Context, opts rt.ResolveOptions) (*rt.ContainerInfo, error) {
	return rt.ResolveByFilter(ctx, r, opts)
}

// RuntimeInfo returns metadata about the connected Docker/Podman engine.
func (r *Runtime) RuntimeInfo(ctx context.Context) (*rt.RuntimeInfo, error) {
	ver, err := r.cli.ServerVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("server version: %w", err)
	}

	return &rt.RuntimeInfo{
		Name:       r.name,
		Version:    ver.Version,
		SocketPath: r.socketPath,
	}, nil
}
