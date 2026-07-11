package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeType identifies a container runtime backend.
type RuntimeType string

const (
	RuntimeAuto   RuntimeType = "auto"
	RuntimeCRI    RuntimeType = "cri"
	RuntimeDocker RuntimeType = "docker"
	RuntimePodman RuntimeType = "podman"
)

// SocketCandidate represents a runtime socket to probe.
type SocketCandidate struct {
	Path        string
	RuntimeType RuntimeType
	Name        string
}

// DefaultSocketCandidates returns the ordered list of sockets to probe.
func DefaultSocketCandidates() []SocketCandidate {
	candidates := []SocketCandidate{
		{"/run/containerd/containerd.sock", RuntimeCRI, "containerd"},
		{"/var/run/containerd/containerd.sock", RuntimeCRI, "containerd"},
		{"/var/run/crio/crio.sock", RuntimeCRI, "CRI-O"},
		{"/run/crio/crio.sock", RuntimeCRI, "CRI-O"},
		{"/var/run/docker.sock", RuntimeDocker, "Docker"},
		{"/run/docker.sock", RuntimeDocker, "Docker"},
		{"/run/podman/podman.sock", RuntimePodman, "Podman"},
		{"/var/run/podman/podman.sock", RuntimePodman, "Podman"},
	}

	// Docker Desktop and rootless installs place the socket under the user's
	// home or XDG runtime directory rather than /var/run.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			SocketCandidate{filepath.Join(home, ".docker", "run", "docker.sock"), RuntimeDocker, "Docker Desktop"},
			SocketCandidate{filepath.Join(home, ".docker", "desktop", "docker.sock"), RuntimeDocker, "Docker Desktop"},
		)
	}

	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		xdgRuntime = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	candidates = append(candidates,
		SocketCandidate{filepath.Join(xdgRuntime, "podman", "podman.sock"), RuntimePodman, "Podman (rootless)"},
		SocketCandidate{filepath.Join(xdgRuntime, "docker.sock"), RuntimeDocker, "Docker (rootless)"},
	)

	return candidates
}

// DetectSocket resolves a runtime endpoint, honoring in order: an explicit
// endpoint (from --socket), the DOCKER_HOST/CONTAINER_HOST environment, then a
// probe of well-known socket locations. The returned endpoint is either a
// filesystem socket path or a scheme-qualified host URL (e.g. tcp://host:2375)
// understood by the runtime client.
func DetectSocket(endpoint string, runtimeType RuntimeType) (string, RuntimeType, error) {
	// 1. An explicit endpoint always wins.
	if endpoint != "" {
		ep, rt, err := resolveEndpoint(endpoint, runtimeType)
		if err != nil {
			return "", "", fmt.Errorf("specified socket %q: %w", endpoint, err)
		}
		return ep, rt, nil
	}

	// 2. DOCKER_HOST / CONTAINER_HOST — how Docker and Podman advertise a
	//    non-default daemon location. This is the common reason auto-detection
	//    would otherwise miss a daemon whose socket is not in a standard path.
	if envEP, envType, envVar := endpointFromEnv(runtimeType); envEP != "" {
		ep, rt, err := resolveEndpoint(envEP, envType)
		if err != nil {
			return "", "", fmt.Errorf("%s=%q: %w", envVar, envEP, err)
		}
		return ep, rt, nil
	}

	// 3. Probe well-known socket paths on the filesystem.
	for _, c := range DefaultSocketCandidates() {
		if runtimeType != RuntimeAuto && c.RuntimeType != runtimeType {
			continue
		}
		if _, err := os.Stat(c.Path); err == nil {
			return c.Path, c.RuntimeType, nil
		}
	}

	return "", "", fmt.Errorf("no container runtime socket found (checked DOCKER_HOST/CONTAINER_HOST and known CRI, Docker, and Podman socket paths); use --socket to specify one")
}

// endpointFromEnv returns the endpoint advertised via the environment for the
// requested runtime type (or the first one found when auto), the runtime type
// it implies, and the variable it came from. The endpoint is empty when nothing
// applicable is set. CRI has no such convention and is never sourced here.
func endpointFromEnv(runtimeType RuntimeType) (endpoint string, rtType RuntimeType, envVar string) {
	try := func(name string, t RuntimeType) bool {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			endpoint, rtType, envVar = v, t, name
			return true
		}
		return false
	}

	switch runtimeType {
	case RuntimeDocker:
		try("DOCKER_HOST", RuntimeDocker)
	case RuntimePodman:
		// Podman's native variable is CONTAINER_HOST, but it also honors
		// DOCKER_HOST for Docker-CLI compatibility.
		_ = try("CONTAINER_HOST", RuntimePodman) || try("DOCKER_HOST", RuntimePodman)
	case RuntimeAuto:
		_ = try("DOCKER_HOST", RuntimeDocker) || try("CONTAINER_HOST", RuntimePodman)
	}
	return
}

