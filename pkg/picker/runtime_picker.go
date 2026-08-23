package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/containershell/containershell/pkg/runtime"
	"github.com/containershell/containershell/pkg/tui"
)

type runtimeModel struct {
	runtimes []runtime.DetectedRuntime
	cursor   int
	selected *runtime.DetectedRuntime
	quitting bool
}

func (m runtimeModel) Init() tea.Cmd { return nil }

func (m runtimeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "ctrl+n":
			if m.cursor < len(m.runtimes)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.runtimes) > 0 {
				sel := m.runtimes[m.cursor]
				m.selected = &sel
			}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m runtimeModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(tui.BadgeStyle.Render(" " + tui.Logo + " containershell "))
	b.WriteString(tui.TitleStyle.Render(" select a runtime"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("↑/↓ move · Enter select · Esc cancel"))
	b.WriteString("\n\n")

	for i, r := range m.runtimes {
		if i == m.cursor {
			line := fmt.Sprintf("▸ %-8s %s", runtimeLabel(r.RuntimeType), r.Endpoint)
			b.WriteString(tui.SelectedStyle.Render(line))
		} else {
			b.WriteString("  " + fmt.Sprintf("%-8s", runtimeLabel(r.RuntimeType)))
			b.WriteString(tui.DimStyle.Render(" " + r.Endpoint))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func runtimeLabel(t runtime.RuntimeType) string {
	switch t {
	case runtime.RuntimeDocker:
		return "Docker"
	case runtime.RuntimePodman:
		return "Podman"
	case runtime.RuntimeCRI:
		return "CRI"
	default:
		return string(t)
	}
}

// PickRuntime prompts the user to choose among the detected runtimes. With zero
// or one runtime it returns immediately without prompting.
func PickRuntime(runtimes []runtime.DetectedRuntime) (*runtime.DetectedRuntime, error) {
	switch len(runtimes) {
	case 0:
		return nil, fmt.Errorf("no container runtimes detected")
	case 1:
		return &runtimes[0], nil
	}

	result, err := tea.NewProgram(runtimeModel{runtimes: runtimes}).Run()
	if err != nil {
		return nil, fmt.Errorf("runtime picker failed: %w", err)
	}

	final := result.(runtimeModel)
	if final.selected == nil {
		return nil, fmt.Errorf("cancelled by user")
	}
	return final.selected, nil
}
