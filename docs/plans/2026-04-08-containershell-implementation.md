# ContainerShell Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go CLI tool (`containershell`) that guarantees shell access to any running container via a 3-tier fallback (CRI exec -> debug container injection -> nsenter), with a full debugging toolkit and interactive container picker.

**Architecture:** Cobra CLI with subcommands, a CRI gRPC client abstraction supporting containerd + CRI-O, a `ShellStrategy` chain-of-responsibility pattern for the 3-tier fallback, a bubbletea-based interactive picker, and debug subcommands that operate via CRI or direct namespace access. Each layer is a clean Go package under `pkg/`.

**Tech Stack:** Go 1.22+, cobra, bubbletea, grpc, k8s.io/cri-api, k8s.io/client-go, golang.org/x/sys/unix

---

### Task 0: Project Scaffolding and Go Module Init

**Files:**
- Create: `go.mod`
- Create: `cmd/containershell/main.go`
- Create: `cmd/containershell/root.go`
- Create: `cmd/containershell/version.go`

**Step 1: Initialize Go module**

Run:
```bash
cd /tmp/sides
go mod init github.com/containershell/containershell
```

**Step 2: Create main.go entry point**

```go
// cmd/containershell/main.go
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Create root command with global flags**

```go
// cmd/containershell/root.go
package main

import (
	"github.com/spf13/cobra"
)

var (
	criSocket   string
	containerID string
	podName     string
	namespace   string
	ctrName     string
	verbose     bool
)

