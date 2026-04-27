package enrich

import (
	"github.com/ding/ding/internal/scanner"
)

// serviceCategory maps mDNS service type strings to device category labels.
// mDNS data is more authoritative than port/vendor guesses, so these
// values override whatever classify.Annotate may have set.
var serviceCategory = map[string]string{
	"_googlecast._tcp":      "Streaming Dongle",
	"_airplay._tcp":         "Apple TV",
	"_raop._tcp":            "Speaker",       // AirPlay audio (HomePod, Apple TV audio)
	"_hap._tcp":             "Smart Device",  // HomeKit accessory
	"_homekit._tcp":         "Smart Device",
	"_spotify-connect._tcp": "Speaker",
	"_printer._tcp":         "Printer",
	"_ipp._tcp":             "Printer",
	"_pdl-datastream._tcp":  "Printer",
	"_workstation._tcp":     "Computer",
	"_ssh._tcp":             "Computer",
	"_smb._tcp":             "NAS",
	"_afpovertcp._tcp":      "Computer", // Apple File Protocol → Mac
	"_daap._tcp":            "Media Server",
	"_device-info._tcp":     "Apple Device",
}

// servicePriority controls which category wins when a device advertises
// multiple mDNS services. Higher number = higher priority.
var servicePriority = map[string]int{
	"_googlecast._tcp":      10,
	"_airplay._tcp":         9,
	"_raop._tcp":            8,
	"_spotify-connect._tcp": 7,
	"_hap._tcp":             6,
	"_homekit._tcp":         6,
	"_printer._tcp":         5,
	"_ipp._tcp":             5,
	"_pdl-datastream._tcp":  4,
	"_daap._tcp":            3,
	"_smb._tcp":             2,
	"_afpovertcp._tcp":      2,
	"_workstation._tcp":     1,
	"_ssh._tcp":             1,
	"_device-info._tcp":     0,
}

// ApplyMDNS overlays mDNS discovery data onto scan results.
//
// For each result whose IP matches an mDNS event:
//   - DeviceType is updated with the authoritative service-derived category.
//
// Device instance names (e.g. "Bedroom TV") are deliberately NOT auto-applied
// as labels — those are user-assigned and should remain under user control.
func ApplyMDNS(results []scanner.Result, events []scanner.MdnsEvent) {
	if len(events) == 0 {
		return
	}

	// Build IP → highest-priority event map.
	type entry struct {
		category string
		priority int
	}
	best := make(map[string]entry, len(events))
	for _, ev := range events {
		cat, ok := serviceCategory[ev.Service]
		if !ok {
			continue
		}
		pri := servicePriority[ev.Service]
		if prev, exists := best[ev.IP]; !exists || pri > prev.priority {
			best[ev.IP] = entry{cat, pri}
		}
	}

	for i := range results {
		if e, ok := best[results[i].IP]; ok {
			c := e.category
			results[i].DeviceType = &c
		}
	}
}
