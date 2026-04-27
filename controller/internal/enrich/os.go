// ============================================================
// controller/internal/enrich/os.go — OS fingerprinting
//
// Three signal sources, applied in priority order:
//
// 1. Hostname patterns (most reliable when present)
//      "android-*"          → Android
//      contains "iphone"    → iOS
//      contains "ipad"      → iOS
//
// 2. Device type inferred by classify (vendor / port rules)
//      "Samsung Device", "Phone" with TTL 64 → Android
//      "Apple Device" with TTL 64 → iOS (iPhones / iPads are the common case)
//
// 3. TTL rounded to nearest standard initial value
//      Received ≤ 64  → initial 64  → Linux/macOS/iOS/Android family
//      Received ≤ 128 → initial 128 → Windows
//      Received > 128 → initial 255 → network equipment — skipped
//
//    Port hints break ties within the TTL-64 family:
//      548, 5009  → macOS     62078 → iOS     5555 → Android
//      (none)     → Linux
// ============================================================

package enrich

import (
	"strings"

	"github.com/ding/ding/internal/scanner"
)

// AnnotateOS sets OS on each result that has enough signal. Skips results
// that already have an OS set by a higher-priority source.
func AnnotateOS(results []scanner.Result) {
	for i := range results {
		if results[i].OS != nil {
			continue
		}
		results[i].OS = inferOS(&results[i])
	}
}

func inferOS(r *scanner.Result) *string {
	// 1. Hostname patterns — most reliable signal when present.
	if r.Hostname != nil {
		h := strings.ToLower(*r.Hostname)
		switch {
		case strings.HasPrefix(h, "android-"):
			return ptr("Android")
		case strings.Contains(h, "iphone"):
			return ptr("iOS")
		case strings.Contains(h, "ipad"):
			return ptr("iOS")
		}
	}

	// 2. Vendor-based device type — classify already determined the category,
	//    so we can infer the OS without needing TTL.
	if r.DeviceType != nil {
		switch strings.ToLower(*r.DeviceType) {
		case "samsung device":
			// Samsung's network devices are almost exclusively Android phones/tablets.
			return ptr("Android")
		case "gaming console":
			// Nintendo → proprietary; Sony PlayStation → FreeBSD-derived; Xbox → custom.
			// We can't distinguish here, so skip.
		}
	}

	// 3. TTL-based inference — requires a ping reply.
	if r.TTL == nil {
		return nil
	}

	switch roundTTL(*r.TTL) {
	case 64:
		// Could be Linux, macOS, iOS, or Android — use port hints to narrow it down.
		os := refineUnix(r)
		// If classify already said it's a phone/tablet vendor, prefer "Android" over
		// the generic "Linux" fallback (Android runs the Linux kernel).
		if os == "Linux" && r.DeviceType != nil {
			switch strings.ToLower(*r.DeviceType) {
			case "phone", "apple device":
				// Apple Device with TTL 64: iPhones and iPads are far more common
				// on home networks than Macs, so lean toward iOS.
				if strings.ToLower(*r.DeviceType) == "apple device" {
					return ptr("iOS")
				}
				return ptr("Android")
			}
		}
		return ptr(os)
	case 128:
		return ptr("Windows")
	default:
		// 255 → network equipment; vendor/classify own that label.
		return nil
	}
}

// refineUnix distinguishes macOS / iOS / Android from the generic Linux bucket
// using open port hints. Falls back to "Linux" when no specific port is visible.
func refineUnix(r *scanner.Result) string {
	for _, p := range r.OpenPorts {
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