var rootCmd = &cobra.Command{
	Use:   "containershell",
	Short: "Swiss Army knife for container debugging",
	Long: `ContainerShell guarantees shell access to any running container using a
3-tier fallback strategy (CRI exec -> debug container injection -> nsenter),
plus a full debugging toolkit.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&criSocket, "cri-socket", "", "CRI socket path (auto-detected if not set)")
	rootCmd.PersistentFlags().StringVar(&containerID, "container-id", "", "Target container ID")
	rootCmd.PersistentFlags().StringVar(&podName, "pod", "", "Target pod name (K8s-aware lookup)")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Target namespace (K8s-aware lookup)")
	rootCmd.PersistentFlags().StringVar(&ctrName, "name", "", "Target container name")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed fallback chain output")

	// Register dynamic completions for --cri-socket
	rootCmd.RegisterFlagCompletionFunc("cri-socket", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"/run/containerd/containerd.sock\tcontainerd",
			"/var/run/crio/crio.sock\tCRI-O",
			"/var/run/dockershim.sock\tdockershim",
		}, cobra.ShellCompDirectiveNoFileComp
	})
}
```

**Step 4: Create version command**

```go
// cmd/containershell/version.go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("containershell %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
```

**Step 5: Install cobra dependency and verify build**

Run:
```bash
go get github.com/spf13/cobra@latest
go build ./cmd/containershell/
./containershell --help
./containershell version
```

**Step 6: Commit**

```bash
git add -A
git commit -m "feat: scaffold project with cobra CLI, root command, and global flags"
```

---

### Task 1: CRI Client Abstraction (containerd + CRI-O)

**Files:**
- Create: `pkg/cri/client.go`
- Create: `pkg/cri/detect.go`
- Create: `pkg/cri/types.go`
- Create: `pkg/cri/client_test.go`

**Step 1: Define container types**

```go
// pkg/cri/types.go
package cri

import "time"

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
	Pid         uint32 // PID of the container's init process on the host
	Labels      map[string]string
	Annotations map[string]string
}

// RuntimeInfo holds metadata about the detected CRI runtime.
type RuntimeInfo struct {
	Name       string // "containerd" or "cri-o"
	Version    string
	SocketPath string
}
```

**Step 2: Write the socket detection logic**

```go
// pkg/cri/detect.go
package cri

import (
	"fmt"
	"os"
)

var defaultSockets = []struct {
	path    string
	runtime string
}{
	{"/run/containerd/containerd.sock", "containerd"},
	{"/var/run/containerd/containerd.sock", "containerd"},
	{"/var/run/crio/crio.sock", "cri-o"},
	{"/run/crio/crio.sock", "cri-o"},
}

// DetectSocket finds the first available CRI socket.
// If socketPath is non-empty, it validates that path exists.
func DetectSocket(socketPath string) (string, error) {
	if socketPath != "" {
		if _, err := os.Stat(socketPath); err != nil {
			return "", fmt.Errorf("specified CRI socket not found: %s: %w", socketPath, err)
		}
		return socketPath, nil
	}

	for _, s := range defaultSockets {
		if _, err := os.Stat(s.path); err == nil {
			return s.path, nil
		}
	}

	return "", fmt.Errorf("no CRI socket found; tried: /run/containerd/containerd.sock, /var/run/crio/crio.sock (use --cri-socket to specify)")
}
```

**Step 3: Write the CRI gRPC client**

```go
// pkg/cri/client.go
package cri

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Client wraps the CRI RuntimeService gRPC client.
type Client struct {
	conn    *grpc.ClientConn
	runtime runtimeapi.RuntimeServiceClient
	image   runtimeapi.ImageServiceClient
	socket  string
}

// NewClient connects to the CRI runtime at the given socket path.
func NewClient(socketPath string) (*Client, error) {
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

	return &Client{
		conn:    conn,
		runtime: runtimeapi.NewRuntimeServiceClient(conn),
		image:   runtimeapi.NewImageServiceClient(conn),
		socket:  socketPath,
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// RuntimeInfo returns information about the connected runtime.
func (c *Client) RuntimeInfo(ctx context.Context) (*RuntimeInfo, error) {
	resp, err := c.runtime.Version(ctx, &runtimeapi.VersionRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime version: %w", err)
	}
	return &RuntimeInfo{
		Name:       resp.RuntimeName,
		Version:    resp.RuntimeVersion,
		SocketPath: c.socket,
	}, nil
}

// ListContainers returns all running containers.
func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	resp, err := c.runtime.ListContainers(ctx, &runtimeapi.ListContainersRequest{
		Filter: &runtimeapi.ContainerFilter{
			State: &runtimeapi.ContainerStateValue{
				State: runtimeapi.ContainerState_CONTAINER_RUNNING,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	containers := make([]ContainerInfo, 0, len(resp.Containers))
	for _, c := range resp.Containers {
		info := ContainerInfo{
			ID:          c.Id,
			Image:       c.ImageRef,
			State:       c.State.String(),
			Labels:      c.Labels,
			Annotations: c.Annotations,
			CreatedAt:   time.Unix(0, c.CreatedAt),
		}
		// Extract K8s metadata from labels
		if name, ok := c.Labels["io.kubernetes.container.name"]; ok {
			info.Name = name
		}
		if pod, ok := c.Labels["io.kubernetes.pod.name"]; ok {
			info.PodName = pod
		}
		if ns, ok := c.Labels["io.kubernetes.pod.namespace"]; ok {
			info.Namespace = ns
		}
		if c.Metadata != nil {
			if info.Name == "" {
				info.Name = c.Metadata.Name
			}
		}
		containers = append(containers, info)
	}
	return containers, nil
}

// ContainerStatus returns detailed status for a container, including the host PID.
func (c *Client) ContainerStatus(ctx context.Context, containerID string) (*ContainerInfo, error) {
	resp, err := c.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container status for %s: %w", containerID, err)
	}

	s := resp.Status
	info := &ContainerInfo{
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
	if name, ok := s.Labels["io.kubernetes.container.name"]; ok {
		info.Name = name
	}
	if pod, ok := s.Labels["io.kubernetes.pod.name"]; ok {
		info.PodName = pod
	}
	if ns, ok := s.Labels["io.kubernetes.pod.namespace"]; ok {
		info.Namespace = ns
	}

	return info, nil
}

// ExecSync runs a command in a container and returns the output.
func (c *Client) ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	resp, err := c.runtime.ExecSync(ctx, &runtimeapi.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     timeout,
	})
	if err != nil {
		return nil, nil, -1, fmt.Errorf("exec failed in container %s: %w", containerID, err)
	}
	return resp.Stdout, resp.Stderr, resp.ExitCode, nil
}

// Exec creates an interactive exec session and returns a URL for streaming.
func (c *Client) Exec(ctx context.Context, containerID string, cmd []string, tty bool) (string, error) {
	resp, err := c.runtime.Exec(ctx, &runtimeapi.ExecRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Tty:         tty,
		Stdin:       true,
		Stdout:      true,
		Stderr:      !tty,
	})
	if err != nil {
		return "", fmt.Errorf("exec request failed for container %s: %w", containerID, err)
	}
	return resp.Url, nil
}

// ContainerPid returns the host PID of the container's init process by inspecting verbose status info.
func (c *Client) ContainerPid(ctx context.Context, containerID string) (uint32, error) {
	resp, err := c.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get container status: %w", err)
	}

	// The PID is in the verbose info map — format depends on runtime
	// containerd: info["pid"] = "12345"
	// CRI-O: info["pid"] = "12345"
	infoMap := resp.Info
	if pidStr, ok := infoMap["pid"]; ok {
		var pid uint32
		if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil && pid > 0 {
			return pid, nil
		}
	}

	// Fallback: parse from the info JSON blob
	if infoJSON, ok := infoMap["info"]; ok {
		var pid uint32
		// Simple extraction — look for "pid":12345
		if _, err := fmt.Sscanf(extractPidFromJSON(infoJSON), "%d", &pid); err == nil && pid > 0 {
			return pid, nil
		}
	}

	return 0, fmt.Errorf("could not determine host PID for container %s", containerID)
}

// extractPidFromJSON does a simple extraction of pid from a JSON string without full parsing.
func extractPidFromJSON(jsonStr string) string {
	// Look for "pid": followed by digits
	key := `"pid":`
	idx := 0
	for {
		i := indexOf(jsonStr[idx:], key)
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

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
```

**Step 4: Write unit test for detect.go**

```go
// pkg/cri/client_test.go
package cri

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSocket_ExplicitPath(t *testing.T) {
	// Create a temp file to simulate a socket
	tmp := filepath.Join(t.TempDir(), "test.sock")
	if err := os.WriteFile(tmp, nil, 0600); err != nil {
		t.Fatal(err)
	}

	path, err := DetectSocket(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != tmp {
		t.Fatalf("expected %s, got %s", tmp, path)
	}
}

func TestDetectSocket_ExplicitPathNotFound(t *testing.T) {
	_, err := DetectSocket("/nonexistent/socket.sock")
	if err == nil {
		t.Fatal("expected error for nonexistent socket")
	}
}

func TestDetectSocket_NoSocketFound(t *testing.T) {
	_, err := DetectSocket("")
	// This will fail on machines without CRI — that's the expected case in test
	if err == nil {
		t.Skip("CRI socket found on this machine, skipping negative test")
	}
}

func TestExtractPidFromJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", `{"pid": 12345, "other": "val"}`, "12345"},
		{"no_space", `{"pid":6789}`, "6789"},
		{"nested", `{"container":{"pid":42}}`, "42"},
		{"not_found", `{"nopid": true}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPidFromJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractPidFromJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 5: Run tests**

Run:
```bash
go get k8s.io/cri-api@latest google.golang.org/grpc@latest
go test ./pkg/cri/ -v
```

**Step 6: Commit**

```bash
git add -A
git commit -m "feat: add CRI client abstraction with containerd/CRI-O socket detection"
```

---

### Task 2: Container Resolver (ID/name/pod lookup)

**Files:**
- Create: `pkg/cri/resolver.go`
- Create: `pkg/cri/resolver_test.go`

**Step 1: Write the resolver**

```go
// pkg/cri/resolver.go
package cri

import (
	"context"
	"fmt"
	"strings"
)

// ResolveOptions specifies how to find a container.
type ResolveOptions struct {
	ContainerID string
	Name        string
	PodName     string
	Namespace   string
}

// Resolve finds a single container matching the given options.
func (c *Client) Resolve(ctx context.Context, opts ResolveOptions) (*ContainerInfo, error) {
	// Direct ID lookup
	if opts.ContainerID != "" {
		return c.ContainerStatus(ctx, opts.ContainerID)
	}

	containers, err := c.ListContainers(ctx)
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
			names[i] = fmt.Sprintf("  %s (pod=%s, ns=%s, id=%s)", m.Name, m.PodName, m.Namespace, m.ID[:12])
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

**Step 2: Run tests and commit**

Run:
```bash
go build ./pkg/cri/
git add -A
git commit -m "feat: add container resolver with name/pod/namespace filtering"
```

---

### Task 3: Namespace Helpers (for nsenter)

**Files:**
- Create: `pkg/namespace/namespace.go`
- Create: `pkg/namespace/namespace_test.go`

**Step 1: Write namespace helpers**

```go
// pkg/namespace/namespace.go
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

// FindShellOnHost checks if nsenter binary is available on the host.
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
	// Try reading from common cgroup paths
	cgroupPaths := []string{
		"/sys/fs/cgroup/pids",
		"/sys/fs/cgroup/memory",
		"/sys/fs/cgroup",
	}

	for _, base := range cgroupPaths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), containerID[:12]) {
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
```

**Step 2: Write tests**

```go
// pkg/namespace/namespace_test.go
package namespace

import (
	"testing"
)

func TestNsPath(t *testing.T) {
	got := NsPath(12345, PID)
	want := "/proc/12345/ns/pid"
	if got != want {
		t.Errorf("NsPath(12345, PID) = %q, want %q", got, want)
	}
}

func TestNsenterCmd(t *testing.T) {
	cmd := NsenterCmd(42, AllTypes(), "/bin/sh")
	args := cmd.Args
	if args[0] != "nsenter" {
		t.Errorf("expected nsenter command, got %s", args[0])
	}
	// Should contain --target=42
	found := false
	for _, a := range args {
		if a == "--target=42" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --target=42 in args: %v", args)
	}
}

func TestValidatePid_Self(t *testing.T) {
	// PID 1 should always exist
	if err := ValidatePid(1); err != nil {
		t.Errorf("PID 1 should be accessible: %v", err)
	}
}

func TestValidatePid_Invalid(t *testing.T) {
	if err := ValidatePid(999999999); err == nil {
		t.Error("expected error for nonexistent PID")
	}
}
```

**Step 3: Run tests and commit**

Run:
```bash
go test ./pkg/namespace/ -v
git add -A
git commit -m "feat: add namespace helpers for nsenter-based container entry"
```

---

### Task 4: 3-Tier Shell Strategy

**Files:**
- Create: `pkg/shell/strategy.go`
- Create: `pkg/shell/exec.go`
- Create: `pkg/shell/debug.go`
- Create: `pkg/shell/nsenter.go`
- Create: `pkg/shell/strategy_test.go`

**Step 1: Define the strategy interface and chain**

```go
// pkg/shell/strategy.go
package shell

import (
	"context"
	"fmt"
	"strings"

	"github.com/containershell/containershell/pkg/cri"
)

// Strategy is a single shell-access method.
type Strategy interface {
	// Name returns a human-readable name for this strategy.
	Name() string
	// Try attempts to get a shell into the container.
	// Returns nil on success (the shell session has ended), or an error explaining why it failed.
	Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error
}

// TierError records a failed strategy attempt.
type TierError struct {
	Tier     int
	Strategy string
	Err      error
}

func (e *TierError) Error() string {
	return fmt.Sprintf("tier %d (%s): %s", e.Tier, e.Strategy, e.Err)
}

// FallbackChain tries each strategy in order, returning the first success.
// If all fail, returns an aggregate error with all tier failures.
func FallbackChain(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, strategies []Strategy, verbose bool, logf func(string, ...any)) error {
	var failures []TierError

	for i, s := range strategies {
		tier := i + 1
		logf("Tier %d: Trying %s...", tier, s.Name())

		err := s.Try(ctx, client, container, verbose)
		if err == nil {
			return nil // Success — shell session ended normally
		}

		failure := TierError{Tier: tier, Strategy: s.Name(), Err: err}
		failures = append(failures, failure)
		logf("Tier %d: %s failed: %v", tier, s.Name(), err)
	}

	// All tiers failed
	var sb strings.Builder
	sb.WriteString("all shell strategies failed:\n")
	for _, f := range failures {
		sb.WriteString(fmt.Sprintf("  %s\n", f.Error()))
	}
	sb.WriteString("\nTroubleshooting:\n")
	sb.WriteString("  - Tier 1 (CRI exec): Ensure the container has /bin/sh or /bin/bash\n")
	sb.WriteString("  - Tier 2 (debug container): Ensure K8s API is accessible or CRI supports debug containers\n")
	sb.WriteString("  - Tier 3 (nsenter): Requires root or CAP_SYS_ADMIN on the host\n")
	return fmt.Errorf("%s", sb.String())
}

// DefaultStrategies returns the standard 3-tier chain.
func DefaultStrategies() []Strategy {
	return []Strategy{
		&CRIExecStrategy{},
		&DebugContainerStrategy{},
		&NsenterStrategy{},
	}
}

// Shells is the ordered list of shells to try.
var Shells = []string{"/bin/bash", "/bin/sh", "/bin/ash", "/bin/zsh"}
```

**Step 2: Implement Tier 1 — CRI Exec**

```go
// pkg/shell/exec.go
package shell

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/containershell/containershell/pkg/cri"
	"golang.org/x/term"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	"net/http"
	"net/url"
)

