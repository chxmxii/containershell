# Multi-Runtime Support Design

**Date:** 2026-04-08
**Status:** Approved

## Problem

ContainerShell only supports CRI-compatible runtimes (containerd, CRI-O) via gRPC. Users running Docker Engine or Podman get:

```
failed to list containers: rpc error: code = Unimplemented desc = unknown service runtime.v1.RuntimeService
```

Additionally, rootless container runtimes (Podman rootless, Docker rootless) are not detected because the tool only probes system-level socket paths.

## Goals

1. Support Docker Engine, Podman (rootful + rootless), and CRI runtimes
2. Auto-detect the available runtime
3. Allow all users (root and non-root) to use containershell
4. Maintain the existing 3-tier fallback shell strategy

## Design

### Runtime Interface

A `runtime.Runtime` interface abstracts container runtime operations:

```go
type Runtime interface {
    ListContainers(ctx context.Context) ([]ContainerInfo, error)
    Resolve(ctx context.Context, opts ResolveOptions) (*ContainerInfo, error)
    ExecInteractive(ctx context.Context, containerID string, cmd []string, tty bool) error
    ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) ([]byte, []byte, int32, error)
    ContainerPid(ctx context.Context, containerID string) (uint32, error)
    RuntimeInfo(ctx context.Context) (*RuntimeInfo, error)
    Close() error
}
```

`ExecInteractive` handles TTY setup and stdin/stdout streaming internally, since Docker/Podman attach directly (no SPDY URLs).

### Backends

Two implementations:

1. **CRI backend** (`pkg/runtime/cri/`): Existing CRI gRPC logic refactored from `pkg/cri/`. Supports containerd and CRI-O.

2. **Docker backend** (`pkg/runtime/docker/`): Uses Docker Engine SDK (`github.com/docker/docker/client`). Works for both Docker Engine and Podman via Podman's Docker-compatible API socket.

### Socket Detection

Auto-detection probes sockets in order, connects, and verifies the API responds:

| Priority | Runtime | Socket Path | Access |
|----------|---------|-------------|--------|
| 1 | containerd | `/run/containerd/containerd.sock` | root |
| 2 | containerd | `/var/run/containerd/containerd.sock` | root |
| 3 | CRI-O | `/var/run/crio/crio.sock` | root |
| 4 | CRI-O | `/run/crio/crio.sock` | root |
| 5 | Docker | `/var/run/docker.sock` | root/docker group |
| 6 | Podman (root) | `/run/podman/podman.sock` | root |
| 7 | Podman (rootless) | `$XDG_RUNTIME_DIR/podman/podman.sock` | any user |
| 8 | Docker (rootless) | `$XDG_RUNTIME_DIR/docker.sock` | any user |

New CLI flag: `--runtime auto|cri|docker|podman` (default: `auto`).
Existing `--cri-socket` flag renamed to `--socket` (with `--cri-socket` kept as deprecated alias).

### Shell Strategy Changes

All strategies accept `runtime.Runtime` instead of `*cri.Client`:

- **Tier 1 (exec):** Uses `rt.ExecSync()` for shell probing, `rt.ExecInteractive()` for session.
- **Tier 2 (debug container):** Unchanged (already uses docker/podman CLI).
- **Tier 3 (nsenter):** Uses `rt.ContainerPid()`.

### Package Layout

```
pkg/
├── runtime/
│   ├── runtime.go       # Interface, shared types
│   ├── detect.go        # Auto-detect runtime
│   ├── cri/
│   │   └── cri.go       # CRI gRPC implementation
│   └── docker/
│       └── docker.go    # Docker Engine API (Docker + Podman)
├── shell/               # Uses runtime.Runtime
├── picker/              # Uses runtime.ContainerInfo
├── debug/               # Uses runtime.Runtime
└── namespace/           # Unchanged
```

`pkg/cri/` is removed after refactoring into `pkg/runtime/cri/`.

### Dependencies

New: `github.com/docker/docker` (Docker Engine SDK)
Existing: `google.golang.org/grpc`, `k8s.io/cri-api` (for CRI backend)
