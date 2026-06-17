package dashboard

import "time"

// Config holds dashboard configuration options.
type Config struct {
	// RefreshInterval is the polling interval for the container list.
	// Valid range: [1s, 60s]. Defaults to 5s.
	RefreshInterval time.Duration

	// RuntimeType specifies the container runtime to connect to.
	// Valid values: "auto", "cri", "docker", "podman".
	RuntimeType string

	// SocketPath is the path to the runtime socket.
	// Empty string means auto-detect.
	SocketPath string

	// Verbose enables verbose logging of shell tier attempts.
	Verbose bool
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		RefreshInterval: 5 * time.Second,
		RuntimeType:     "auto",
		SocketPath:      "",
		Verbose:         false,
	}
}