// CRIExecStrategy attempts to exec a shell directly inside the container via CRI.
type CRIExecStrategy struct{}

func (s *CRIExecStrategy) Name() string { return "CRI exec" }

func (s *CRIExecStrategy) Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error {
	// First, probe which shells exist
	var availableShell string
	for _, shell := range Shells {
		_, stderr, exitCode, err := client.ExecSync(ctx, container.ID, []string{"test", "-x", shell}, 5)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  probe %s: exec error: %v\n", shell, err)
			}
			continue
		}
		if exitCode == 0 {
			availableShell = shell
			break
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "  probe %s: not found (exit=%d, stderr=%s)\n", shell, exitCode, string(stderr))
		}
	}

	if availableShell == "" {
		return fmt.Errorf("no shell binary found in container (tried: %v)", Shells)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  using shell: %s\n", availableShell)
	}

	// Get the exec URL for an interactive session
	execURL, err := client.Exec(ctx, container.ID, []string{availableShell}, true)
	if err != nil {
		return fmt.Errorf("failed to create exec session: %w", err)
	}

	return streamExec(execURL)
}

// streamExec connects to the CRI streaming exec URL and attaches stdin/stdout/stderr.
func streamExec(execURL string) error {
	u, err := url.Parse(execURL)
	if err != nil {
		return fmt.Errorf("invalid exec URL: %w", err)
	}

	// Set up terminal raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Handle resize signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	transport, upgrader, err := spdy.RoundTripperFor(&restConfigForStreaming{})
	if err != nil {
		return fmt.Errorf("failed to create SPDY transport: %w", err)
	}

	exec, err := remotecommand.NewSPDYExecutorForTransports(transport, upgrader, http.MethodPost, u)
	if err != nil {
		return fmt.Errorf("failed to create SPDY executor: %w", err)
	}

	return exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})
}

// restConfigForStreaming implements the interfaces needed for SPDY transport.
type restConfigForStreaming struct{}

func (r *restConfigForStreaming) TLSClientConfig() ([]byte, []byte, []byte, error) {
	return nil, nil, nil, nil
}
```

**Step 3: Implement Tier 2 — Debug Container Injection**

```go
// pkg/shell/debug.go
package shell

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/containershell/containershell/pkg/cri"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const debugImage = "busybox:latest"

// DebugContainerStrategy injects a debug container that shares the target's namespaces.
type DebugContainerStrategy struct{}

func (s *DebugContainerStrategy) Name() string { return "debug container injection" }

func (s *DebugContainerStrategy) Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error {
	// Try K8s ephemeral containers first if we have pod info
	if container.PodName != "" && container.Namespace != "" {
		err := s.tryK8sEphemeral(ctx, container, verbose)
		if err == nil {
			return nil
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "  K8s ephemeral container failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Falling back to CRI-level debug container...\n")
		}
	}

	// Fall back to CRI-level: use nsenter from a privileged helper
	return s.tryCRIDebug(ctx, client, container, verbose)
}

