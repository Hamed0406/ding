package enrich

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ding/ding/internal/scanner"
)

// ── unit tests ───────────────────────────────────────────────────────────────

func TestSnmpRequest_structure(t *testing.T) {
	pkt := snmpRequest()
	if len(pkt) != 57 {
		t.Fatalf("expected 57 bytes, got %d", len(pkt))
	}
	// Outer SEQUENCE tag
	if pkt[0] != 0x30 {
		t.Errorf("byte 0: want 0x30, got %02x", pkt[0])
	}
	// Packet layout (0-indexed):
	//  [0-1]   SEQUENCE header
	//  [2-4]   INTEGER 1 (version v2c)
	//  [5]     OCTET STRING tag; [6] length=6; [7-12] "public"
	//  [13-14] GetRequest-PDU header
	//  [15-20] requestID INTEGER TLV
	//  [21-26] errorStatus + errorIndex
	//  [27-28] VarBindList header
	//  [29-42] VarBind 1 (sysDescr OID at [31-40], NULL at [41-42])
	//  [43-56] VarBind 2 (sysName  OID at [45-54], NULL at [55-56])

	// Version = v2c (INTEGER 1)
	if pkt[2] != 0x02 || pkt[3] != 0x01 || pkt[4] != 0x01 {
		t.Errorf("version TLV: want 02 01 01, got %02x %02x %02x", pkt[2], pkt[3], pkt[4])
	}
	// Community OCTET STRING tag at [5], first char 'p' at [7]
	if pkt[5] != 0x04 || pkt[7] != 0x70 {
		t.Errorf("community tag or first byte wrong: tag=%02x first=%02x", pkt[5], pkt[7])
	}
	// GetRequest-PDU tag at [13]
	if pkt[13] != 0xA0 {
		t.Errorf("PDU tag: want 0xA0, got %02x", pkt[13])
	}
	// sysDescr OID: last meaningful byte (before trailing 0x00) is at [39] = 0x01
	if pkt[39] != 0x01 {
		t.Errorf("sysDescr OID component: want 0x01, got %02x", pkt[39])
	}
	// sysName OID: last meaningful byte is at [53] = 0x05
	if pkt[53] != 0x05 {
		t.Errorf("sysName OID component: want 0x05, got %02x", pkt[53])
	}
}

func TestSnmpRequest_uniqueRequestIDs(t *testing.T) {
	ids := make(map[[4]byte]bool)
	for i := 0; i < 100; i++ {
		p := snmpRequest()
		var id [4]byte
		copy(id[:], p[16:20])
		ids[id] = true
	}
	// With 100 random 32-bit IDs the collision probability is negligible;
	// fewer than 50 unique values would indicate a broken RNG.
	if len(ids) < 50 {
		t.Errorf("request IDs not random enough: only %d unique in 100 calls", len(ids))
	}
}

func TestMatchSNMPDescr(t *testing.T) {
	cases := []struct {
		descr string
		want  string
	}{
		{"Cisco IOS Software, Version 15.5(3)M4a", "Router"},
		{"MikroTik RouterOS 6.49.6", "Router"},
		{"Linux diskstation 4.4.59 #42218 SMP", "NAS"},        // Synology
		{"QNAP NAS model TS-231", "NAS"},
		{"HP ETHERNET MULTI-ENVIRONMENT", "Printer"},
		{"HP LaserJet 4200 Series", "Printer"},
		{"Brother NC-7400W, Node Type:BR", "Printer"},
		{"Hikvision SNMP Agent", "IP Camera"},
		{"FortiOS v7.0.9", "Firewall"},
		{"pfSense firewall 2.7.0", "Firewall"},
		{"some unknown device string xyz", ""},                 // no match
		{"", ""},
	}
	for _, tc := range cases {
		got := matchSNMPDescr(tc.descr)
		if got != tc.want {
			t.Errorf("matchSNMPDescr(%q) = %q, want %q", tc.descr, got, tc.want)
		}
	}
}

func TestParseSNMPResponse_valid(t *testing.T) {
	// Build a minimal GetResponse with sysDescr="RouterOS" and sysName="core-sw"
	descr := "RouterOS"
	name := "core-sw"

	pkt := buildTestSNMPResponse(descr, name)
	gotDescr, gotName := parseSNMPResponse(pkt)

	if gotDescr != descr {
		t.Errorf("sysDescr: got %q, want %q", gotDescr, descr)
	}
	if gotName != name {
		t.Errorf("sysName: got %q, want %q", gotName, name)
	}
}

func TestParseSNMPResponse_truncated(t *testing.T) {
	// Should not panic on short / malformed input
	cases := [][]byte{
		nil,
		{},
		{0x30, 0x05, 0x00},
		{0xFF, 0xFF, 0xFF},
	}
	for _, tc := range cases {
		descr, name := parseSNMPResponse(tc)
		if descr != "" || name != "" {
			t.Errorf("expected empty results for malformed input %v, got %q %q", tc, descr, name)
		}
	}
}

