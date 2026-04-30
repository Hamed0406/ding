// SNMP sysDescr / sysName enricher
//
// Sends an SNMPv2c GetRequest (community "public") for two OIDs:
//   1.3.6.1.2.1.1.1.0  sysDescr — human-readable device description
//   1.3.6.1.2.1.1.5.0  sysName  — configured host/device name
//
// sysDescr is matched against a fingerprint table to classify devices that
// vendor/port/HTTP probes couldn't identify (routers, switches, NAS, printers).
// sysName fills in Hostname when DNS and NetBIOS both came up empty.
//
// No external SNMP library — the two-OID request packet is 57 bytes of
// hand-crafted BER/ASN.1, well within "simple enough to inline" territory.

package enrich

import (
	"encoding/binary"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ding/ding/internal/scanner"
)

// SNMPDeviceType sends an SNMPv2c GetRequest to UDP 161 on each alive device
// that still needs a DeviceType or Hostname, and fills them in from sysDescr
// and sysName respectively.
//
//   - workers: max simultaneous UDP probes (e.g. 16)
//   - perHost: read deadline per probe (e.g. 800ms)
func SNMPDeviceType(results []scanner.Result, workers int, perHost time.Duration) {
	if workers <= 0 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range results {
		r := &results[i]
		if !r.Alive {
			continue
		}
		// Skip devices that are already fully enriched.
		if r.DeviceType != nil && r.Hostname != nil {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(r *scanner.Result) {
			defer wg.Done()
			defer func() { <-sem }()

			descr, name, err := snmpGet(r.IP, perHost)
			if err != nil {
				return
			}

			mu.Lock()
			defer mu.Unlock()

			if r.DeviceType == nil && descr != "" {
				if dt := matchSNMPDescr(descr); dt != "" {
					r.DeviceType = &dt
				}
			}
			if r.Hostname == nil && name != "" {
				r.Hostname = &name
			}
		}(r)
	}
	wg.Wait()
}

// snmpGet sends an SNMPv2c GetRequest for sysDescr + sysName and returns
// (sysDescr, sysName, error). Empty strings mean no response or no value.
func snmpGet(ip string, timeout time.Duration) (descr, name string, err error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(ip, "161"), timeout)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout)) //nolint:errcheck

	pkt := snmpRequest()
	if _, err = conn.Write(pkt); err != nil {
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

// snmpRequest builds a 57-byte SNMPv2c GetRequest for sysDescr + sysName.
//
// Packet layout (BER-encoded ASN.1):
//
//	SEQUENCE {
//	  INTEGER 1                    -- version: v2c
//	  OCTET STRING "public"        -- community
//	  GetRequest-PDU (0xA0) {
//	    INTEGER requestID           -- 4-byte random
//	    INTEGER 0                   -- errorStatus
//	    INTEGER 0                   -- errorIndex
//	    SEQUENCE {                  -- VarBindList
//	      SEQUENCE { OID sysDescr; NULL }
//	      SEQUENCE { OID sysName;  NULL }
//	    }
//	  }
//	}
func snmpRequest() []byte {
	// Fixed-layout packet — only requestID (bytes 16–19) changes per call.
	pkt := []byte{
		0x30, 0x37, // SEQUENCE, 55 bytes
		0x02, 0x01, 0x01, // INTEGER 1 (v2c)
		0x04, 0x06, 0x70, 0x75, 0x62, 0x6C, 0x69, 0x63, // OCTET STRING "public"
		0xA0, 0x2A, // GetRequest-PDU, 42 bytes
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, // INTEGER requestID (filled below)
		0x02, 0x01, 0x00, // INTEGER 0 (errorStatus: noError)
		0x02, 0x01, 0x00, // INTEGER 0 (errorIndex)
		0x30, 0x1C, // SEQUENCE VarBindList, 28 bytes
		// VarBind 1 — sysDescr (1.3.6.1.2.1.1.1.0)
		0x30, 0x0C,
		0x06, 0x08, 0x2B, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00,
		0x05, 0x00,
		// VarBind 2 — sysName (1.3.6.1.2.1.1.5.0)
		0x30, 0x0C,
		0x06, 0x08, 0x2B, 0x06, 0x01, 0x02, 0x01, 0x01, 0x05, 0x00,
		0x05, 0x00,
	}
	binary.BigEndian.PutUint32(pkt[16:], rand.N[uint32](0xFFFFFFFF))
	return pkt
}

// parseSNMPResponse walks the BER-encoded GetResponse and returns
// (sysDescr, sysName). Either may be empty if absent or on error.
//
// Expected structure:
//
//	SEQUENCE                        outer message
//	  INTEGER                       version
//	  OCTET STRING                  community
//	  GetResponse-PDU (0xA2)
//	    INTEGER                     requestID
//	    INTEGER                     errorStatus
//	    INTEGER                     errorIndex
//	    SEQUENCE                    VarBindList
//	      SEQUENCE                  VarBind[0] — sysDescr
//	        OID                     1.3.6.1.2.1.1.1.0
//	        OCTET STRING            value
//	      SEQUENCE                  VarBind[1] — sysName
//	        OID                     1.3.6.1.2.1.1.5.0
//	        OCTET STRING            value
func parseSNMPResponse(buf []byte) (descr, name string) {
	c := &berCursor{buf: buf}

	// Outer SEQUENCE
	tag, content, ok := c.readTLV()
	if !ok || tag != 0x30 {
		return
	}
	c = &berCursor{buf: content}

	c.skip() // version INTEGER
	c.skip() // community OCTET STRING

	// GetResponse PDU (0xA2)
	tag, pdu, ok := c.readTLV()
	if !ok || tag != 0xA2 {
		return
	}
	c = &berCursor{buf: pdu}

	c.skip() // requestID
	c.skip() // errorStatus
	c.skip() // errorIndex

	// VarBindList SEQUENCE
	tag, vbl, ok := c.readTLV()
	if !ok || tag != 0x30 {
		return
	}
	c = &berCursor{buf: vbl}

	// First VarBind → sysDescr
	if tag, vb, ok := c.readTLV(); ok && tag == 0x30 {
		vc := &berCursor{buf: vb}
		vc.skip() // skip OID
		if vtag, val, ok := vc.readTLV(); ok && vtag == 0x04 {
			descr = strings.TrimSpace(string(val))
		}
	}
	// Second VarBind → sysName
	if tag, vb, ok := c.readTLV(); ok && tag == 0x30 {
		vc := &berCursor{buf: vb}
		vc.skip() // skip OID
		if vtag, val, ok := vc.readTLV(); ok && vtag == 0x04 {
			name = strings.TrimSpace(string(val))
		}
	}
	return
}

// berCursor is a minimal sequential BER reader.
type berCursor struct {
	buf []byte
	pos int
}

// readTLV reads one TLV from the cursor position and advances past it.
// Returns (tag, value, true) on success, (0, nil, false) on any error.
func (c *berCursor) readTLV() (tag byte, value []byte, ok bool) {
	if c.pos >= len(c.buf) {
		return 0, nil, false
	}
	tag = c.buf[c.pos]
	c.pos++

	if c.pos >= len(c.buf) {
		return 0, nil, false
	}
	b := c.buf[c.pos]
	c.pos++

	var length int
	if b < 0x80 {
		length = int(b)
	} else {
		n := int(b & 0x7F)
		if n == 0 || n > 4 || c.pos+n > len(c.buf) {
			return 0, nil, false
		}
		for i := 0; i < n; i++ {
			length = (length << 8) | int(c.buf[c.pos])
			c.pos++
		}
	}

	if c.pos+length > len(c.buf) {
		return 0, nil, false
	}
	value = c.buf[c.pos : c.pos+length]
	c.pos += length
	return tag, value, true
}

// skip advances past one TLV, returning false if the buffer is exhausted.
func (c *berCursor) skip() bool {
	_, _, ok := c.readTLV()
	return ok
}

// matchSNMPDescr returns a device category by matching sysDescr substrings.
// Rules are ordered most-specific first; the first match wins.
func matchSNMPDescr(descr string) string {
	d := strings.ToLower(descr)
	for _, fp := range snmpFingerprints {
		if strings.Contains(d, fp.sub) {
			return fp.dtype
		}
	}
	return ""
}

var snmpFingerprints = []struct {
	sub   string
	dtype string
}{
	// ── Networking gear ───────────────────────────────────────────────────
	{"cisco ios", "Router"},
	{"cisco nx-os", "Router"},
	{"cisco adaptive security", "Firewall"}, // ASA
	{"juniper networks", "Router"},
	{"junos", "Router"},
	{"routeros", "Router"},   // MikroTik
	{"mikrotik", "Router"},
	{"edgeos", "Router"},     // Ubiquiti EdgeRouter
	{"airos", "Router"},      // Ubiquiti AirOS
	{"unifi", "Router"},      // Ubiquiti UniFi
	{"dd-wrt", "Router"},
	{"openwrt", "Router"},
	{"tomato", "Router"},
	{"ubiquiti", "Router"},
	{"zyxel", "Router"},
	{"netgear", "Router"},
	{"draytek", "Router"},
	// ── Firewalls ────────────────────────────────────────────────────────
	{"fortios", "Firewall"},
	{"fortigate", "Firewall"},
	{"palo alto", "Firewall"},
	{"panos", "Firewall"},
	{"pfsense", "Firewall"},
	{"opnsense", "Firewall"},
	{"watchguard", "Firewall"},
	{"sonicwall", "Firewall"},
	// ── NAS / Storage ────────────────────────────────────────────────────
	{"synology", "NAS"},
	{"diskstation", "NAS"},  // Synology sysDescr often contains "diskstation"
	{"qnap", "NAS"},
	{"freenas", "NAS"},
	{"truenas", "NAS"},
	{"xigmanas", "NAS"},
	{"openmediavault", "NAS"},
	// ── Printers ─────────────────────────────────────────────────────────
	{"hp ethernet multi-environment", "Printer"}, // HP JetDirect — most specific
	{"laserjet", "Printer"},
	{"officejet", "Printer"},
	{"deskjet", "Printer"},
	{"brother nc-", "Printer"},
	{"brother", "Printer"},
	{"canon ir", "Printer"},
	{"canon i-sensys", "Printer"},
	{"ricoh", "Printer"},
	{"aficio", "Printer"},       // Ricoh Aficio series
	{"xerox", "Printer"},
	{"epson", "Printer"},
	{"lexmark", "Printer"},
	{"kyocera", "Printer"},
	{"konica minolta", "Printer"},
	{"sharp mx", "Printer"},     // Sharp MFPs
	{"oki data", "Printer"},
	// ── IP Cameras ───────────────────────────────────────────────────────
	{"hikvision", "IP Camera"},
	{"dahua", "IP Camera"},
	{"axis", "IP Camera"},
	{"hanwha", "IP Camera"},
	{"vivotek", "IP Camera"},
	{"reolink", "IP Camera"},
}
