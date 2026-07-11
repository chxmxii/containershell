// Package cri implements the runtime.Runtime interface using the CRI gRPC API.
package cri

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/containershell/containershell/pkg/runtime"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Compile-time check: *Runtime must satisfy runtime.Runtime.
var _ runtime.Runtime = (*Runtime)(nil)

// Runtime implements runtime.Runtime via CRI gRPC.
type Runtime struct {
	conn   *grpc.ClientConn
	client runtimeapi.RuntimeServiceClient
	image  runtimeapi.ImageServiceClient
	socket string
}

// New connects to the CRI runtime at the given socket path and returns a Runtime.
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
		conn:   conn,
		client: runtimeapi.NewRuntimeServiceClient(conn),
		image:  runtimeapi.NewImageServiceClient(conn),
		socket: socketPath,
	}, nil
}

// Close closes the gRPC connection.
func (r *Runtime) Close() error {
	return r.conn.Close()
}

// RuntimeInfo returns information about the connected CRI runtime.
func (r *Runtime) RuntimeInfo(ctx context.Context) (*runtime.RuntimeInfo, error) {
	resp, err := r.client.Version(ctx, &runtimeapi.VersionRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime version: %w", err)
	}
	return &runtime.RuntimeInfo{
		Name:       resp.RuntimeName,
		Version:    resp.RuntimeVersion,
		SocketPath: r.socket,
	}, nil
}

// ListContainers returns all running containers.
func (r *Runtime) ListContainers(ctx context.Context) ([]runtime.ContainerInfo, error) {
	resp, err := r.client.ListContainers(ctx, &runtimeapi.ListContainersRequest{
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
		info := toCtrInfo(c.Id, c.ImageRef, c.State.String(), c.Labels, c.Annotations, c.CreatedAt)
		if c.Metadata != nil && info.Name == "" {
			info.Name = c.Metadata.Name
		}
		containers = append(containers, info)
	}
	return containers, nil
}

// ContainerStatus returns detailed status for a container.
func (r *Runtime) ContainerStatus(ctx context.Context, containerID string) (*runtime.ContainerInfo, error) {
	resp, err := r.client.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container status for %s: %w", containerID, err)
	}

	s := resp.Status
	info := toCtrInfo(s.Id, s.ImageRef, s.State.String(), s.Labels, s.Annotations, s.CreatedAt)
	if s.Metadata != nil {
		info.Name = s.Metadata.Name
	}
	// K8s labels override metadata name
	applyK8sLabels(&info, s.Labels)
	return &info, nil
}

// ExecSync runs a command in a container synchronously and returns the output.
func (r *Runtime) ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	resp, err := r.client.ExecSync(ctx, &runtimeapi.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     timeout,
	})
	if err != nil {
		return nil, nil, -1, fmt.Errorf("exec failed in container %s: %w", containerID, err)
	}
	return resp.Stdout, resp.Stderr, resp.ExitCode, nil
}

// ExecInteractive creates an interactive exec session via CRI streaming (SPDY).
func (r *Runtime) ExecInteractive(ctx context.Context, containerID string, cmd []string, tty bool) error {
	resp, err := r.client.Exec(ctx, &runtimeapi.ExecRequest{
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

	return streamExec(resp.Url, tty)
}

// ContainerPid returns the host PID of the container's init process.
func (r *Runtime) ContainerPid(ctx context.Context, containerID string) (uint32, error) {
	resp, err := r.client.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get container status: %w", err)
	}

	infoMap := resp.Info
	// Try the direct "pid" key first (containerd and CRI-O)
	if pidStr, ok := infoMap["pid"]; ok {
		var pid uint32
		if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil && pid > 0 {
			return pid, nil
		}
	}

	// Fallback: parse pid from the verbose info JSON blob
	if infoJSON, ok := infoMap["info"]; ok {
		var pid uint32
		if _, err := fmt.Sscanf(extractPidFromJSON(infoJSON), "%d", &pid); err == nil && pid > 0 {
			return pid, nil
		}
	}

	return 0, fmt.Errorf("could not determine host PID for container %s", containerID)
}

// ContainerLogs tails the container log file reported by CRI status into w.
func (r *Runtime) ContainerLogs(ctx context.Context, containerID string, follow bool, tail int64, w io.Writer) error {
	resp, err := r.client.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     false,
	})
	if err != nil {
		return fmt.Errorf("failed to get container status for %s: %w", containerID, err)
	}

	logPath := resp.Status.LogPath
	if logPath == "" {
		return fmt.Errorf("no log path found for container %s", containerID)
	}

	// Build a tail command to read the log file on the host
	args := []string{}
	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, logPath)

	cmd := exec.CommandContext(ctx, "tail", args...)
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start tail for %s: %w", logPath, err)
	}

	// If following, handle context cancellation gracefully
	if follow {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		select {
		case <-ctx.Done():
			_ = cmd.Process.Signal(syscall.SIGTERM)
			return ctx.Err()
		case err := <-done:
			return err
		}
	}

	return cmd.Wait()
}

// Resolve finds a container matching the given options using list+filter.
func (r *Runtime) Resolve(ctx context.Context, opts runtime.ResolveOptions) (*runtime.ContainerInfo, error) {
	return runtime.ResolveByFilter(ctx, r, opts)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// toCtrInfo builds a ContainerInfo and applies Kubernetes label overrides.
func toCtrInfo(id, imageRef, state string, labels, annotations map[string]string, createdAtNano int64) runtime.ContainerInfo {
	info := runtime.ContainerInfo{
		ID:          id,
		Image:       imageRef,
		State:       state,
		Labels:      labels,
		Annotations: annotations,
		CreatedAt:   time.Unix(0, createdAtNano),
	}
	applyK8sLabels(&info, labels)
	return info
}

// applyK8sLabels sets Name, PodName, and Namespace from standard Kubernetes labels.
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

// streamExec connects to the CRI streaming exec URL via SPDY and attaches stdio.
func streamExec(execURL string, tty bool) error {
	u, err := url.Parse(execURL)
	if err != nil {
		return fmt.Errorf("invalid exec URL: %w", err)
	}

	if tty {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	}

	// Handle resize signals
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
		Tty:    tty,
	})
}

// extractPidFromJSON does a simple extraction of "pid" from a JSON string
// without pulling in encoding/json for a partial parse.
func extractPidFromJSON(jsonStr string) string {
	key := `"pid":`
	idx := 0
	for {
		i := strings.Index(jsonStr[idx:], key)
		if i < 0 {
			return ""
		}
		idx += i + len(key)
		// Skip whitespace
		for idx < len(jsonStr) && (jsonStr[idx] == ' ' || jsonStr[idx] == '\t') {
			idx++
		}
		// Read digits
		start := idx
		for idx < len(jsonStr) && jsonStr[idx] >= '0' && jsonStr[idx] <= '9' {
			idx++
		}
		if idx > start {
			return jsonStr[start:idx]
		}
	}
}
