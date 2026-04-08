package namespace

import (
	"testing"
)

func TestNsPath(t *testing.T) {
	got := NsPath(12345, PID)
	want := "/proc/12345/ns/pid"
	if got != want {
		t.Errorf("NsPath(12345, PID) = %q, want %q", got, want)
	}
}

func TestNsenterCmd(t *testing.T) {
	cmd := NsenterCmd(42, AllTypes(), "/bin/sh")
	args := cmd.Args
	if args[0] != "nsenter" {
		t.Errorf("expected nsenter command, got %s", args[0])
	}
	found := false
	for _, a := range args {
		if a == "--target=42" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --target=42 in args: %v", args)
	}
	// Check all namespace flags are present
	expectedFlags := map[string]bool{"--pid": false, "--net": false, "--mount": false, "--uts": false, "--ipc": false}
	for _, a := range args {
		if _, ok := expectedFlags[a]; ok {
			expectedFlags[a] = true
		}
	}
	for flag, found := range expectedFlags {
		if !found {
			t.Errorf("expected %s in args: %v", flag, args)
		}
	}
	// Last arg should be the shell
	if args[len(args)-1] != "/bin/sh" {
		t.Errorf("expected last arg to be /bin/sh, got %s", args[len(args)-1])
	}
}

func TestAllTypes(t *testing.T) {
	types := AllTypes()
	if len(types) != 5 {
		t.Fatalf("expected 5 namespace types, got %d", len(types))
	}
}

func TestValidatePid_Self(t *testing.T) {
	if err := ValidatePid(1); err != nil {
		t.Errorf("PID 1 should be accessible: %v", err)
	}
}

func TestValidatePid_Invalid(t *testing.T) {
	if err := ValidatePid(999999999); err == nil {
		t.Error("expected error for nonexistent PID")
	}
}