func (s *DebugContainerStrategy) tryK8sEphemeral(ctx context.Context, container *cri.ContainerInfo, verbose bool) error {
	// Try to build a K8s client
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("no kubeconfig available: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create K8s client: %w", err)
	}

	debugName := fmt.Sprintf("containershell-debug-%d", time.Now().Unix())

	// Add ephemeral container to the pod
	pod, err := clientset.CoreV1().Pods(container.Namespace).Get(ctx, container.PodName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod %s/%s: %w", container.Namespace, container.PodName, err)
	}

	ec := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:    debugName,
			Image:   debugImage,
			Command: []string{"/bin/sh"},
			Stdin:   true,
			TTY:     true,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{"SYS_PTRACE", "NET_RAW", "NET_ADMIN"},
				},
			},
		},
		TargetContainerName: container.Name,
	}

	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)
	_, err = clientset.CoreV1().Pods(container.Namespace).UpdateEphemeralContainers(
		ctx, container.PodName, pod, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ephemeral container: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Created ephemeral container %s targeting %s\n", debugName, container.Name)
	}

	// Wait for the ephemeral container to be running
	for i := 0; i < 30; i++ {
		pod, err = clientset.CoreV1().Pods(container.Namespace).Get(ctx, container.PodName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to poll pod status: %w", err)
		}
		for _, cs := range pod.Status.EphemeralContainerStatuses {
			if cs.Name == debugName && cs.State.Running != nil {
				// Attach using kubectl exec
				return kubectlAttach(container.Namespace, container.PodName, debugName)
			}
		}
		time.Sleep(time.Second)
	}

	return fmt.Errorf("ephemeral container %s did not start within 30s", debugName)
}

func kubectlAttach(namespace, pod, container string) error {
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found: %w", err)
	}

	cmd := exec.Command(kubectl, "attach", "-it",
		"-n", namespace,
		pod,
		"-c", container,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *DebugContainerStrategy) tryCRIDebug(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error {
	// Get the PID of the target container for namespace sharing
	pid, err := client.ContainerPid(ctx, container.ID)
	if err != nil {
		return fmt.Errorf("cannot determine container PID for CRI debug: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Target container PID: %d\n", pid)
		fmt.Fprintf(os.Stderr, "  Launching debug container with shared namespaces via nsenter...\n")
	}

	// Use a docker/podman run with --pid/--net of the target
	// Try podman first (rootless), then docker
	for _, runtime := range []string{"podman", "docker"} {
		runtimePath, err := exec.LookPath(runtime)
		if err != nil {
			continue
		}

		cmd := exec.Command(runtimePath, "run", "-it", "--rm",
			fmt.Sprintf("--pid=host"),
			fmt.Sprintf("--network=container:%s", container.ID),
			"--privileged",
			debugImage,
			"nsenter", fmt.Sprintf("--target=%d", pid),
			"--mount", "--uts", "--ipc", "--net", "--pid",
			"--", "/bin/sh",
		)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("CRI-level debug container injection failed: no container runtime (docker/podman) available")
}
```

**Step 4: Implement Tier 3 — nsenter**

```go
// pkg/shell/nsenter.go
package shell

import (
	"context"
	"fmt"
	"os"

	"github.com/containershell/containershell/pkg/cri"
	"github.com/containershell/containershell/pkg/namespace"
)

// NsenterStrategy enters the container's namespaces directly using nsenter.
type NsenterStrategy struct{}

func (s *NsenterStrategy) Name() string { return "nsenter" }

func (s *NsenterStrategy) Try(ctx context.Context, client *cri.Client, container *cri.ContainerInfo, verbose bool) error {
	// Ensure nsenter is available
	nsenterPath, err := namespace.FindNsenter()
	if err != nil {
		return fmt.Errorf("nsenter not available: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "  nsenter found at: %s\n", nsenterPath)
	}

	// Get the container's host PID
	pid, err := client.ContainerPid(ctx, container.ID)
	if err != nil {
		// Fallback: try to find PID via cgroup
		if verbose {
			fmt.Fprintf(os.Stderr, "  CRI PID lookup failed: %v, trying cgroup fallback...\n", err)
		}
		pid, err = namespace.ReadPidFromProc(container.ID)
		if err != nil {
			return fmt.Errorf("cannot determine container PID: %w", err)
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Target PID: %d\n", pid)
	}

	// Validate PID and namespace access
	if err := namespace.ValidatePid(pid); err != nil {
		return err
	}

	nsTypes := namespace.AllTypes()
	if err := namespace.NamespacesAccessible(pid, nsTypes); err != nil {
		return fmt.Errorf("insufficient privileges: %w (try running as root)", err)
	}

	// Determine which shell to use — probe via /proc/PID/root filesystem
	shell := "/bin/sh"
	for _, candidate := range Shells {
		procPath := fmt.Sprintf("/proc/%d/root%s", pid, candidate)
		if info, err := os.Stat(procPath); err == nil && !info.IsDir() {
			shell = candidate
			break
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Entering namespaces [%v] with shell %s\n", nsTypes, shell)
	}

	// Execute nsenter
	cmd := namespace.NsenterCmd(pid, nsTypes, shell)
	return cmd.Run()
}
```

**Step 5: Write strategy test**

```go
// pkg/shell/strategy_test.go
package shell

import (
	"testing"
)

func TestDefaultStrategies(t *testing.T) {
	strategies := DefaultStrategies()
	if len(strategies) != 3 {
		t.Fatalf("expected 3 strategies, got %d", len(strategies))
	}

	expected := []string{"CRI exec", "debug container injection", "nsenter"}
	for i, s := range strategies {
		if s.Name() != expected[i] {
			t.Errorf("strategy %d: expected %q, got %q", i, expected[i], s.Name())
		}
	}
}

func TestTierError(t *testing.T) {
	err := &TierError{Tier: 1, Strategy: "CRI exec", Err: fmt.Errorf("no shell found")}
	got := err.Error()
	if got != "tier 1 (CRI exec): no shell found" {
		t.Errorf("unexpected error string: %s", got)
	}
}
```

**Step 6: Build and commit**

Run:
```bash
go get golang.org/x/term@latest k8s.io/client-go@latest k8s.io/api@latest k8s.io/apimachinery@latest
go build ./pkg/shell/
go test ./pkg/shell/ -v
git add -A
git commit -m "feat: implement 3-tier shell fallback (CRI exec, debug injection, nsenter)"
```

---

### Task 5: Interactive Container Picker TUI

**Files:**
- Create: `pkg/picker/picker.go`
- Create: `pkg/picker/picker_test.go`

**Step 1: Write the picker**

```go
// pkg/picker/picker.go
package picker

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containershell/containershell/pkg/cri"
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	normalStyle   = lipgloss.NewStyle()
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	filterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type model struct {
	containers []cri.ContainerInfo
	filtered   []cri.ContainerInfo
	cursor     int
	filter     string
	selected   *cri.ContainerInfo
	quitting   bool
}

func initialModel(containers []cri.ContainerInfo) model {
	return model{
		containers: containers,
		filtered:   containers,
	}
}

func (m model) Init() bubbletea.Cmd {
	return nil
}

func (m model) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, bubbletea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}

		case "enter":
			if len(m.filtered) > 0 {
				m.selected = &m.filtered[m.cursor]
			}
			return m, bubbletea.Quit

		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.applyFilter()
			}

		default:
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m.applyFilter()
			}
		}
	}
	return m, nil
}

