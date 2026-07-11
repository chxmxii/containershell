package debug

import (
	"strings"
	"testing"
)

// A representative /proc/<pid>/net/tcp table: one LISTEN socket on 127.0.0.1:80
// and one ESTABLISHED connection 127.0.0.1:8080 -> 192.168.0.2:53748.
const sampleTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 0200A8C0:D1F4 01 00000000:00000000 00:00000000 00000000  1000        0 65432 1 0000000000000000 20 4 30 10 -1
`

func TestParseProcNetTCP(t *testing.T) {
	entries, err := parseProcNet("tcp", strings.NewReader(sampleTCP))
	if err != nil {
		t.Fatalf("parseProcNet: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	if entries[0].local != "127.0.0.1:80" || entries[0].remote != "0.0.0.0:0" || entries[0].state != "LISTEN" {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[1].local != "127.0.0.1:8080" || entries[1].remote != "192.168.0.2:53748" || entries[1].state != "ESTABLISHED" {
		t.Errorf("entry1 = %+v", entries[1])
	}
}

func TestParseHexIPv4(t *testing.T) {
	tests := map[string]string{
		"0100007F": "127.0.0.1",
		"00000000": "0.0.0.0",
		"0200A8C0": "192.168.0.2",
	}
	for in, want := range tests {
		got, err := parseHexIP(in, false)
		if err != nil {
			t.Fatalf("parseHexIP(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("parseHexIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseHexIPv6(t *testing.T) {
	// ::1 (loopback) as the kernel stores it: four little-endian 32-bit words.
	got, err := parseHexIP("00000000000000000000000001000000", true)
	if err != nil {
		t.Fatalf("parseHexIP v6: %v", err)
	}
	if got != "::1" {
		t.Errorf("v6 loopback = %q, want ::1", got)
	}
}

func TestParseHexAddr(t *testing.T) {
	got, err := parseHexAddr("0100007F:0050", false)
	if err != nil {
		t.Fatalf("parseHexAddr v4: %v", err)
	}
	if got != "127.0.0.1:80" {
		t.Errorf("v4 addr = %q, want 127.0.0.1:80", got)
	}

	got, err = parseHexAddr("00000000000000000000000001000000:1F90", true)
	if err != nil {
		t.Fatalf("parseHexAddr v6: %v", err)
	}
	if got != "[::1]:8080" {
		t.Errorf("v6 addr = %q, want [::1]:8080", got)
	}
}

func TestParseProcNetSkipsGarbage(t *testing.T) {
	data := "header line to skip\ngarbage\n   0: 0100007F:0050 00000000:0000 0A rest\n"
	entries, err := parseProcNet("tcp", strings.NewReader(data))
	if err != nil {
		t.Fatalf("parseProcNet: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (garbage line skipped)", len(entries))
	}
}

func TestUDPState(t *testing.T) {
	if udpState("07") != "UNCONN" {
		t.Errorf("udpState(07) = %q, want UNCONN", udpState("07"))
	}
	if udpState("01") != "ESTAB" {
		t.Errorf("udpState(01) = %q, want ESTAB", udpState("01"))
	}
}

func TestFormatSockets(t *testing.T) {
	out := formatSockets([]socketEntry{
		{proto: "tcp", local: "127.0.0.1:80", remote: "0.0.0.0:0", state: "LISTEN"},
	})
	if !strings.Contains(out, "Proto") || !strings.Contains(out, "Local Address") {
		t.Errorf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "tcp") || !strings.Contains(out, "127.0.0.1:80") || !strings.Contains(out, "LISTEN") {
		t.Errorf("missing row data:\n%s", out)
	}
}
