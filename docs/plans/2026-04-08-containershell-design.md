# ContainerShell Design

## Purpose

A single Go binary that guarantees shell access to any running container using a 3-tier fallback strategy, plus a full debugging toolkit — a Swiss Army knife for container debugging.

## 3-Tier Shell Access (Cascading Fallback)

| Tier | Method | When it works | When it fails |
|------|--------|---------------|---------------|
| 1 | CRI Exec | Container has a shell binary (`/bin/sh`, `/bin/bash`) | Distroless/scratch images with no shell |
| 2 | Debug Container Injection | K8s ephemeral container (if kubeconfig available) or CRI-level debug container sharing target namespaces | No K8s API + CRI doesn't support it |
| 3 | nsenter | Direct namespace entry (pid, net, mnt, uts, ipc) from host — always works if container is running | Requires root/CAP_SYS_ADMIN on the host |

Each tier is attempted in order. On failure, the tool logs why it failed and transparently moves to the next tier.

## CRI Backend

Supports both containerd and CRI-O via the CRI gRPC API:

- Auto-detects socket at `/run/containerd/containerd.sock` and `/var/run/crio/crio.sock`
- Uses `k8s.io/cri-api` for runtime-agnostic CRI calls
- Custom socket path via `--cri-socket` flag

## Container Targeting

### Interactive Picker (default)

- Lists all running containers with name, image, pod, namespace, uptime
- Fuzzy-searchable interactive TUI using bubbletea
- Graceful fallback to simple list selection if terminal doesn't support TUI

### Direct Targeting (scripted use)

- `--container-id` — target by CRI container ID
- `--pod` / `--namespace` — K8s-aware lookup
- `--name` — match by container name

## Debug Toolkit Subcommands

| Command | Description |
|---------|-------------|
| `shell` | Interactive shell (default, uses 3-tier fallback) |
| `logs` | Stream container logs |
| `inspect` | Container metadata, config, mounts, env |
| `top` | Process list inside the container |
| `netstat` | Network connections and listeners |
| `tcpdump` | Packet capture on container's network namespace |
| `strace` | Trace syscalls of a process in the container |
| `cp` | Copy files in/out of the container filesystem |
| `portfw` | Port forward from host to container |
| `env` | Dump environment variables |
| `fs` | Browse/search the container filesystem |
| `completion` | Generate shell completions (bash/zsh/fish/powershell) |

## Shell Completion

- Cobra built-in `completion` subcommand for bash, zsh, fish, powershell
- Dynamic completions for container IDs, pod names, and namespaces via `ValidArgsFunction`
- Custom completions for `--cri-socket` (suggests known socket paths)

## Project Structure

```
containershell/
├── cmd/containershell/     # main entry, CLI (cobra)
├── pkg/
│   ├── cri/                # CRI gRPC client (containerd + CRI-O)
│   ├── shell/              # 3-tier shell strategy
│   │   ├── exec.go         # Tier 1: CRI exec
│   │   ├── debug.go        # Tier 2: debug container injection
│   │   └── nsenter.go      # Tier 3: nsenter
│   ├── picker/             # Interactive container picker TUI
│   ├── debug/              # Toolkit subcommands (logs, top, tcpdump, etc.)
│   ├── k8s/                # Optional K8s API client for ephemeral containers
│   └── namespace/          # Linux namespace helpers
├── go.mod
└── go.sum
```

## Key Dependencies

- `google.golang.org/grpc` — CRI socket communication
- `k8s.io/cri-api` — CRI protobuf definitions
- `k8s.io/client-go` — K8s API for ephemeral containers
- `github.com/spf13/cobra` — CLI framework
- `github.com/charmbracelet/bubbletea` — Interactive TUI picker
- `golang.org/x/sys/unix` — nsenter / namespace syscalls

## Error Handling

- Each tier returns a structured error explaining why it failed (no shell binary, no K8s API, insufficient privileges)
- Final error aggregates all three failures with clear guidance on what the user needs to fix
- `--verbose` flag shows the fallback chain in real time