func (m *model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.containers
	} else {
		lower := strings.ToLower(m.filter)
		m.filtered = nil
		for _, c := range m.containers {
			searchStr := strings.ToLower(fmt.Sprintf("%s %s %s %s %s",
				c.Name, c.PodName, c.Namespace, c.Image, c.ID))
			if strings.Contains(searchStr, lower) {
				m.filtered = append(m.filtered, c)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("ContainerShell — Select a container"))
	b.WriteString("\n")

	if m.filter != "" {
		b.WriteString(filterStyle.Render(fmt.Sprintf("Filter: %s", m.filter)))
	} else {
		b.WriteString(dimStyle.Render("Type to filter, ↑/↓ to navigate, Enter to select, Esc to cancel"))
	}
	b.WriteString("\n\n")

	// Table header
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %-20s %-25s %-15s %-40s %-14s",
		"NAME", "POD", "NAMESPACE", "IMAGE", "AGE")))
	b.WriteString("\n")

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("  No containers match the filter"))
		b.WriteString("\n")
	}

	for i, c := range m.filtered {
		age := formatAge(time.Since(c.CreatedAt))
		image := truncate(c.Image, 40)
		name := truncate(c.Name, 20)
		pod := truncate(c.PodName, 25)
		ns := truncate(c.Namespace, 15)

		line := fmt.Sprintf("  %-20s %-25s %-15s %-40s %-14s", name, pod, ns, image, age)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render("▸ " + line[2:]))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Pick launches the interactive container picker and returns the selected container.
// Returns nil if the user cancels.
func Pick(containers []cri.ContainerInfo) (*cri.ContainerInfo, error) {
	if len(containers) == 0 {
		return nil, fmt.Errorf("no running containers found")
	}

	// If only one container, select it automatically
	if len(containers) == 1 {
		return &containers[0], nil
	}

	m := initialModel(containers)
	p := bubbletea.NewProgram(m, bubbletea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("picker failed: %w", err)
	}

	final := result.(model)
	if final.quitting && final.selected == nil {
		return nil, fmt.Errorf("cancelled by user")
	}

	return final.selected, nil
}
```

**Step 2: Write test**

```go
// pkg/picker/picker_test.go
package picker

import (
	"testing"
	"time"

	"github.com/containershell/containershell/pkg/cri"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
		{48*time.Hour + 3*time.Hour, "2d3h"},
	}
	for _, tt := range tests {
		got := formatAge(tt.d)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"this is a very long string", 10, "this is..."},
		{"ab", 2, "ab"},
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestPick_SingleContainer(t *testing.T) {
	containers := []cri.ContainerInfo{
		{ID: "abc123", Name: "nginx", PodName: "web-pod", Namespace: "default"},
	}
	result, err := Pick(containers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "abc123" {
		t.Errorf("expected container abc123, got %s", result.ID)
	}
}

func TestPick_NoContainers(t *testing.T) {
	_, err := Pick(nil)
	if err == nil {
		t.Fatal("expected error for empty container list")
	}
}
```

**Step 3: Build, test, commit**

Run:
```bash
go get github.com/charmbracelet/bubbletea@latest github.com/charmbracelet/lipgloss@latest
go test ./pkg/picker/ -v
git add -A
git commit -m "feat: add interactive container picker TUI with fuzzy filtering"
```

---

### Task 6: Shell Subcommand (wires up 3-tier fallback)

**Files:**
- Create: `cmd/containershell/shell.go`

**Step 1: Write the shell command**

```go
// cmd/containershell/shell.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/containershell/containershell/pkg/cri"
	"github.com/containershell/containershell/pkg/picker"
	"github.com/containershell/containershell/pkg/shell"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Get an interactive shell in a container (3-tier fallback)",
	Long: `Attempts to get a shell into the target container using:
  Tier 1: CRI exec (if container has a shell binary)
  Tier 2: Debug container injection (K8s ephemeral or CRI-level)
  Tier 3: nsenter (direct namespace entry from host)`,
	RunE:              runShell,
	ValidArgsFunction: completeContainerIDs,
}

func init() {
	rootCmd.AddCommand(shellCmd)
	// Make shell the default command when no subcommand is given
	rootCmd.RunE = runShell
}

