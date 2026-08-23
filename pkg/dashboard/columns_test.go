package dashboard

import "testing"

// rowWidth returns the total display width of a row assembled from these columns:
// the 2-col marker, each visible column, and a gap before every column after the
// first.
func (c listColumns) rowWidth() int {
	total := listMarkerWidth + c.name
	for _, w := range []int{c.pod, c.ns, c.state, c.age} {
		if w > 0 {
			total += listColGap + w
		}
	}
	return total
}

// TestComputeColumnsNeverExceedsWidth is the invariant that prevents wrapping:
// an assembled row must always fit within the panel width.
func TestComputeColumnsNeverExceedsWidth(t *testing.T) {
	for w := 0; w <= 200; w++ {
		for _, hasPod := range []bool{false, true} {
			for _, hasNs := range []bool{false, true} {
				c := computeColumns(w, hasPod, hasNs)

				// Only meaningful once there is room for the marker + one column.
				if w-listMarkerWidth >= 1 {
					if got := c.rowWidth(); got > w {
						t.Errorf("computeColumns(%d,%v,%v) row width %d exceeds %d (%+v)",
							w, hasPod, hasNs, got, w, c)
					}
					if c.name < 1 {
						t.Errorf("computeColumns(%d,%v,%v) hid the name column (%+v)",
							w, hasPod, hasNs, c)
					}
				}

				if !hasPod && c.pod != 0 {
					t.Errorf("computeColumns(%d,%v,%v) showed pod despite hasPod=false", w, hasPod, hasNs)
				}
				if !hasNs && c.ns != 0 {
					t.Errorf("computeColumns(%d,%v,%v) showed ns despite hasNs=false", w, hasPod, hasNs)
				}
			}
		}
	}
}

// TestComputeColumnsHidesPodNsForDocker checks that a plain Docker/Podman list
// (no pods, no namespaces) spends the width on name/state/age instead.
func TestComputeColumnsHidesPodNsForDocker(t *testing.T) {
	c := computeColumns(70, false, false)
	if c.pod != 0 || c.ns != 0 {
		t.Errorf("pod/ns should be hidden for non-k8s lists, got %+v", c)
	}
	if c.name == 0 || c.state == 0 || c.age == 0 {
		t.Errorf("name/state/age should all be shown at width 70, got %+v", c)
	}
}

// TestComputeColumnsShowsPodNsWhenWide checks that a wide panel with k8s metadata
// present surfaces every column.
func TestComputeColumnsShowsPodNsWhenWide(t *testing.T) {
	c := computeColumns(80, true, true)
	if c.name == 0 || c.pod == 0 || c.ns == 0 || c.state == 0 || c.age == 0 {
		t.Errorf("all columns should be visible at width 80 with k8s metadata, got %+v", c)
	}
}

// TestComputeColumnsDegradesGracefully checks the priority order: as width
// shrinks, the least important columns (pod, then namespace) drop first while
// name, state, and age survive.
func TestComputeColumnsDegradesGracefully(t *testing.T) {
	narrow := computeColumns(30, true, true)
	if narrow.name == 0 || narrow.state == 0 || narrow.age == 0 {
		t.Errorf("name/state/age should survive at width 30, got %+v", narrow)
	}
}
