package namespace

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// Type represents a Linux namespace type.
type Type string

const (
	PID  Type = "pid"
	Net  Type = "net"
	Mnt  Type = "mnt"
	UTS  Type = "uts"
	IPC  Type = "ipc"
	User Type = "user"
)

// AllTypes returns the default set of namespaces to enter.
func AllTypes() []Type {
	return []Type{PID, Net, Mnt, UTS, IPC}
}

// NsPath returns the namespace file path for a given PID and namespace type.
func NsPath(pid uint32, nsType Type) string {
	return fmt.Sprintf("/proc/%d/ns/%s", pid, nsType)
}

// ValidatePid checks that the given PID exists and is accessible.
func ValidatePid(pid uint32) error {
	procPath := fmt.Sprintf("/proc/%d", pid)
	if _, err := os.Stat(procPath); err != nil {
		return fmt.Errorf("PID %d not accessible: %w", pid, err)
	}
	return nil
}

// NamespacesAccessible checks that all namespace files for a PID are readable.
func NamespacesAccessible(pid uint32, nsTypes []Type) error {
	for _, ns := range nsTypes {
		path := NsPath(pid, ns)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("namespace %s not accessible for PID %d: %w", ns, pid, err)
		}
	}
	return nil
}

// NsenterCmd builds an exec.Cmd that uses nsenter to enter the container's namespaces.
func NsenterCmd(pid uint32, nsTypes []Type, shell string) *exec.Cmd {
	args := []string{
		fmt.Sprintf("--target=%d", pid),
	}
	for _, ns := range nsTypes {
		switch ns {
		case PID:
			args = append(args, "--pid")
		case Net:
			args = append(args, "--net")
		case Mnt:
			args = append(args, "--mount")
		case UTS:
			args = append(args, "--uts")
		case IPC:
			args = append(args, "--ipc")
		case User:
			args = append(args, "--user")
		}
	}
	args = append(args, "--", shell)

	cmd := exec.Command("nsenter", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	return cmd
}

// FindNsenter checks if nsenter binary is available on the host.
func FindNsenter() (string, error) {
	path, err := exec.LookPath("nsenter")
	if err != nil {
		return "", fmt.Errorf("nsenter not found in PATH: %w", err)
	}
	return path, nil
}

// ReadPidFromProc reads the PID from /proc for a container.
// This is a fallback when the CRI doesn't return the PID directly.
func ReadPidFromProc(containerID string) (uint32, error) {
	cgroupPaths := []string{
		"/sys/fs/cgroup/pids",
		"/sys/fs/cgroup/memory",
		"/sys/fs/cgroup",
	}

	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	for _, base := range cgroupPaths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), shortID) {
				procsFile := fmt.Sprintf("%s/%s/cgroup.procs", base, entry.Name())
				data, err := os.ReadFile(procsFile)
				if err != nil {
					continue
				}
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				if len(lines) > 0 {
					pid, err := strconv.ParseUint(lines[0], 10, 32)
					if err == nil {
						return uint32(pid), nil
					}
				}
			}
		}
	}

	return 0, fmt.Errorf("could not find PID for container %s via cgroup", containerID)
}
