// Package vendor maps a MAC address to the manufacturer that registered
// its OUI (the first 24 bits of the address) with the IEEE.
//
// The IEEE-published OUI database is embedded into the binary at compile
// time, so lookups are pure in-memory operations — no network, no API
// keys, no rate limits, works fully offline.
//
// To refresh the bundled database:
//
//	curl -o controller/internal/vendor/oui.txt https://standards-oui.ieee.org/oui/oui.txt
//
// Then rebuild — the new file is picked up automatically by go:embed.
package vendor

import (
	_ "embed"
	"strings"
	"sync"

	"github.com/ding/ding/internal/scanner"
)

//go:embed oui.txt
var ouiRaw string

// Lazy-loaded map: 24-bit OUI prefix → vendor name.
// Loading parses ~6 MB once on first call (~50 ms) and is cached forever.
var (
	ouiOnce sync.Once
	ouiMap  map[uint32]string
)

// Annotate fills in the Vendor field for every result that has a MAC.
// Lookups are pure in-memory map reads, so this is fast (≈110ns each)
// and does not need concurrency. Results without a MAC or without a
// known OUI are left untouched.
func Annotate(results []scanner.Result) {
	for i := range results {
		r := &results[i]
		if r.MAC == nil || r.Vendor != nil {
			continue
		}
		if v := Lookup(*r.MAC); v != "" {
			r.Vendor = &v
		}
	}
}

// Lookup returns the vendor name for a MAC address, or "" if the OUI
// is not registered (or the address is malformed).
//
// Accepts the common formats Ding produces:
//
//	"aa:bb:cc:dd:ee:ff"
//	"AA-BB-CC-DD-EE-FF"
//	"aabbccddeeff"
func Lookup(mac string) string {
	prefix, ok := parseOUI(mac)
	if !ok {
		return ""
	}
	ouiOnce.Do(loadOUI)
	return ouiMap[prefix]
}

// parseOUI extracts the first 24 bits of a MAC address as a uint32.
// Returns false if the input is too short or contains non-hex digits.
func parseOUI(mac string) (uint32, bool) {
	// Strip the common separators so we can parse the hex digits directly.
	cleaned := strings.Map(func(r rune) rune {
		if r == ':' || r == '-' || r == '.' {
			return -1
		}
		return r
	}, mac)

	if len(cleaned) < 6 {
		return 0, false
	}

	var prefix uint32
	for i := 0; i < 6; i++ {
		c := cleaned[i]
		var nibble uint32
		switch {
		case c >= '0' && c <= '9':
			nibble = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			nibble = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			nibble = uint32(c-'A') + 10
		default:
			return 0, false
		}
		prefix = (prefix << 4) | nibble
	}
	return prefix, true
}

// loadOUI parses the embedded oui.txt. The file's relevant lines look like:
//
//	28-6F-B9   (hex)		Nokia Shanghai Bell Co., Ltd.
//
// Everything else (continuation lines for addresses, headers, blanks) is
// ignored.
func loadOUI() {
	ouiMap = make(map[uint32]string, 40000)

	for _, line := range strings.Split(ouiRaw, "\n") {
		// Fast filter — only "(hex)" lines carry the OUI/vendor pair.
		hexIdx := strings.Index(line, "(hex)")
		if hexIdx < 0 {
			continue
		}

		// The OUI prefix is the substring before "(hex)", trimmed.
		ouiStr := strings.TrimSpace(line[:hexIdx])
		prefix, ok := parseOUI(ouiStr)
		if !ok {
			continue
		}

		// Vendor name follows "(hex)", separated by tabs/spaces.
		vendor := strings.TrimSpace(line[hexIdx+len("(hex)"):])
		if vendor == "" {
			continue
		}
		ouiMap[prefix] = vendor
	}
}
