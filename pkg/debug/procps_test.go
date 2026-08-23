package debug

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// ProcTop rooted at our own PID must list this test process without any
// privileges — the property the container fallback relies on.
func TestProcTopIncludesSelf(t *testing.T) {
	out, err := ProcTop(os.Getpid())
	if err != nil {
		t.Fatalf("ProcTop failed: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("%7d ", os.Getpid())) {
		t.Fatalf("output does not list our own PID %d:\n%s", os.Getpid(), out)
	}
	if !strings.HasPrefix(out, fmt.Sprintf("%7s %-10s %s\n", "PID", "USER", "COMMAND")) {
		t.Fatalf("missing header:\n%s", out)
	}
}

// ProcTreePids rooted at our parent must include both the parent and us.
func TestProcTreePidsIncludesDescendants(t *testing.T) {
	pids, err := ProcTreePids(os.Getppid())
	if err != nil {
		t.Fatalf("ProcTreePids failed: %v", err)
	}
	found := map[int]bool{}
	for _, p := range pids {
		found[p] = true
	}
	if !found[os.Getppid()] || !found[os.Getpid()] {
		t.Fatalf("expected parent %d and self %d in tree, got %v", os.Getppid(), os.Getpid(), pids)
	}
}

func TestProcTopUnknownPid(t *testing.T) {
	// PID 0 never appears as a /proc entry.
	if _, err := ProcTop(0); err == nil {
		t.Fatal("expected an error for a nonexistent PID")
	}
}
