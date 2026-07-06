package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempSocket creates an empty file named `name` in a temp dir and returns its
// path. resolveEndpoint only stats the path, so a regular file is enough to
// stand in for a socket, and the name drives runtime-type inference.
func tempSocket(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatalf("write temp socket: %v", err)
	}
	return p
}

// clearEnv removes the daemon env vars so a test starts from a known state.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("CONTAINER_HOST", "")
}

func TestSplitScheme(t *testing.T) {
	cases := []struct{ in, scheme, rest string }{
		{"/var/run/docker.sock", "", "/var/run/docker.sock"},
		{"unix:///run/docker.sock", "unix", "/run/docker.sock"},
		{"tcp://1.2.3.4:2375", "tcp", "1.2.3.4:2375"},
		{"ssh://user@host", "ssh", "user@host"},
		{"", "", ""},
	}
	for _, c := range cases {
		gotScheme, gotRest := splitScheme(c.in)
		if gotScheme != c.scheme || gotRest != c.rest {
			t.Errorf("splitScheme(%q) = (%q, %q), want (%q, %q)", c.in, gotScheme, gotRest, c.scheme, c.rest)
		}
	}
}

func TestInferRuntimeType(t *testing.T) {
	cases := map[string]RuntimeType{
		"/run/containerd/containerd.sock": RuntimeCRI,
		"/run/crio/crio.sock":             RuntimeCRI,
		"/var/run/docker.sock":            RuntimeDocker,
		"/run/podman/podman.sock":         RuntimePodman,
		"/some/weird/thing.sock":          RuntimeDocker, // default
	}
	for path, want := range cases {
		if got := inferRuntimeType(path); got != want {
			t.Errorf("inferRuntimeType(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestResolveEndpoint_UnixPathAndURL(t *testing.T) {
	sock := tempSocket(t, "docker.sock")

	// Bare filesystem path.
	ep, rt, err := resolveEndpoint(sock, RuntimeAuto)
	if err != nil {
		t.Fatalf("bare path: unexpected error: %v", err)
	}
	if ep != sock {
		t.Errorf("bare path: endpoint = %q, want %q", ep, sock)
	}
	if rt != RuntimeDocker {
		t.Errorf("bare path: type = %q, want docker", rt)
	}

	// unix:// URL resolves to the same bare path.
	ep, rt, err = resolveEndpoint("unix://"+sock, RuntimeAuto)
	if err != nil {
		t.Fatalf("unix url: unexpected error: %v", err)
	}
	if ep != sock {
		t.Errorf("unix url: endpoint = %q, want %q", ep, sock)
	}
	if rt != RuntimeDocker {
		t.Errorf("unix url: type = %q, want docker", rt)
	}
}

func TestResolveEndpoint_MissingSocketErrors(t *testing.T) {
	if _, _, err := resolveEndpoint("/no/such/place/docker.sock", RuntimeAuto); err == nil {
		t.Fatal("expected error for missing socket, got nil")
	}
	if _, _, err := resolveEndpoint("unix:///no/such/place/docker.sock", RuntimeDocker); err == nil {
		t.Fatal("expected error for missing unix:// socket, got nil")
	}
}

func TestResolveEndpoint_RemoteSchemePassthrough(t *testing.T) {
	for _, ep := range []string{"tcp://1.2.3.4:2375", "ssh://user@host", "npipe:////./pipe/docker_engine"} {
		got, rt, err := resolveEndpoint(ep, RuntimeAuto)
		if err != nil {
			t.Errorf("resolveEndpoint(%q): unexpected error: %v", ep, err)
		}
		if got != ep {
			t.Errorf("resolveEndpoint(%q): endpoint = %q, want passthrough", ep, got)
		}
		if rt != RuntimeDocker {
			t.Errorf("resolveEndpoint(%q): type = %q, want docker default", ep, rt)
		}
	}
}

func TestResolveEndpoint_RemoteSchemeKeepsExplicitType(t *testing.T) {
	got, rt, err := resolveEndpoint("tcp://1.2.3.4:2375", RuntimePodman)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "tcp://1.2.3.4:2375" || rt != RuntimePodman {
		t.Errorf("got (%q, %q), want (tcp://1.2.3.4:2375, podman)", got, rt)
	}
}

func TestEndpointFromEnv(t *testing.T) {
	t.Run("auto prefers DOCKER_HOST", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "unix:///a.sock")
		t.Setenv("CONTAINER_HOST", "unix:///b.sock")
		ep, rt, name := endpointFromEnv(RuntimeAuto)
		if ep != "unix:///a.sock" || rt != RuntimeDocker || name != "DOCKER_HOST" {
			t.Errorf("got (%q, %q, %q), want docker DOCKER_HOST", ep, rt, name)
		}
	})

	t.Run("auto falls to CONTAINER_HOST", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		t.Setenv("CONTAINER_HOST", "unix:///b.sock")
		ep, rt, name := endpointFromEnv(RuntimeAuto)
		if ep != "unix:///b.sock" || rt != RuntimePodman || name != "CONTAINER_HOST" {
			t.Errorf("got (%q, %q, %q), want podman CONTAINER_HOST", ep, rt, name)
		}
	})

	t.Run("podman honors DOCKER_HOST fallback", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "unix:///a.sock")
		t.Setenv("CONTAINER_HOST", "")
		ep, rt, name := endpointFromEnv(RuntimePodman)
		if ep != "unix:///a.sock" || rt != RuntimePodman || name != "DOCKER_HOST" {
			t.Errorf("got (%q, %q, %q), want podman via DOCKER_HOST", ep, rt, name)
		}
	})

	t.Run("cri ignores env", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "unix:///a.sock")
		t.Setenv("CONTAINER_HOST", "unix:///b.sock")
		if ep, _, _ := endpointFromEnv(RuntimeCRI); ep != "" {
			t.Errorf("CRI should ignore env, got %q", ep)
		}
	})

	t.Run("whitespace is treated as unset", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "   ")
		t.Setenv("CONTAINER_HOST", "")
		if ep, _, _ := endpointFromEnv(RuntimeAuto); ep != "" {
			t.Errorf("blank DOCKER_HOST should be ignored, got %q", ep)
		}
	})
}

