# ContainerShell

Swiss Army knife for container debugging. Guarantees shell access to any running container (Including Distroless containers) using a 3-tier fallback strategy, plus a full debugging toolkit.

Works with containerd, CRI-O, Docker, and Podman. Auto-detects the runtime.

## How It Works

ContainerShell tries three methods to get you a shell, falling through automatically:

1. **Exec** -- runs a shell directly inside the container (probes for bash, sh, ash, zsh)
2. **Debug container** -- injects a K8s ephemeral container or CRI-level debug sidecar
3. **Nsenter** -- enters the container's namespaces from the host (requires root)

If one tier fails, the next one kicks in. You always get a shell.

## Install

```bash
git clone https://github.com/chxmxii/containershell.git
cd containershell
make build
sudo make install
```

Requires Go 1.25+.

## Usage

```bash
# Interactive shell (auto-picks runtime + container via TUI picker)
containershell

# Target a specific container
containershell --name nginx
containershell --pod web-pod --namespace production
containershell --container-id abc123def456

# Specify runtime explicitly
containershell --runtime docker --name myapp
containershell --socket /run/containerd/containerd.sock
```

## Debug Commands

```bash
containershell logs --follow --tail 50      # Stream container logs
containershell inspect                       # Container metadata, config, mounts
containershell top                           # Processes inside the container
containershell env                           # Environment variables
containershell netstat                       # Network connections and listeners
containershell tcpdump -i eth0 -c 100       # Packet capture
containershell strace --follow-forks         # Syscall tracing
containershell fs --path /app --recursive    # Browse container filesystem
containershell cp --dir from --src /etc/hosts --dst ./hosts  # Copy files out
containershell cp --dir to --src ./config.yaml --dst /app/   # Copy files in
containershell portfw --local 8080 --remote 80               # Port forwarding
```

All debug commands accept the same targeting flags (`--name`, `--pod`, `--namespace`, `--container-id`). If no target is specified, the interactive picker launches.

## Supported Runtimes

| Runtime | Detection | Socket |
|---------|-----------|--------|
| containerd | auto | `/run/containerd/containerd.sock` |
| CRI-O | auto | `/var/run/crio/crio.sock` |
| Docker | auto | `/var/run/docker.sock` |
| Podman | auto | `/run/podman/podman.sock` |

Rootless sockets (via `$XDG_RUNTIME_DIR`) are also probed automatically.

## Shell Completions

```bash
# Bash
source <(containershell completion bash)

# Zsh
source <(containershell completion zsh)

# Fish
containershell completion fish | source
```

## Build

```bash
make build       # Build binary with version info
make test        # Run tests
make lint        # Run golangci-lint
make install     # Install to /usr/local/bin
make clean       # Remove binary
```

## License

MIT
