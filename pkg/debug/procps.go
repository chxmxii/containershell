package debug

import (
	"fmt"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
)

// procEntry is one process read from the host /proc.
type procEntry struct {
	pid     int
	ppid    int
	uid     string
	command string
}

// ProcTop lists a container's processes by walking the host /proc process
// tree rooted at the container's init PID. Unlike exec (needs a ps binary in
// the image) or nsenter (needs root), this works unprivileged: status and
// cmdline are world-readable. PIDs are host PIDs.
func ProcTop(initPid int) (string, error) {
	procs, err := readProcTable()
	if err != nil {
		return "", err
	}
	if _, ok := procs[initPid]; !ok {
		return "", fmt.Errorf("process %d not found in /proc", initPid)
	}

	// Collect the init process and all its descendants.
	children := make(map[int][]int, len(procs))
	for pid, p := range procs {
		children[p.ppid] = append(children[p.ppid], pid)
	}
	var pids []int
	stack := []int{initPid}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		pids = append(pids, pid)
		stack = append(stack, children[pid]...)
	}
	sort.Ints(pids)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%7s %-10s %s\n", "PID", "USER", "COMMAND"))
	for _, pid := range pids {
		p := procs[pid]
		b.WriteString(fmt.Sprintf("%7d %-10s %s\n", pid, userName(p.uid), p.command))
	}
	b.WriteString("(host PIDs, read from /proc)\n")
	return b.String(), nil
}

// readProcTable parses every numeric /proc entry into a process table.
func readProcTable() (map[int]*procEntry, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("reading /proc: %w", err)
	}

	procs := make(map[int]*procEntry, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			continue // process exited while scanning
		}

		p := &procEntry{pid: pid}
		var name string
		for _, line := range strings.Split(string(status), "\n") {
			key, val, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			fields := strings.Fields(val)
			if len(fields) == 0 {
				continue
			}
			switch key {
			case "Name":
				name = fields[0]
			case "PPid":
				p.ppid, _ = strconv.Atoi(fields[0])
			case "Uid":
				p.uid = fields[0] // real uid
			}
		}

		// Prefer the full command line; kernel threads and unreadable entries
		// fall back to the bracketed process name.
		if cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil && len(cmdline) > 0 {
			p.command = strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " "))
		}
		if p.command == "" {
			p.command = "[" + name + "]"
		}
		procs[pid] = p
	}
	return procs, nil
}

// userName resolves a numeric uid to a username, falling back to the uid.
func userName(uid string) string {
	if u, err := user.LookupId(uid); err == nil {
		return u.Username
	}
	return uid
}