func TestSNMPDeviceType_offline(t *testing.T) {
	alive := true
	results := []scanner.Result{
		{IP: "192.0.2.1", Alive: alive}, // TEST-NET — guaranteed unreachable
	}
	start := time.Now()
	SNMPDeviceType(results, 1, 100*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("took too long: %v", elapsed)
	}
	if results[0].DeviceType != nil {
		t.Errorf("expected nil DeviceType, got %q", *results[0].DeviceType)
	}
}

func TestSNMPDeviceType_skipDead(t *testing.T) {
	results := []scanner.Result{{IP: "192.0.2.1", Alive: false}}
	SNMPDeviceType(results, 1, 100*time.Millisecond)
	if results[0].DeviceType != nil {
		t.Errorf("dead host should not be probed")
	}
}

func TestSNMPDeviceType_skipFullyEnriched(t *testing.T) {
	dt := "Router"
	hn := "core-sw"
	results := []scanner.Result{{IP: "192.0.2.1", Alive: true, DeviceType: &dt, Hostname: &hn}}
	SNMPDeviceType(results, 1, 100*time.Millisecond)
	if *results[0].DeviceType != dt || *results[0].Hostname != hn {
		t.Errorf("already-enriched device should not be modified")
	}
}

// ── live loopback test ────────────────────────────────────────────────────────

func TestSNMPDeviceType_liveServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live server test in short mode")
	}

	resp := buildTestSNMPResponse("MikroTik RouterOS 6.49.6", "edge-router")

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

	descr, name, err := snmpGetPort("127.0.0.1", port, 2*time.Second)
	if err != nil {
		t.Fatalf("snmpGet: %v", err)
	}
	if descr != "MikroTik RouterOS 6.49.6" {
		t.Errorf("sysDescr: got %q", descr)
	}
	if name != "edge-router" {
		t.Errorf("sysName: got %q", name)
	}

	// Full pipeline: SNMPDeviceType should pick Router via matchSNMPDescr
	alive := true
	results := []scanner.Result{{IP: "127.0.0.1", Alive: alive}}
	snmpDeviceTypePort(results, 1, 2*time.Second, port)
	if results[0].DeviceType == nil || *results[0].DeviceType != "Router" {
		t.Errorf("expected DeviceType=Router, got %v", results[0].DeviceType)
	}
	if results[0].Hostname == nil || *results[0].Hostname != "edge-router" {
		t.Errorf("expected Hostname=edge-router, got %v", results[0].Hostname)
	}
}

// ── test helpers ─────────────────────────────────────────────────────────────

// buildTestSNMPResponse constructs a minimal valid SNMPv2c GetResponse packet.
func buildTestSNMPResponse(sysDescr, sysName string) []byte {
	octetString := func(s string) []byte {
		b := []byte(s)
		return append([]byte{0x04, byte(len(b))}, b...)
	}
	oid := func(last byte) []byte {
		// OID 1.3.6.1.2.1.1.X.0 — encoded bytes are identical except position 7
		return []byte{0x06, 0x08, 0x2B, 0x06, 0x01, 0x02, 0x01, 0x01, last, 0x00}
	}
	seq := func(content []byte) []byte {
		return append([]byte{0x30, byte(len(content))}, content...)
	}

	vb1 := seq(append(oid(0x01), octetString(sysDescr)...))
	vb2 := seq(append(oid(0x05), octetString(sysName)...))
	vbl := seq(append(vb1, vb2...))

	// requestID INTEGER 4-bytes, errorStatus/errorIndex = 0
	reqID := []byte{0x02, 0x04, 0x00, 0x00, 0x00, 0x01}
	errSt := []byte{0x02, 0x01, 0x00}
	errIdx := []byte{0x02, 0x01, 0x00}
	pduContent := concat(reqID, errSt, errIdx, vbl)

	pdu := append([]byte{0xA2, byte(len(pduContent))}, pduContent...)
	version := []byte{0x02, 0x01, 0x01}
	community := append([]byte{0x04, 0x06}, []byte("public")...)
	msgContent := concat(version, community, pdu)
	return append([]byte{0x30, byte(len(msgContent))}, msgContent...)
}

func concat(slices ...[]byte) []byte {
	var out []byte
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// snmpGetPort is like snmpGet but uses an explicit port — test helper only.
func snmpGetPort(ip string, port int, timeout time.Duration) (descr, name string, err error) {
	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", ip, port), timeout)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout)) //nolint:errcheck
	if _, err = conn.Write(snmpRequest()); err != nil {
		return "", "", err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", "", err
	}
	descr, name = parseSNMPResponse(buf[:n])
	return descr, name, nil
}

// snmpDeviceTypePort is like SNMPDeviceType but targets an explicit port.
func snmpDeviceTypePort(results []scanner.Result, workers int, perHost time.Duration, port int) {
	for i := range results {
		r := &results[i]
		if !r.Alive || (r.DeviceType != nil && r.Hostname != nil) {
			continue
		}
		descr, name, err := snmpGetPort(r.IP, port, perHost)
		if err != nil {
			continue
		}
		if r.DeviceType == nil && descr != "" {
			if dt := matchSNMPDescr(descr); dt != "" {
				r.DeviceType = &dt
			}
		}
		if r.Hostname == nil && name != "" {
			r.Hostname = &name
		}
	}
}
