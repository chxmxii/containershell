package debug

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/containershell/containershell/pkg/runtime"
)

// Logs streams container logs from the log file path reported by the runtime.
func Logs(ctx context.Context, rt runtime.Runtime, containerID string, follow bool, tail int64) error {
	return rt.ContainerLogs(ctx, containerID, follow, tail)
}

// LogsOutput returns the last `tail` lines of container logs as a string.
// It first attempts to read logs via ExecSync inside the container, then
// falls back to reading the init process stdout via nsenter.
func LogsOutput(ctx context.Context, rt runtime.Runtime, containerID string, tail int64) (string, error) {
	if tail <= 0 {
		tail = 100
	}

	// Try ExecSync first — works when "tail" is available inside the container.
	tailCmd := []string{"tail", "-n", fmt.Sprintf("%d", tail), "/proc/1/fd/1"}
	stdout, _, exitCode, err := rt.ExecSync(ctx, containerID, tailCmd, 10)
	if err == nil && exitCode == 0 && len(stdout) > 0 {
		return string(stdout), nil
	}

	// Fallback: use nsenter to read init process stdout from the host side.
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

	if pipeErr := <-errCh; pipeErr != nil && len(result) == 0 {
		return "", fmt.Errorf("failed to retrieve logs: %w", pipeErr)
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