// resolveEndpoint validates and normalizes a single endpoint. Filesystem socket
// paths and unix:// URLs are verified to exist and returned as a bare path;
// remote schemes (tcp, ssh, http(s), npipe) cannot be stat'd and are passed
// through verbatim for the client to validate on connect.
func resolveEndpoint(endpoint string, runtimeType RuntimeType) (string, RuntimeType, error) {
	scheme, rest := splitScheme(endpoint)

	switch scheme {
	case "", "unix":
		path := endpoint
		if scheme == "unix" {
			path = rest
		}
		if path == "" {
			return "", "", fmt.Errorf("empty socket path")
		}
		if _, err := os.Stat(path); err != nil {
			return "", "", fmt.Errorf("socket not found: %s: %w", path, err)
		}
		if runtimeType == RuntimeAuto {
			runtimeType = inferRuntimeType(path)
		}
		return path, runtimeType, nil

	default:
		// A remote or abstract endpoint. It cannot map to a CRI socket, and the
		// scheme carries no docker/podman hint, so default to Docker when auto.
		if runtimeType == RuntimeAuto {
			runtimeType = RuntimeDocker
		}
		return endpoint, runtimeType, nil
	}
}

// splitScheme splits a "scheme://rest" endpoint. A bare path (no "://") returns
// an empty scheme and the endpoint unchanged.
func splitScheme(endpoint string) (scheme, rest string) {
	if i := strings.Index(endpoint, "://"); i >= 0 {
		return endpoint[:i], endpoint[i+len("://"):]
	}
	return "", endpoint
}

func inferRuntimeType(socketPath string) RuntimeType {
	base := filepath.Base(socketPath)
	switch {
	case base == "containerd.sock" || base == "crio.sock":
		return RuntimeCRI
	case base == "docker.sock":
		return RuntimeDocker
	case base == "podman.sock":
		return RuntimePodman
	default:
		return RuntimeDocker
	}
}

// DetectedRuntime is an available runtime endpoint discovered on the host.
type DetectedRuntime struct {
	Endpoint    string
	RuntimeType RuntimeType
	Name        string
}

// DetectAllRuntimes returns every runtime endpoint currently reachable on the
// host: the DOCKER_HOST / CONTAINER_HOST endpoints when set, plus every
// well-known socket path that exists. Entries are de-duplicated (following
// symlinks) and ordered environment-first, then by socket-probe priority. It is
// the basis for automatic runtime selection when --runtime is left on "auto".
func DetectAllRuntimes() []DetectedRuntime {
	var raw []DetectedRuntime

	for _, e := range []struct {
		env string
		typ RuntimeType
	}{
		{"DOCKER_HOST", RuntimeDocker},
		{"CONTAINER_HOST", RuntimePodman},
	} {
		v := strings.TrimSpace(os.Getenv(e.env))
		if v == "" {
			continue
		}
		if ep, typ, err := resolveEndpoint(v, e.typ); err == nil {
			raw = append(raw, DetectedRuntime{Endpoint: ep, RuntimeType: typ, Name: e.env})
		}
	}

	for _, c := range DefaultSocketCandidates() {
		if _, err := os.Stat(c.Path); err == nil {
			raw = append(raw, DetectedRuntime{Endpoint: c.Path, RuntimeType: c.RuntimeType, Name: c.Name})
		}
	}

	return dedupeRuntimes(raw)
}

// dedupeRuntimes drops entries that resolve to the same socket. Filesystem paths
// are compared after following symlinks (so /var/run/docker.sock and
// /run/docker.sock collapse to a single Docker entry); non-path endpoints such
// as tcp:// URLs compare verbatim.
func dedupeRuntimes(in []DetectedRuntime) []DetectedRuntime {
	seen := make(map[string]bool, len(in))
	var out []DetectedRuntime
	for _, r := range in {
		key := r.Endpoint
		if real, err := filepath.EvalSymlinks(r.Endpoint); err == nil {
			key = real
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}
