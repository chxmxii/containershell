package debug

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/runtime"
)

// PortForward forwards a local port to a port in the container's network namespace.
func PortForward(ctx context.Context, rt runtime.Runtime, containerID string, localPort, remotePort int) error {
	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", localPort, err)
	}
	defer listener.Close()

	fmt.Fprintf(os.Stderr, "Forwarding 127.0.0.1:%d -> container:%d (Ctrl+C to stop)\n", localPort, remotePort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept failed: %w", err)
			}
		}

		go func() {
			defer conn.Close()
			cmd := exec.CommandContext(ctx, "nsenter",
				fmt.Sprintf("--target=%d", pid),
				"--net",
				"--", "socat", fmt.Sprintf("TCP:127.0.0.1:%d", remotePort), "STDIO",
			)

			stdin, _ := cmd.StdinPipe()
			stdout, _ := cmd.StdoutPipe()

			if err := cmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "nsenter socat failed: %v\n", err)
				return
			}

			go io.Copy(stdin, conn)
			io.Copy(conn, stdout)
			cmd.Wait()
		}()
	}
}