func TestDetectSocket_DockerHostUnix(t *testing.T) {
	clearEnv(t)
	sock := tempSocket(t, "docker.sock")
	t.Setenv("DOCKER_HOST", "unix://"+sock)

	ep, rt, err := DetectSocket("", RuntimeAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != sock || rt != RuntimeDocker {
		t.Errorf("got (%q, %q), want (%q, docker)", ep, rt, sock)
	}
}

func TestDetectSocket_DockerHostTCP(t *testing.T) {
	clearEnv(t)
	t.Setenv("DOCKER_HOST", "tcp://1.2.3.4:2375")

	ep, rt, err := DetectSocket("", RuntimeAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "tcp://1.2.3.4:2375" || rt != RuntimeDocker {
		t.Errorf("got (%q, %q), want (tcp://1.2.3.4:2375, docker)", ep, rt)
	}
}

func TestDetectSocket_ContainerHostPodman(t *testing.T) {
	clearEnv(t)
	sock := tempSocket(t, "podman.sock")
	t.Setenv("CONTAINER_HOST", "unix://"+sock)

	ep, rt, err := DetectSocket("", RuntimeAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != sock || rt != RuntimePodman {
		t.Errorf("got (%q, %q), want (%q, podman)", ep, rt, sock)
	}
}

func TestDetectSocket_ExplicitOverridesEnv(t *testing.T) {
	clearEnv(t)
	envSock := tempSocket(t, "docker.sock")
	flagSock := tempSocket(t, "podman.sock")
	t.Setenv("DOCKER_HOST", "unix://"+envSock)

	ep, rt, err := DetectSocket(flagSock, RuntimeAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != flagSock || rt != RuntimePodman {
		t.Errorf("explicit --socket should win: got (%q, %q), want (%q, podman)", ep, rt, flagSock)
	}
}

func TestDetectSocket_DockerHostMissingReports(t *testing.T) {
	clearEnv(t)
	t.Setenv("DOCKER_HOST", "unix:///definitely/not/here/docker.sock")

	_, _, err := DetectSocket("", RuntimeAuto)
	if err == nil {
		t.Fatal("expected error for missing DOCKER_HOST socket, got nil")
	}
	if !strings.Contains(err.Error(), "DOCKER_HOST") {
		t.Errorf("error should name DOCKER_HOST, got: %v", err)
	}
}

func TestDetectSocket_CRIIgnoresDockerHost(t *testing.T) {
	clearEnv(t)
	t.Setenv("DOCKER_HOST", "tcp://1.2.3.4:2375")

	ep, _, err := DetectSocket("", RuntimeCRI)
	if err == nil && strings.HasPrefix(ep, "tcp://") {
		t.Errorf("CRI detection must not pick up DOCKER_HOST, got %q", ep)
	}
}

func TestDefaultSocketCandidates_IncludesDesktopAndRootless(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/4242")
	var haveDesktop, haveRootless bool
	for _, c := range DefaultSocketCandidates() {
		if strings.Contains(c.Path, filepath.Join(".docker", "run", "docker.sock")) {
			haveDesktop = true
		}
		if c.Path == "/run/user/4242/docker.sock" {
			haveRootless = true
		}
	}
	if !haveDesktop {
		t.Error("candidate list should include the Docker Desktop socket path")
	}
	if !haveRootless {
		t.Error("candidate list should include the XDG rootless docker socket path")
	}
}
