package debug

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/containershell/containershell/pkg/runtime"
)

// Logs streams container logs to stdout using the runtime's native log API.
func Logs(ctx context.Context, rt runtime.Runtime, containerID string, follow bool, tail int64) error {
	return rt.ContainerLogs(ctx, containerID, follow, tail, os.Stdout)
}

// LogsOutput returns the last `tail` lines of container logs as a string.
//
// It prefers the runtime's native log API, which returns the container's
// captured stdout/stderr regardless of what binaries exist inside it — the
// reliable source for distroless and other minimal images. Only if that yields
// nothing does it fall back to reading the init process stdout (via in-container
// tail, then nsenter from the host).
func LogsOutput(ctx context.Context, rt runtime.Runtime, containerID string, tail int64) (string, error) {
	if tail <= 0 {
		tail = 100
	}

	// Primary: the runtime's native logs. A successful call is authoritative,
	// including when the container simply has not written anything yet —
	// falling through on empty output would surface fallback errors (e.g.
	// nsenter permission failures) for perfectly healthy containers.
	var buf bytes.Buffer
	logErr := rt.ContainerLogs(ctx, containerID, false, tail, &buf)
	if logErr == nil {
		if buf.Len() == 0 {
			return "(no log output)", nil
		}
		return buf.String(), nil
	}

	// Fallback 1: read the init process stdout from inside the container.
	tailCmd := []string{"tail", "-n", fmt.Sprintf("%d", tail), "/proc/1/fd/1"}
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, tailCmd, 10)
	if err == nil && exitCode == 0 && len(stdout) > 0 {
		return string(stdout), nil
	}

	// Fallback 2: read the init process stdout from the host side via nsenter.
	pid, pidErr := rt.ContainerPid(ctx, containerID)
	if pidErr != nil {
		return "", fmt.Errorf("failed to retrieve logs: cannot determine PID: %w", pidErr)
	}

	args := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount", "--pid",
		"--", "tail", "-n", fmt.Sprintf("%d", tail), "/proc/1/fd/1",
	}

	pr, pw := io.Pipe()
	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		cmd := exec.CommandContext(captureCtx, "nsenter", args...)
		cmd.Stdout = pw
		cmd.Stderr = pw
		errCh <- cmd.Run()
	}()

	result, _ := io.ReadAll(pr)
	pr.Close()

	// On failure the captured output is nsenter's own stderr, not log
	// content — never return it as if it were the container's logs.
	if pipeErr := <-errCh; pipeErr != nil {
		return "", fmt.Errorf("failed to retrieve logs: native log API (%v); nsenter fallback (%v — needs root): %s",
			logErr, pipeErr, strings.TrimSpace(string(result)))
	}

	return string(result), nil
}

// LogsStream returns a channel that receives log lines in real-time from the container.
// The returned cancel function stops the streaming goroutine and closes the channel.
// Callers must call the cancel function when done consuming logs.
func LogsStream(ctx context.Context, rt runtime.Runtime, containerID string) (<-chan string, context.CancelFunc, error) {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot determine PID for log streaming: %w", err)
	}

	streamCtx, cancel := context.WithCancel(ctx)

	// Use nsenter to tail -f the container's init process stdout.
	// This is the most portable approach across runtime backends.
	args := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount", "--pid",
		"--", "tail", "-f", "-n", "0", "/proc/1/fd/1",
	}
	cmd := exec.CommandContext(streamCtx, "nsenter", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to start log stream: %w", err)
	}

	ch := make(chan string, 64)

	go func() {
		defer close(ch)
		defer cmd.Wait() //nolint:errcheck
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-streamCtx.Done():
				return
			case ch <- scanner.Text():
			}
		}
	}()

	return ch, cancel, nil
}
