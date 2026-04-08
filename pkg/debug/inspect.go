package debug

import (
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
