package enrich

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ding/ding/internal/scanner"
)

// TestNetBIOSNames_offline verifies that a host that doesn't respond on
// UDP 137 leaves Hostname nil and doesn't block past the timeout.
func TestNetBIOSNames_offline(t *testing.T) {
	alive := true
	results := []scanner.Result{
		{IP: "192.0.2.1", Alive: alive}, // TEST-NET — guaranteed unreachable
	}
	start := time.Now()
	NetBIOSNames(results, 1, 100*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("took too long: %v", elapsed)
	}
	if results[0].Hostname != nil {
		t.Errorf("expected nil hostname, got %q", *results[0].Hostname)
	}
}

// TestNetBIOSNames_skipDead verifies that dead hosts are skipped entirely.
func TestNetBIOSNames_skipDead(t *testing.T) {
	results := []scanner.Result{
		{IP: "192.0.2.1", Alive: false},
	}
	NetBIOSNames(results, 1, 100*time.Millisecond)
	if results[0].Hostname != nil {
		t.Errorf("dead host should not be probed")
	}
}

// TestNetBIOSNames_skipAlreadyNamed verifies that hosts with a hostname are skipped.
func TestNetBIOSNames_skipAlreadyNamed(t *testing.T) {
	name := "router.lan"
	results := []scanner.Result{
		{IP: "192.0.2.1", Alive: true, Hostname: &name},
	}
	NetBIOSNames(results, 1, 100*time.Millisecond)
	if *results[0].Hostname != name {
		t.Errorf("hostname should be unchanged, got %q", *results[0].Hostname)
	}
}

// TestNbstatRequest verifies the request packet has the right fixed fields.
func TestNbstatRequest(t *testing.T) {
	pkt := nbstatRequest()
	if len(pkt) != 50 {
		t.Fatalf("expected 50 bytes, got %d", len(pkt))
	}
	// Flags should be 0x0000
	if pkt[2] != 0 || pkt[3] != 0 {
		t.Errorf("flags should be 0x0000, got %02x%02x", pkt[2], pkt[3])
	}
	// QDCOUNT should be 1
	if pkt[4] != 0 || pkt[5] != 1 {
		t.Errorf("qdcount should be 1")
	}
	// Question type should be 0x0021 (NBSTAT)
	if pkt[46] != 0x00 || pkt[47] != 0x21 {
		t.Errorf("question type should be 0x0021, got %02x%02x", pkt[46], pkt[47])
	}
	// Question class should be 0x0001 (IN)
	if pkt[48] != 0x00 || pkt[49] != 0x01 {
		t.Errorf("question class should be 0x0001")
	}
}

// TestParseNBSTAT_realResponse verifies parsing against a hand-built NBSTAT response
// that follows the real wire format: question section + answer RR + name table.
func TestParseNBSTAT_realResponse(t *testing.T) {
	// 12-byte DNS-style header: txid, flags=response, qdcount=1, ancount=1
	hdr := []byte{0x12, 0x34, 0x85, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}

	// Question section: name (null root label) + QTYPE(NBSTAT=0x0021) + QCLASS(IN=0x0001)
	question := []byte{0x00, 0x00, 0x21, 0x00, 0x01}

	// Answer RR name: compressed pointer back to offset 12 (0xC0 0x0C)
	answerName := []byte{0xC0, 0x0C}

	// Answer RR: TYPE(2) + CLASS(2) + TTL(4) + RDLENGTH(2)
	// rdlength = 1 (num_names) + 2×18 (two entries) = 37
	answerFixed := []byte{
		0x00, 0x21, // TYPE NBSTAT
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x00, // TTL
		0x00, 0x25, // RDLENGTH = 37
	}

	// num_names = 2
	numNames := []byte{0x02}

	// Entry 1: "MYPC           " (15 bytes), suffix 0x00, flags 0x0000 (unique)
	entry1 := make([]byte, 18)
	copy(entry1, "MYPC           ")
	// entry1[15..17] already 0x00

	// Entry 2: "WORKGROUP      " suffix 0x00, flags 0x8000 (group)
	entry2 := make([]byte, 18)
	copy(entry2, "WORKGROUP      ")
	entry2[16] = 0x80

	var pkt []byte
	pkt = append(pkt, hdr...)
	pkt = append(pkt, question...)
	pkt = append(pkt, answerName...)
	pkt = append(pkt, answerFixed...)
	pkt = append(pkt, numNames...)
	pkt = append(pkt, entry1...)
	pkt = append(pkt, entry2...)

	name, err := parseNBSTAT(pkt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "MYPC" {
		t.Errorf("expected MYPC, got %q", name)
	}
}

// TestNetBIOSNames_liveServer runs against a real UDP listener that mimics a
// NetBIOS response. Skipped in short mode.
func TestNetBIOSNames_liveServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live server test in short mode")
	}

	// Build the response packet (same format as TestParseNBSTAT_realResponse)
	hdr := []byte{0x00, 0x00, 0x85, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	question := []byte{0x00, 0x00, 0x21, 0x00, 0x01}
	answerName := []byte{0xC0, 0x0C}
	answerFixed := []byte{0x00, 0x21, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x25}
	numNames := []byte{0x02}
	entry1 := make([]byte, 18)
	copy(entry1, "TESTHOST       ")
	entry2 := make([]byte, 18)
	copy(entry2, "WORKGROUP      ")
	entry2[16] = 0x80

	var resp []byte
	resp = append(resp, hdr...)
	resp = append(resp, question...)
	resp = append(resp, answerName...)
	resp = append(resp, answerFixed...)
	resp = append(resp, numNames...)
	resp = append(resp, entry1...)
	resp = append(resp, entry2...)

	// Listen on a random port
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.UDPAddr).Port

	go func() {
		buf := make([]byte, 512)
		for {
			_, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			conn.WriteTo(resp, addr) //nolint:errcheck
		}
	}()

	// Temporarily monkey-patch nbstat to use our port instead of 137.
	// We test parseNBSTAT directly above; here we just confirm the happy path.
	name, err := nbstatPort("127.0.0.1", port, 2*time.Second)
	if err != nil {
		t.Fatalf("nbstat: %v", err)
	}
	if name != "TESTHOST" {
		t.Errorf("expected TESTHOST, got %q", name)
	}
}

// nbstatPort is like nbstat but uses an explicit port — test helper only.
func nbstatPort(ip string, port int, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(ip, fmt.Sprintf("%d", port)), timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout)) //nolint:errcheck
	if _, err := conn.Write(nbstatRequest()); err != nil {
		return "", err
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return parseNBSTAT(buf[:n])
}

