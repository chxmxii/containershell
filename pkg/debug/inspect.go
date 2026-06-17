package debug

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/containershell/containershell/pkg/runtime"
)

// Inspect prints detailed container metadata.
func Inspect(ctx context.Context, rt runtime.Runtime, containerID string) error {
	info, err := rt.ContainerStatus(ctx, containerID)
	if err != nil {
		return err
	}

	fmt.Println("--- Container Info ---")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}

// InspectOutput returns detailed container metadata as a JSON string.
func InspectOutput(ctx context.Context, rt runtime.Runtime, containerID string) (string, error) {
	info, err := rt.ContainerStatus(ctx, containerID)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.WriteString("--- Container Info ---\n")
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(info); err != nil {
		return "", err
	}
	return buf.String(), nil
}
