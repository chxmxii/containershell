package debug

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// socketEntry is one row parsed from a /proc/<pid>/net/{tcp,udp}[6] table.
type socketEntry struct {
	proto  string // tcp, tcp6, udp, udp6
	local  string // formatted host:port
	remote string // formatted host:port
	state  string
}

// netstatFromProc reads the kernel socket tables for the network namespace of
// the given PID (via /proc/<pid>/net) and renders a netstat-like table. It
// needs no ss/netstat binary in the container or on the host — only read access
// to the target process's /proc entry.
func netstatFromProc(pid uint32) (string, error) {
	sources := []struct{ proto, file string }{
		{"tcp", "tcp"}, {"tcp6", "tcp6"},
		{"udp", "udp"}, {"udp6", "udp6"},
	}

	var entries []socketEntry
	var readAny bool
	var firstErr error

	for _, s := range sources {
		path := fmt.Sprintf("/proc/%d/net/%s", pid, s.file)
		f, err := os.Open(path)
		if err != nil {
			// tcp6/udp6 are absent when IPv6 is disabled; keep going.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		es, perr := parseProcNet(s.proto, f)
		f.Close()
		if perr != nil {
			if firstErr == nil {
				firstErr = perr
			}
			continue
		}
		readAny = true
		entries = append(entries, es...)
	}

	if !readAny {
		return "", firstErr
	}
	return formatSockets(entries), nil
}

// parseProcNet parses one /proc/net socket table (tcp, tcp6, udp, or udp6).
func parseProcNet(proto string, r io.Reader) ([]socketEntry, error) {
	isUDP := strings.HasPrefix(proto, "udp")
	isV6 := strings.HasSuffix(proto, "6")

	var out []socketEntry
	sc := bufio.NewScanner(r)
	header := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if header {
			header = false // first line is the column header
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		local, err := parseHexAddr(fields[1], isV6)
		if err != nil {
			continue
		}
		remote, err := parseHexAddr(fields[2], isV6)
		if err != nil {
			continue
		}

		state := tcpState(fields[3])
		if isUDP {
			state = udpState(fields[3])
		}

		out = append(out, socketEntry{proto: proto, local: local, remote: remote, state: state})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// parseHexAddr converts a "HEXADDR:HEXPORT" token (as found in /proc/net tables)
// into a formatted host:port string.
func parseHexAddr(token string, v6 bool) (string, error) {
	host, portHex, ok := strings.Cut(token, ":")
	if !ok {
		return "", fmt.Errorf("malformed address %q", token)
	}
	port, err := strconv.ParseUint(portHex, 16, 32)
	if err != nil {
		return "", fmt.Errorf("bad port in %q: %w", token, err)
	}
	ip, err := parseHexIP(host, v6)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(ip, strconv.FormatUint(port, 10)), nil
}

// parseHexIP decodes a /proc/net hex IP address. The kernel writes addresses in
// host byte order (little-endian on common platforms): a v4 address is one
// 32-bit little-endian word; a v6 address is four such words.
func parseHexIP(h string, v6 bool) (string, error) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return "", fmt.Errorf("bad hex IP %q: %w", h, err)
	}

	if v6 {
		if len(b) != 16 {
			return "", fmt.Errorf("bad v6 address length %d", len(b))
		}
		ip := make(net.IP, net.IPv6len)
		for i := 0; i < 4; i++ {
			w := b[i*4 : i*4+4]
			ip[i*4+0], ip[i*4+1], ip[i*4+2], ip[i*4+3] = w[3], w[2], w[1], w[0]
		}
		return ip.String(), nil
	}

	if len(b) != 4 {
		return "", fmt.Errorf("bad v4 address length %d", len(b))
	}
	return net.IPv4(b[3], b[2], b[1], b[0]).String(), nil
}

var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
	"04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT",
	"07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK",
	"0A": "LISTEN", "0B": "CLOSING", "0C": "NEW_SYN_RECV",
}

func tcpState(code string) string {
	if s, ok := tcpStates[strings.ToUpper(code)]; ok {
		return s
	}
	return code
}

// udpState maps the limited state a UDP socket reports. 07 (CLOSE) means the
// socket is unconnected; 01 (ESTABLISHED) means it has a fixed peer.
func udpState(code string) string {
	switch strings.ToUpper(code) {
	case "07":
		return "UNCONN"
	case "01":
		return "ESTAB"
	default:
		return "-"
	}
}

// formatSockets renders parsed socket entries as an aligned netstat-like table.
func formatSockets(entries []socketEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-5s %-28s %-28s %s\n", "Proto", "Local Address", "Foreign Address", "State")
	for _, e := range entries {
		fmt.Fprintf(&b, "%-5s %-28s %-28s %s\n", e.proto, e.local, e.remote, e.state)
	}
	return b.String()
}