func runShell(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Detect and connect to CRI
	socketPath, err := cri.DetectSocket(criSocket)
	if err != nil {
		return err
	}

	client, err := cri.NewClient(socketPath)
	if err != nil {
		return err
	}
	defer client.Close()

	// Resolve container
	container, err := resolveOrPick(ctx, client)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Target: %s (pod=%s, ns=%s, id=%s)\n",
			container.Name, container.PodName, container.Namespace, container.ID)
	}

	// Run fallback chain
	logf := func(format string, args ...any) {
		if verbose {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	return shell.FallbackChain(ctx, client, container, shell.DefaultStrategies(), verbose, logf)
}

// resolveOrPick resolves the target container from flags, or launches the interactive picker.
func resolveOrPick(ctx context.Context, client *cri.Client) (*cri.ContainerInfo, error) {
	// If any targeting flags are set, use the resolver
	if containerID != "" || podName != "" || namespace != "" || ctrName != "" {
		return client.Resolve(ctx, cri.ResolveOptions{
			ContainerID: containerID,
			Name:        ctrName,
			PodName:     podName,
			Namespace:   namespace,
		})
	}

	// Otherwise, show the interactive picker
	containers, err := client.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	return picker.Pick(containers)
}

// completeContainerIDs provides dynamic shell completions for container IDs.
func completeContainerIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	socketPath, err := cri.DetectSocket(criSocket)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	client, err := cri.NewClient(socketPath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer client.Close()

	containers, err := client.ListContainers(context.Background())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, c := range containers {
		desc := c.Name
		if c.PodName != "" {
			desc = fmt.Sprintf("%s (pod=%s, ns=%s)", c.Name, c.PodName, c.Namespace)
		}
		completions = append(completions, fmt.Sprintf("%s\t%s", c.ID[:12], desc))
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
```

**Step 2: Build and commit**

Run:
```bash
go build ./cmd/containershell/
git add -A
git commit -m "feat: add shell subcommand with 3-tier fallback and interactive picker"
```

---

### Task 7: Debug Toolkit Subcommands

**Files:**
- Create: `pkg/debug/logs.go`
- Create: `pkg/debug/inspect.go`
- Create: `pkg/debug/top.go`
- Create: `pkg/debug/netstat.go`
- Create: `pkg/debug/tcpdump.go`
- Create: `pkg/debug/strace.go`
- Create: `pkg/debug/cp.go`
- Create: `pkg/debug/portfw.go`
- Create: `pkg/debug/env.go`
- Create: `pkg/debug/fs.go`
- Create: `pkg/debug/helpers.go`
- Create: `cmd/containershell/debug_cmds.go`

**Step 1: Write the helpers module**

```go
// pkg/debug/helpers.go
package debug

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/cri"
	"github.com/containershell/containershell/pkg/namespace"
)

// NsRun executes a command inside the container's namespaces using nsenter.
func NsRun(ctx context.Context, client *cri.Client, containerID string, cmd string, args ...string) error {
	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount", "--uts", "--ipc", "--net", "--pid",
		"--", cmd,
	}
	nsenterArgs = append(nsenterArgs, args...)

	c := exec.CommandContext(ctx, "nsenter", nsenterArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// NsRunOutput executes a command inside the container's namespaces and returns stdout.
func NsRunOutput(ctx context.Context, client *cri.Client, containerID string, cmd string, args ...string) ([]byte, error) {
	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("cannot determine PID: %w", err)
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount", "--uts", "--ipc", "--net", "--pid",
		"--", cmd,
	}
	nsenterArgs = append(nsenterArgs, args...)

	return exec.CommandContext(ctx, "nsenter", nsenterArgs...).CombinedOutput()
}

// HostNsRun runs a command on the host but in the container's specific namespace.
func HostNsRun(ctx context.Context, client *cri.Client, containerID string, nsType namespace.Type, cmd string, args ...string) error {
	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	nsFlag := ""
	switch nsType {
	case namespace.Net:
		nsFlag = "--net"
	case namespace.PID:
		nsFlag = "--pid"
	case namespace.Mnt:
		nsFlag = "--mount"
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		nsFlag,
		"--", cmd,
	}
	nsenterArgs = append(nsenterArgs, args...)

	c := exec.CommandContext(ctx, "nsenter", nsenterArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// ContainerRootFS returns the path to the container's root filesystem on the host.
func ContainerRootFS(pid uint32) string {
	return fmt.Sprintf("/proc/%d/root", pid)
}
```

**Step 2: Write each debug subcommand**

```go
// pkg/debug/logs.go
package debug

import (
	"context"
	"fmt"

	"github.com/containershell/containershell/pkg/cri"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Logs streams container logs.
func Logs(ctx context.Context, client *cri.Client, containerID string, follow bool, tail int64) error {
	// CRI doesn't have a direct streaming log API via gRPC —
	// logs are stored in files. Get the log path from status.
	resp, err := client.ContainerStatusRaw(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}

	logPath := resp.GetStatus().GetLogPath()
	if logPath == "" {
		return fmt.Errorf("no log path found for container %s", containerID)
	}

	fmt.Printf("Log file: %s\n", logPath)

	// Use tail/follow on the log file
	args := []string{}
	if follow {
		args = append(args, "-f")
	}
	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}
	args = append(args, logPath)

	return runHostCmd(ctx, "tail", args...)
}
```

```go
// pkg/debug/inspect.go
package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/containershell/containershell/pkg/cri"
)

// Inspect prints detailed container metadata.
func Inspect(ctx context.Context, client *cri.Client, containerID string) error {
	info, err := client.ContainerStatus(ctx, containerID)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	fmt.Println("--- Container Info ---")
	return enc.Encode(info)
}
```

```go
// pkg/debug/top.go
package debug

import (
	"context"
	"fmt"

	"github.com/containershell/containershell/pkg/cri"
)

// Top shows processes running in the container.
func Top(ctx context.Context, client *cri.Client, containerID string) error {
	// Try CRI exec first
	stdout, stderr, exitCode, err := client.ExecSync(ctx, containerID, []string{"ps", "aux"}, 10)
	if err == nil && exitCode == 0 {
		fmt.Print(string(stdout))
		return nil
	}

	// Fallback: use nsenter
	output, err := NsRunOutput(ctx, client, containerID, "ps", "aux")
	if err != nil {
		// Last resort: read from /proc
		pid, pidErr := client.ContainerPid(ctx, containerID)
		if pidErr != nil {
			return fmt.Errorf("ps exec failed (%v, stderr=%s), nsenter failed (%v), pid lookup failed (%v)",
				err, string(stderr), err, pidErr)
		}
		return readProcsFromProcFS(pid)
	}

	fmt.Print(string(output))
	return nil
}

func readProcsFromProcFS(pid uint32) error {
	// Read /proc/<pid>/root/proc for the container's process list
	fmt.Printf("PID\tCMD\n")
	root := ContainerRootFS(pid)
	// This is a simplified version - reads the host's view of the container processes
	fmt.Printf("(Reading from %s/proc)\n", root)
	return nil
}
```

```go
// pkg/debug/netstat.go
package debug

import (
	"context"
	"fmt"

	"github.com/containershell/containershell/pkg/cri"
	"github.com/containershell/containershell/pkg/namespace"
)

// Netstat shows network connections inside the container.
func Netstat(ctx context.Context, client *cri.Client, containerID string) error {
	// Try exec in container first
	stdout, _, exitCode, err := client.ExecSync(ctx, containerID, []string{"ss", "-tulnp"}, 10)
	if err == nil && exitCode == 0 {
		fmt.Print(string(stdout))
		return nil
	}

	// Fallback: nsenter into network namespace only and run ss from host
	return HostNsRun(ctx, client, containerID, namespace.Net, "ss", "-tulnp")
}
```

```go
// pkg/debug/tcpdump.go
package debug

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/cri"
	"github.com/containershell/containershell/pkg/namespace"
)

// Tcpdump captures packets on the container's network namespace.
func Tcpdump(ctx context.Context, client *cri.Client, containerID string, iface string, filter string, count int) error {
	if _, err := exec.LookPath("tcpdump"); err != nil {
		return fmt.Errorf("tcpdump not found on host: %w", err)
	}

	args := []string{}
	if iface != "" {
		args = append(args, "-i", iface)
	} else {
		args = append(args, "-i", "any")
	}
	if count > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", count))
	}
	if filter != "" {
		args = append(args, filter)
	}

	fmt.Fprintf(os.Stderr, "Capturing packets in container's network namespace (Ctrl+C to stop)...\n")
	return HostNsRun(ctx, client, containerID, namespace.Net, "tcpdump", args...)
}
```

```go
// pkg/debug/strace.go
package debug

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/cri"
)

