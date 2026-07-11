package picker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/containershell/containershell/pkg/runtime"
)

func twoRuntimes() []runtime.DetectedRuntime {
	return []runtime.DetectedRuntime{
		{Endpoint: "/run/docker.sock", RuntimeType: runtime.RuntimeDocker, Name: "Docker"},
		{Endpoint: "/run/podman/podman.sock", RuntimeType: runtime.RuntimePodman, Name: "Podman"},
	}
}

func TestRuntimeModelSelectsSecond(t *testing.T) {
	m := runtimeModel{runtimes: twoRuntimes()}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(runtimeModel)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(runtimeModel)
	if m.selected == nil || m.selected.RuntimeType != runtime.RuntimePodman {
		t.Fatalf("selected = %+v, want Podman", m.selected)
	}
}

func TestRuntimeModelCancel(t *testing.T) {
	m := runtimeModel{runtimes: twoRuntimes()}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(runtimeModel)
	if m.selected != nil {
		t.Errorf("cancel should leave nothing selected, got %+v", m.selected)
	}
	if !m.quitting {
		t.Error("cancel should set quitting")
	}
}

func TestRuntimeModelCursorClamped(t *testing.T) {
	m := runtimeModel{runtimes: twoRuntimes()}
	// Up at the top stays at 0.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(runtimeModel)
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	// Down past the end clamps to the last index.
	for i := 0; i < 5; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(runtimeModel)
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (clamped)", m.cursor)
	}
}

func TestPickRuntimeShortCircuits(t *testing.T) {
	// Zero runtimes is an error; one runtime returns without prompting.
	if _, err := PickRuntime(nil); err == nil {
		t.Error("expected error for zero runtimes")
	}
	one := twoRuntimes()[:1]
	got, err := PickRuntime(one)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RuntimeType != runtime.RuntimeDocker {
		t.Errorf("got %+v, want Docker", got)
	}
}

func TestRuntimeLabel(t *testing.T) {
	cases := map[runtime.RuntimeType]string{
		runtime.RuntimeDocker: "Docker",
		runtime.RuntimePodman: "Podman",
		runtime.RuntimeCRI:    "CRI",
	}
	for typ, want := range cases {
		if got := runtimeLabel(typ); got != want {
			t.Errorf("runtimeLabel(%q) = %q, want %q", typ, got, want)
		}
	}
}
