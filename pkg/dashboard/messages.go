package dashboard

import (
	"time"

	"github.com/containershell/containershell/pkg/runtime"
)

// tickMsg is sent on each polling interval to trigger a container list refresh.
type tickMsg time.Time

// containersLoadedMsg carries the result of a ListContainers call.
type containersLoadedMsg struct {
	containers []runtime.ContainerInfo
	err        error
}

// runtimeInfoMsg carries the result of a RuntimeInfo call.
type runtimeInfoMsg struct {
	info *runtime.RuntimeInfo
	err  error
}

// debugOutputMsg carries the result of a debug command execution.
// forDetail routes the result to the detail panel instead of the overlay;
// view identifies which detail tab requested it, so stale responses (the tab
// changed while the command ran) can be dropped.
type debugOutputMsg struct {
	title     string
	content   string
	err       error
	forDetail bool
	view      DetailView
}

// shellDoneMsg is sent when a shell session ends.
type shellDoneMsg struct {
	err error
}

// logLineMsg carries a single log line received during follow mode.
type logLineMsg string
