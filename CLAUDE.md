# ContainerShell

Swiss Army knife for container debugging. Guarantees shell access to any running container using a 3-tier fallback strategy, plus a full debugging toolkit.

## Project Overview

- **Module:** `github.com/containershell/containershell`
- **Language:** Go 1.25+
- **Binary:** `containershell`
- **CLI Framework:** [cobra](https://github.com/spf13/cobra)
- **TUI:** [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss)

## Architecture

### Runtime Abstraction (`pkg/runtime/`)

The `runtime.Runtime` interface abstracts container operations across three backends:

| Backend | Package | API | Sockets |
|---------|---------|-----|---------|
| CRI (containerd, CRI-O) | `pkg/runtime/cri/` | gRPC (CRI v1) | `/run/containerd/containerd.sock`, `/var/run/crio/crio.sock` |
| Docker | `pkg/runtime/docker/` | Docker Engine API (REST) | `/var/run/docker.sock` |
| Podman | `pkg/runtime/docker/` | Docker-compat API (REST) | `/run/podman/podman.sock`, rootless paths |

Podman uses the same Docker SDK backend since it exposes a Docker-compatible API.

**Key files:**
- `pkg/runtime/runtime.go` — `Runtime` interface, `ContainerInfo`, `ResolveOptions`, shared `ResolveByFilter()` helper
- `pkg/runtime/detect.go` — Socket auto-detection, probes 8 paths including rootless (`$XDG_RUNTIME_DIR`)
- `pkg/runtime/cri/cri.go` — CRI gRPC backend (SPDY streaming for interactive exec)
- `pkg/runtime/docker/docker.go` — Docker/Podman backend (uses `stdcopy.StdCopy` for stream demux)

### 3-Tier Shell Fallback (`pkg/shell/`)

1. **Exec** (`exec.go`) — Direct exec into container (probes for `/bin/bash`, `/bin/sh`, `/bin/ash`, `/bin/zsh`)
2. **Debug Container** (`debug.go`) — K8s ephemeral container injection or CRI-level debug
3. **Nsenter** (`nsenter.go`) — Direct namespace entry from host (requires root/CAP_SYS_ADMIN)

Chain defined in `strategy.go` via `FallbackChain()` and `DefaultStrategies()`.

### Debug Toolkit (`pkg/debug/`)

11 debug commands: `logs`, `inspect`, `top`, `netstat`, `tcpdump`, `strace`, `cp`, `portfw`, `env`, `fs`. All take `runtime.Runtime` as their first runtime parameter. Most use `ExecSync()` or `nsenter` via helpers in `helpers.go`.

### Container Picker (`pkg/picker/`)

Interactive TUI picker using bubbletea for selecting containers when no target is specified via flags.

### CLI (`cmd/containershell/`)

- `root.go` — Global flags: `--socket`, `--runtime` (auto|cri|docker|podman), `--pod`, `--namespace`, `--name`, `--container-id`, `--verbose`
- `shell.go` — Default command, `connectRuntime()` factory, `resolveOrPick()` helper
- `debug_cmds.go` — All debug subcommands, `withContainer()` helper pattern
- `completion.go` — Shell completion generation (bash/zsh/fish/powershell)

## Build

```bash
make build        # builds ./containershell with version/commit/date ldflags
make test         # go test ./... -v
make install      # installs to /usr/local/bin/
make completion   # generates shell completions
make clean        # removes binary
```

If `go` is not in PATH, use `/usr/local/go/bin/go` directly.

## Conventions

- All runtime backends must satisfy `runtime.Runtime` interface with compile-time check: `var _ runtime.Runtime = (*Runtime)(nil)`
- Kubernetes labels (`io.kubernetes.container.name`, `io.kubernetes.pod.name`, `io.kubernetes.pod.namespace`) must be extracted consistently across all backends
- Docker non-TTY streams require `stdcopy.StdCopy` for proper stdout/stderr demultiplexing
- The `--cri-socket` flag is deprecated in favor of `--socket`
- Debug commands use the `withContainer()` helper pattern in `debug_cmds.go`
