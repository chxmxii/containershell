package dashboard

import (
	"sort"
	"strings"

	"github.com/containershell/containershell/pkg/runtime"
)

// NextSortField cycles to the next sort field in the sequence:
// name → pod → namespace → age → state → name
func NextSortField(current SortField) SortField {
	switch current {
	case SortName:
		return SortPod
	case SortPod:
		return SortNamespace
	case SortNamespace:
		return SortAge
	case SortAge:
		return SortState
	case SortState:
		return SortName
	default:
		return SortName
	}
}

// SortContainers returns a new sorted slice without modifying the input.
// Sorting rules:
//   - Name: ascending case-insensitive
//   - Pod: ascending case-insensitive
//   - Namespace: ascending case-insensitive
//   - Age: newest first (CreatedAt descending)
//   - State: ascending case-insensitive
func SortContainers(containers []runtime.ContainerInfo, field SortField) []runtime.ContainerInfo {
	if len(containers) == 0 {
		return nil
	}

	// Create a copy to avoid modifying the input slice
	sorted := make([]runtime.ContainerInfo, len(containers))
	copy(sorted, containers)

	sort.SliceStable(sorted, func(i, j int) bool {
		switch field {
		case SortName:
			return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
		case SortPod:
			return strings.ToLower(sorted[i].PodName) < strings.ToLower(sorted[j].PodName)
		case SortNamespace:
			return strings.ToLower(sorted[i].Namespace) < strings.ToLower(sorted[j].Namespace)
		case SortAge:
			// Newest first = descending CreatedAt
			return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
		case SortState:
			return strings.ToLower(sorted[i].State) < strings.ToLower(sorted[j].State)
		default:
			return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
		}
	})

	return sorted
}

// SortFieldLabel returns the display label for a sort field.
func SortFieldLabel(field SortField) string {
	switch field {
	case SortName:
		return "name"
	case SortPod:
		return "pod"
	case SortNamespace:
		return "namespace"
	case SortAge:
		return "age"
	case SortState:
		return "state"
	default:
		return "name"
	}
}
