package debug

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	goruntime "runtime"

	"github.com/containershell/containershell/pkg/runtime"
	"golang.org/x/sys/unix"
)

// PortForward forwards a local port to a port in the container's network
// namespace. Connections are dialed directly from inside the container's
// netns (via setns on a dedicated thread), so no socat or nsenter binary is
// needed — but joining the namespace requires root.
func PortForward(ctx context.Context, rt runtime.Runtime, containerID string, localPort, remotePort int) error {
	if remotePort <= 0 {
		return fmt.Errorf("a container port is required (--remote/-R)")
	}
	if localPort <= 0 {
		localPort = remotePort
	}

	pid, err := rt.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	// Preflight: verify the container's netns can be entered at all, so the
	// user gets one actionable error instead of a silent per-connection
	// failure after the listener is up.
	if err := enterNetns(pid, func() error { return nil }); err != nil {
		return fmt.Errorf("cannot enter container network namespace: %w (port forwarding requires root — try sudo)", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", localPort, err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

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
			remote, err := dialInNetns(pid, remotePort)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dial container:%d failed: %v\n", remotePort, err)
				return
			}
			defer remote.Close()

			done := make(chan struct{}, 2)
			go func() { io.Copy(remote, conn); done <- struct{}{} }() //nolint:errcheck
			go func() { io.Copy(conn, remote); done <- struct{}{} }() //nolint:errcheck
			// When either direction ends, tear both connections down.
			<-done
		}()
	}
}

// enterNetns runs fn on a dedicated OS thread joined to the network namespace
// of pid. The thread is discarded afterwards (never unlocked), so the
// namespace switch can never leak into the Go scheduler.
func enterNetns(pid uint32, fn func() error) error {
	nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	target, err := os.Open(nsPath)
	if err != nil {
		return err
	}
	defer target.Close()

	errCh := make(chan error, 1)
	go func() {
		goruntime.LockOSThread()
		if err := unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
			errCh <- fmt.Errorf("setns %s: %w", nsPath, err)
			return
		}
		errCh <- fn()
	}()
	return <-errCh
}

// dialInNetns opens a TCP connection to 127.0.0.1:port from inside the
// network namespace of pid and hands it back as a regular net.Conn.
func dialInNetns(pid uint32, port int) (net.Conn, error) {
	var conn net.Conn
	err := enterNetns(pid, func() error {
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			return fmt.Errorf("socket: %w", err)
		}
		sa := &unix.SockaddrInet4{Port: port, Addr: [4]byte{127, 0, 0, 1}}
		if err := unix.Connect(fd, sa); err != nil {
			unix.Close(fd)
			return fmt.Errorf("connect 127.0.0.1:%d: %w", port, err)
		}
		f := os.NewFile(uintptr(fd), "netns-conn")
		defer f.Close()
		c, err := net.FileConn(f)
		if err != nil {
			return err
		}
		conn = c
		return nil
	})
	return conn, err
}
