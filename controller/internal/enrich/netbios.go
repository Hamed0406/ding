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

// NetBIOSNames fills in the Hostname field for alive devices that still have
// no name after reverse-DNS lookup, by sending a NetBIOS Node Status Request
// (NBSTAT) to UDP port 137.
//
// This catches Windows PCs, NAS boxes, and older printers that don't publish
// PTR records but do respond to NetBIOS — filling in the most common gap left
// by the DNS enricher.
//
//   - parallelism: max simultaneous UDP probes (e.g. 16)
//   - perHost:     read deadline per probe (e.g. 1s)
func NetBIOSNames(results []scanner.Result, parallelism int, perHost time.Duration) {
	if parallelism <= 0 {
		parallelism = 1
	}
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for i := range results {
		r := &results[i]
		if r.Hostname != nil || !r.Alive {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(r *scanner.Result) {
			defer wg.Done()
			defer func() { <-sem }()

			name, err := nbstat(r.IP, perHost)
			if err != nil || name == "" {
				return
			}
			r.Hostname = &name
		}(r)
	}
	wg.Wait()
}

// nbstat sends a NetBIOS Node Status Request to ip:137 and returns the
// workstation name (first type-0x00 unique name in the response table).
func nbstat(ip string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(ip, "137"), timeout)
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

// nbstatRequest builds the 50-byte NetBIOS Node Status Request packet.
//
// Format (RFC 1002 §4.2.17):
//   - 2 B  transaction ID (random)
//   - 2 B  flags: 0x0000 (query, non-recursive, NBSTAT op)
//   - 2 B  QDCOUNT = 1
//   - 6 B  ANCOUNT / NSCOUNT / ARCOUNT = 0
//   - 34 B question name: length-prefixed NetBIOS-encoded "*" + padding
//   - 2 B  question type  = 0x0021 (NBSTAT)
//   - 2 B  question class = 0x0001 (IN)
func nbstatRequest() []byte {
	pkt := make([]byte, 50)
	binary.BigEndian.PutUint16(pkt[0:], rand.N[uint16](0xFFFF)) // txid
	binary.BigEndian.PutUint16(pkt[2:], 0x0000)                 // flags
	binary.BigEndian.PutUint16(pkt[4:], 0x0001)                 // qdcount
	// ancount, nscount, arcount all 0 (bytes 6-11 already zero)

	// Question name: NetBIOS-encoded "*" (wildcard) padded to 16 bytes.
	// Each source byte B encodes as two letters: (B>>4)+'A', (B&0xF)+'A'.
	// '*' = 0x2A → 'C','K'; space (0x20, pad) → 'C','A'.
	pkt[12] = 32 // label length (32 encoded chars = 16 NetBIOS bytes)
	pkt[13] = 'C'
	pkt[14] = 'K'
	for i := 15; i < 45; i += 2 { // 15 padding bytes → 30 chars
		pkt[i] = 'C'
		pkt[i+1] = 'A'
	}
	// pkt[45] = 0x00 — root label (already zero)
	binary.BigEndian.PutUint16(pkt[46:], 0x0021) // type NBSTAT
	binary.BigEndian.PutUint16(pkt[48:], 0x0001) // class IN
	return pkt
}

// parseNBSTAT extracts the first workstation (type 0x00, unique) name from a
// NetBIOS Node Status Response.
//
// Response layout after the 12-byte DNS-style header:
//
//	question section: name (variable) + QTYPE(2) + QCLASS(2)
//	answer RR:        name (variable, usually compressed ptr) + TYPE(2) + CLASS(2) + TTL(4) + RDLENGTH(2)
//	rdata:            num_names(1) + num_names×18 bytes
//
// Each 18-byte name table entry: 15-char padded name + 1-byte suffix + 2-byte flags.
func parseNBSTAT(pkt []byte) (string, error) {
	if len(pkt) < 12 {
		return "", nil
	}

	offset := 12

	// Skip echoed question section: name + QTYPE(2) + QCLASS(2)
	offset, _ = skipName(pkt, offset)
	offset += 4

	// Skip answer RR name (usually a 2-byte compressed pointer 0xC0 0x0C)
	offset, _ = skipName(pkt, offset)

	// Skip TYPE(2) + CLASS(2) + TTL(4) + RDLENGTH(2) = 10 bytes
	offset += 10

	if offset >= len(pkt) {
		return "", nil
	}

	numNames := int(pkt[offset])
	offset++

	for i := 0; i < numNames && offset+17 < len(pkt); i++ {
		name := strings.TrimRight(string(pkt[offset:offset+15]), " ")
		suffix := pkt[offset+15]
		flags := binary.BigEndian.Uint16(pkt[offset+16 : offset+18])
		offset += 18

		isGroup := flags&0x8000 != 0
		// type 0x00, unique name = workstation / computer name
		if suffix == 0x00 && !isGroup && name != "" {
			return name, nil
		}
	}
	return "", nil
}

// skipName advances past a DNS-style compressed or label-sequence name
// starting at pkt[offset] and returns the new offset after the name.
func skipName(pkt []byte, offset int) (int, error) {
	for offset < len(pkt) {
		length := int(pkt[offset])
		if length == 0 {
			return offset + 1, nil
		}
		// DNS pointer compression: top two bits set
		if length&0xC0 == 0xC0 {
			return offset + 2, nil
		}
		offset += 1 + length
	}
	return offset, nil
}