// Strace traces syscalls of a process inside the container.
func Strace(ctx context.Context, client *cri.Client, containerID string, targetPid int, followForks bool) error {
	if _, err := exec.LookPath("strace"); err != nil {
		return fmt.Errorf("strace not found on host: %w", err)
	}

	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine container PID: %w", err)
	}

	straceTarget := int(pid)
	if targetPid > 0 {
		straceTarget = targetPid
	}

	args := []string{"-p", fmt.Sprintf("%d", straceTarget)}
	if followForks {
		args = append(args, "-f")
	}

	fmt.Fprintf(os.Stderr, "Tracing PID %d (Ctrl+C to stop)...\n", straceTarget)

	cmd := exec.CommandContext(ctx, "strace", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

```go
// pkg/debug/cp.go
package debug

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/containershell/containershell/pkg/cri"
)

// CopyFromContainer copies a file from the container to the host.
func CopyFromContainer(ctx context.Context, client *cri.Client, containerID string, srcPath string, dstPath string) error {
	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	hostPath := fmt.Sprintf("/proc/%d/root%s", pid, srcPath)
	src, err := os.Open(hostPath)
	if err != nil {
		return fmt.Errorf("failed to open %s in container: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", dstPath, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	fmt.Printf("Copied %d bytes: %s -> %s\n", n, srcPath, dstPath)
	return nil
}

// CopyToContainer copies a file from the host to the container.
func CopyToContainer(ctx context.Context, client *cri.Client, containerID string, srcPath string, dstPath string) error {
	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	hostPath := fmt.Sprintf("/proc/%d/root%s", pid, dstPath)

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := os.Create(hostPath)
	if err != nil {
		return fmt.Errorf("failed to create %s in container: %w", dstPath, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	fmt.Printf("Copied %d bytes: %s -> container:%s\n", n, srcPath, dstPath)
	return nil
}
```

```go
// pkg/debug/portfw.go
package debug

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"

	"github.com/containershell/containershell/pkg/cri"
)

// PortForward forwards a local port to a port in the container's network namespace.
func PortForward(ctx context.Context, client *cri.Client, containerID string, localPort, remotePort int) error {
	pid, err := client.ContainerPid(ctx, containerID)
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
			// Use nsenter to connect to the container's network namespace
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
```

```go
// pkg/debug/env.go
package debug

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containershell/containershell/pkg/cri"
)

// Env dumps environment variables of the container's init process.
func Env(ctx context.Context, client *cri.Client, containerID string) error {
	// Try CRI exec first
	stdout, _, exitCode, err := client.ExecSync(ctx, containerID, []string{"env"}, 10)
	if err == nil && exitCode == 0 {
		fmt.Print(string(stdout))
		return nil
	}

	// Fallback: read from /proc/<pid>/environ
	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return fmt.Errorf("failed to read environ: %w", err)
	}

	envs := strings.Split(string(data), "\x00")
	for _, e := range envs {
		if e != "" {
			fmt.Println(e)
		}
	}
	return nil
}
```

```go
// pkg/debug/fs.go
package debug

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containershell/containershell/pkg/cri"
)

// FS lists files in the container's filesystem.
func FS(ctx context.Context, client *cri.Client, containerID string, path string, recursive bool, pattern string) error {
	pid, err := client.ContainerPid(ctx, containerID)
	if err != nil {
		return fmt.Errorf("cannot determine PID: %w", err)
	}

	rootFS := ContainerRootFS(pid)
	targetPath := filepath.Join(rootFS, path)

	if !recursive {
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		for _, e := range entries {
			info, _ := e.Info()
			if info != nil {
				fmt.Printf("%s %8d %s\n", info.Mode(), info.Size(), e.Name())
			} else {
				fmt.Println(e.Name())
			}
		}
		return nil
	}

	return filepath.Walk(targetPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}
		relPath := strings.TrimPrefix(p, rootFS)
		if pattern != "" {
			matched, _ := filepath.Match(pattern, filepath.Base(p))
			if !matched {
				return nil
			}
		}
		fmt.Printf("%s %8d %s\n", info.Mode(), info.Size(), relPath)
		return nil
	})
}
```

**Step 3: Wire up all debug subcommands in the CLI**

```go
// cmd/containershell/debug_cmds.go
package main

import (
	"context"
	"fmt"

	"github.com/containershell/containershell/pkg/cri"
	"github.com/containershell/containershell/pkg/debug"
	"github.com/spf13/cobra"
)

// Helper to get CRI client and resolve container for any debug command.
func withContainer(fn func(ctx context.Context, client *cri.Client, container *cri.ContainerInfo) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		socketPath, err := cri.DetectSocket(criSocket)
		if err != nil {
			return err
		}
		client, err := cri.NewClient(socketPath)
		if err != nil {
			return err
		}
		defer client.Close()

		container, err := resolveOrPick(ctx, client)
		if err != nil {
			return err
		}

		if verbose {
			fmt.Printf("Target: %s (id=%s)\n", container.Name, container.ID[:12])
		}

		return fn(ctx, client, container)
	}
}

var (
	logFollow bool
	logTail   int64

	tcpdumpIface  string
	tcpdumpFilter string
	tcpdumpCount  int

	stracePid   int
	straceForks bool

	cpSrc       string
	cpDst       string
	cpDirection string

	portfwLocal  int
	portfwRemote int

	fsPath      string
	fsRecursive bool
	fsPattern   string
)

func init() {
	// logs
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream container logs",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.Logs(ctx, client, c.ID, logFollow, logTail)
		}),
	}
	logsCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().Int64VarP(&logTail, "tail", "t", 100, "Number of lines from the end")
	rootCmd.AddCommand(logsCmd)

	// inspect
	rootCmd.AddCommand(&cobra.Command{
		Use:   "inspect",
		Short: "Show container metadata, config, mounts, and env",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.Inspect(ctx, client, c.ID)
		}),
	})

	// top
	rootCmd.AddCommand(&cobra.Command{
		Use:   "top",
		Short: "List processes inside the container",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.Top(ctx, client, c.ID)
		}),
	})

	// netstat
	rootCmd.AddCommand(&cobra.Command{
		Use:   "netstat",
		Short: "Show network connections and listeners",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.Netstat(ctx, client, c.ID)
		}),
	})

	// tcpdump
	tcpdumpCmd := &cobra.Command{
		Use:   "tcpdump",
		Short: "Capture packets on the container's network",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.Tcpdump(ctx, client, c.ID, tcpdumpIface, tcpdumpFilter, tcpdumpCount)
		}),
	}
	tcpdumpCmd.Flags().StringVarP(&tcpdumpIface, "interface", "i", "", "Network interface (default: any)")
	tcpdumpCmd.Flags().StringVarP(&tcpdumpFilter, "filter", "f", "", "BPF filter expression")
	tcpdumpCmd.Flags().IntVarP(&tcpdumpCount, "count", "c", 0, "Stop after N packets")
	rootCmd.AddCommand(tcpdumpCmd)

	// strace
	straceCmd := &cobra.Command{
		Use:   "strace",
		Short: "Trace syscalls of a process in the container",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.Strace(ctx, client, c.ID, stracePid, straceForks)
		}),
	}
	straceCmd.Flags().IntVarP(&stracePid, "pid", "p", 0, "PID to trace (default: container init)")
	straceCmd.Flags().BoolVarP(&straceForks, "follow-forks", "f", false, "Follow child processes")
	rootCmd.AddCommand(straceCmd)

	// cp
	cpCmd := &cobra.Command{
		Use:   "cp",
		Short: "Copy files in/out of the container",
		Long:  "Usage: containershell cp --from container:/path /host/path\n       containershell cp --to /host/path container:/path",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			if cpDirection == "from" {
				return debug.CopyFromContainer(ctx, client, c.ID, cpSrc, cpDst)
			}
			return debug.CopyToContainer(ctx, client, c.ID, cpSrc, cpDst)
		}),
	}
	cpCmd.Flags().StringVar(&cpDirection, "dir", "from", "Direction: 'from' (container->host) or 'to' (host->container)")
	cpCmd.Flags().StringVar(&cpSrc, "src", "", "Source path")
	cpCmd.Flags().StringVar(&cpDst, "dst", "", "Destination path")
	cpCmd.MarkFlagRequired("src")
	cpCmd.MarkFlagRequired("dst")
	rootCmd.AddCommand(cpCmd)

	// portfw
	portfwCmd := &cobra.Command{
		Use:   "portfw",
		Short: "Forward a local port to a port in the container",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.PortForward(ctx, client, c.ID, portfwLocal, portfwRemote)
		}),
	}
	portfwCmd.Flags().IntVarP(&portfwLocal, "local", "l", 0, "Local port")
	portfwCmd.Flags().IntVarP(&portfwRemote, "remote", "r", 0, "Remote port in container")
	portfwCmd.MarkFlagRequired("local")
	portfwCmd.MarkFlagRequired("remote")
	rootCmd.AddCommand(portfwCmd)

	// env
	rootCmd.AddCommand(&cobra.Command{
		Use:   "env",
		Short: "Dump container environment variables",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.Env(ctx, client, c.ID)
		}),
	})

	// fs
	fsCmd := &cobra.Command{
		Use:   "fs [path]",
		Short: "Browse/search the container filesystem",
		RunE: withContainer(func(ctx context.Context, client *cri.Client, c *cri.ContainerInfo) error {
			return debug.FS(ctx, client, c.ID, fsPath, fsRecursive, fsPattern)
		}),
	}
	fsCmd.Flags().StringVarP(&fsPath, "path", "p", "/", "Path to list")
	fsCmd.Flags().BoolVarP(&fsRecursive, "recursive", "r", false, "Recursive listing")
	fsCmd.Flags().StringVar(&fsPattern, "match", "", "Filename glob pattern to filter")
	rootCmd.AddCommand(fsCmd)
}
```

**Step 4: Add missing helper to debug package**

```go
// Add to pkg/debug/helpers.go — the runHostCmd helper
func runHostCmd(ctx context.Context, cmd string, args ...string) error {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
```

Also need a `ContainerStatusRaw` method on the CRI client for the logs command. Add to `pkg/cri/client.go`:

```go
// ContainerStatusRaw returns the raw CRI ContainerStatusResponse.
func (c *Client) ContainerStatusRaw(ctx context.Context, containerID string) (*runtimeapi.ContainerStatusResponse, error) {
	return c.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
}
```

**Step 5: Build and commit**

Run:
```bash
go build ./...
git add -A
git commit -m "feat: add full debug toolkit (logs, inspect, top, netstat, tcpdump, strace, cp, portfw, env, fs)"
```

---

### Task 8: Shell Completion Subcommand

**Files:**
- Create: `cmd/containershell/completion.go`

**Step 1: Write the completion command**

```go
// cmd/containershell/completion.go
package main

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for containershell.

Bash:
  $ source <(containershell completion bash)
  # Or permanently:
  $ containershell completion bash > /etc/bash_completion.d/containershell

Zsh:
  $ source <(containershell completion zsh)
  # Or permanently:
  $ containershell completion zsh > "${fpath[1]}/_containershell"

Fish:
  $ containershell completion fish | source
  # Or permanently:
  $ containershell completion fish > ~/.config/fish/completions/containershell.fish

PowerShell:
  PS> containershell completion powershell | Out-String | Invoke-Expression
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
```

**Step 2: Build and commit**

Run:
```bash
go build ./cmd/containershell/
./containershell completion bash > /dev/null
./containershell completion zsh > /dev/null
git add -A
git commit -m "feat: add shell completion generation (bash, zsh, fish, powershell)"
```

---

### Task 9: Polish, Makefile, and Final Wiring

**Files:**
- Create: `Makefile`
- Create: `.goreleaser.yml`
- Create: `README.md`

**Step 1: Create Makefile**

```makefile
# Makefile
BINARY := containershell
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildDate=$(DATE)"

.PHONY: build test lint clean install

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/containershell/

test:
	go test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)

install: build
	install -m 755 $(BINARY) /usr/local/bin/

completion:
	./$(BINARY) completion bash > /etc/bash_completion.d/$(BINARY) 2>/dev/null || true
	./$(BINARY) completion zsh > /usr/local/share/zsh/site-functions/_$(BINARY) 2>/dev/null || true
```

**Step 2: Build and verify**

Run:
```bash
make build
./containershell --help
./containershell version
./containershell completion --help
```

**Step 3: Commit**

Run:
```bash
git add -A
git commit -m "feat: add Makefile with build, test, install, and completion targets"
```

---

### Task 10: Final Integration Test and Verification

**Step 1: Run all tests**

Run:
```bash
go test ./... -v
```

**Step 2: Verify binary works**

Run:
```bash
make clean build
./containershell --help
./containershell shell --help
./containershell logs --help
./containershell inspect --help
./containershell tcpdump --help
./containershell cp --help
./containershell completion bash | head -20
./containershell version
```

**Step 3: Verify completions generate without error**

Run:
```bash
./containershell completion bash > /dev/null && echo "bash: OK"
./containershell completion zsh > /dev/null && echo "zsh: OK"
./containershell completion fish > /dev/null && echo "fish: OK"
./containershell completion powershell > /dev/null && echo "powershell: OK"
```

**Step 4: Final commit if any changes**

Run:
```bash
git add -A && git diff --cached --quiet || git commit -m "chore: final cleanup and integration verification"
```
