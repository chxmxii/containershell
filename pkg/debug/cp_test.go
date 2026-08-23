package debug

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containershell/containershell/pkg/runtime"
)

// fakeRuntime implements runtime.Runtime with a scriptable ExecSync and a
// PID whose /proc entry does not exist, forcing the exec fallbacks.
type fakeRuntime struct {
	execSync func(cmd []string) (stdout, stderr []byte, exitCode int32, err error)
}

func (f *fakeRuntime) ListContainers(context.Context) ([]runtime.ContainerInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) Resolve(context.Context, runtime.ResolveOptions) (*runtime.ContainerInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) ContainerStatus(context.Context, string) (*runtime.ContainerInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) ExecInteractive(context.Context, string, []string, bool) error { return nil }
func (f *fakeRuntime) ExecSync(_ context.Context, _ string, cmd []string, _ int64) ([]byte, []byte, int32, error) {
	return f.execSync(cmd)
}
func (f *fakeRuntime) ContainerPid(context.Context, string) (uint32, error) {
	// Kernel PIDs are capped at 2^22, so this never exists in /proc.
	return 4294967, nil
}
func (f *fakeRuntime) ContainerLogs(context.Context, string, bool, int64, io.Writer) error {
	return nil
}
func (f *fakeRuntime) RuntimeInfo(context.Context) (*runtime.RuntimeInfo, error) { return nil, nil }
func (f *fakeRuntime) Close() error                                              { return nil }

func TestCopyFromContainerExecFallback(t *testing.T) {
	content := []byte("hello from container\n")
	rt := &fakeRuntime{execSync: func(cmd []string) ([]byte, []byte, int32, error) {
		if len(cmd) != 2 || cmd[0] != "cat" || cmd[1] != "/etc/motd" {
			return nil, nil, 1, fmt.Errorf("unexpected command %v", cmd)
		}
		return content, nil, 0, nil
	}}

	dst := filepath.Join(t.TempDir(), "motd")
	if err := CopyFromContainer(context.Background(), rt, "c1", "/etc/motd", dst); err != nil {
		t.Fatalf("CopyFromContainer failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("wrong content: %q (err %v)", got, err)
	}
}

// The chunked base64 writer must reassemble to the exact source bytes. The
// fake executes each generated shell script with the host sh, writing into a
// temp dir standing in for the container filesystem.
func TestCopyToContainerExecFallbackRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	// >1 chunk of pseudo-random binary data, including NUL bytes.
	data := make([]byte, 100*1024)
	for i := range data {
		data[i] = byte(i * 31)
	}
	src := filepath.Join(t.TempDir(), "src.bin")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	fakeRoot := t.TempDir()
	rt := &fakeRuntime{execSync: func(cmd []string) ([]byte, []byte, int32, error) {
		if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
			return nil, nil, 1, fmt.Errorf("unexpected command %v", cmd)
		}
		// Re-anchor the absolute container path inside the fake root.
		script := strings.ReplaceAll(cmd[2], "'/data/out.bin'", "'"+fakeRoot+"/out.bin'")
		out, err := exec.Command("sh", "-c", script).CombinedOutput()
		if err != nil {
			return nil, out, 1, nil
		}
		return nil, nil, 0, nil
	}}

	if err := CopyToContainer(context.Background(), rt, "c1", src, "/data/out.bin"); err != nil {
		t.Fatalf("CopyToContainer failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(fakeRoot, "out.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("round trip corrupted data: got %d bytes, want %d", len(got), len(data))
	}
}

func TestCopyToContainerEmptyFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(src, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var scripts []string
	rt := &fakeRuntime{execSync: func(cmd []string) ([]byte, []byte, int32, error) {
		scripts = append(scripts, cmd[len(cmd)-1])
		return nil, nil, 0, nil
	}}
	if err := CopyToContainer(context.Background(), rt, "c1", src, "/tmp/empty"); err != nil {
		t.Fatalf("CopyToContainer failed: %v", err)
	}
	if len(scripts) != 1 || !strings.Contains(scripts[0], ">") {
		t.Fatalf("expected a single truncating write, got %v", scripts)
	}
}

// Sanity: chunk encoding is plain single-line base64 (no quoting hazards).
func TestBase64ChunkIsShellSafe(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 255})
	if strings.ContainsAny(b64, "'\"$`\\ \n") {
		t.Fatalf("base64 output not shell-safe: %q", b64)
	}
}
