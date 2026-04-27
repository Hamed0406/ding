// ============================================================
// controller/internal/enrich/os.go — OS fingerprinting from TTL + ports
//
// Every OS starts packets with a fixed initial TTL. By the time a packet
// arrives at the scanner, each router hop has decremented it by one. We
// round the received TTL up to the nearest standard initial value to
// recover the OS family:
//
//   Received ≤ 64  → initial 64  → Linux / macOS / iOS / Android
//   Received ≤ 128 → initial 128 → Windows
//   Received > 128 → initial 255 → network equipment (Cisco IOS, etc.)
//                                  — skipped; vendor/classify already handles it
//
// Open ports refine the Linux/Unix family into a more specific OS:
//   548  (AFP)             → macOS
//   5009 (Airport Admin)   → macOS
//   62078 (iPhone Sync)    → iOS
//   5555 (ADB)             → Android
//   (none of the above)    → Linux
// ============================================================

package enrich

import "github.com/ding/ding/internal/scanner"

// AnnotateOS sets OS on each result that has a TTL but no OS already set.
func AnnotateOS(results []scanner.Result) {
	for i := range results {
		if results[i].OS != nil {
			continue
		}
		results[i].OS = inferOS(&results[i])
	}
}

func inferOS(r *scanner.Result) *string {
	if r.TTL == nil {
		return nil
	}

	switch roundTTL(*r.TTL) {
	case 64:
		return ptr(refineUnix(r.OpenPorts))
	case 128:
		return ptr("Windows")
	default:
		// 255 = network equipment; let vendor/classify own the label
		return nil
	}
}

// refineUnix distinguishes macOS / iOS / Android from the generic Linux bucket
// using port hints. Falls back to "Linux" when no specific port is open.
func refineUnix(ports []uint16) string {
	for _, p := range ports {
		switch p {
		case 548, 5009:
			return "macOS"
		case 62078:
			return "iOS"
		case 5555:
			return "Android"
		}
	}
	return "Linux"
}

// roundTTL returns the nearest standard initial TTL (64, 128, or 255).
func roundTTL(ttl uint8) uint8 {
	switch {
	case ttl <= 64:
		return 64
	case ttl <= 128:
		return 128
	default:
		return 255
	}
}

func ptr(s string) *string { return &s }
