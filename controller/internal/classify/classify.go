// Package classify guesses a human-readable device category from the data
// Ding already has: open ports and MAC vendor name.
//
// Port fingerprints are checked first — they are very specific (e.g. port
// 62078 is exclusively the iOS lockdown service). Vendor substrings are a
// fallback when no port gives a strong signal.
//
// Add new rules to portTypes or vendorTypes; no other code needs changing.
package classify

import (
	"strings"

	"github.com/ding/ding/internal/scanner"
)

// Annotate sets DeviceType on each result in-place.
func Annotate(results []scanner.Result) {
	for i := range results {
		if dt := guess(&results[i]); dt != "" {
			results[i].DeviceType = &dt
		}
	}
}

func guess(r *scanner.Result) string {
	for _, rule := range portTypes {
		for _, p := range r.OpenPorts {
			if p == rule.port {
				return rule.kind
			}
		}
	}
	if r.Vendor != nil {
		v := strings.ToLower(*r.Vendor)
		for _, rule := range vendorTypes {
			if strings.Contains(v, rule.substr) {
				return rule.kind
			}
		}
	}
	return ""
}

var portTypes = []struct {
	port uint16
	kind string
}{
	{8009, "Streaming Dongle"}, // Chromecast
	{8060, "Streaming Dongle"}, // Roku
	{7000, "Apple TV"},         // AirPlay video
	{5009, "Apple TV"},         // Apple TV remote pairing
	{8008, "Smart TV"},         // Google Cast on TVs
	{62078, "Phone"},           // iOS lockdown service (iPhones / iPads only)
	{9100, "Printer"},          // JetDirect / HP network printing
	{515, "Printer"},           // LPD/LPR
	{631, "Printer"},           // IPP
	{554, "IP Camera"},         // RTSP video stream
	{1883, "IoT Hub"},          // MQTT broker
	{10243, "Smart TV"},        // Windows Media Player network sharing
	{3689, "Media Server"},     // DAAP (iTunes / Music)
	{5000, "NAS"},              // Synology DSM
	{5001, "NAS"},              // Synology DSM (HTTPS)
	{8080, "NAS"},              // QNAP web UI default
}

var vendorTypes = []struct {
	substr string
	kind   string
}{
	// ---- Networking gear ----
	{"teltonika", "Router"},
	{"mikrotik", "Router"},
	{"ubiquiti", "Router"},
	{"cisco", "Router"},
	{"netgear", "Router"},
	{"tp-link", "Router"},
	{"d-link", "Router"},
	{"asus", "Router"},
	{"linksys", "Router"},
	{"zyxel", "Router"},
	{"technicolor", "Router"},
	{"arris", "Router"},
	{"sagemcom", "Router"},
	{"actiontec", "Router"},
	{"fortinet", "Firewall"},
	{"palo alto", "Firewall"},
	{"watchguard", "Firewall"},

	// ---- Smart / IoT ----
	{"espressif", "Smart Device"},
	{"tuya", "Smart Device"},
	{"ampak technology", "Smart Device"},
	{"lumi united", "Smart Device"},    // Aqara / Xiaomi Zigbee
	{"shenzhen lumi", "Smart Device"},
	{"shenzhen bailing", "Smart Device"},
	{"wemo", "Smart Device"},
	{"signify", "Smart Device"}, // Philips Hue parent company
	{"philips lighting", "Smart Device"},
	{"ikea", "Smart Device"}, // Tradfri
	{"belkin", "Smart Device"},

	// ---- Computers ----
	{"raspberry pi", "Computer"},
	{"intel corporate", "Computer"},
	{"dell", "Computer"},
	{"hewlett packard", "Computer"},
	{"hp inc", "Computer"},
	{"lenovo", "Computer"},
	{"asustek", "Computer"},
	{"gigabyte", "Computer"},
	{"microsoft", "Computer"},
	{"vmware", "Computer"},
	{"parallels", "Computer"},

	// ---- Phones / Tablets ----
	{"apple", "Apple Device"}, // Mac, iPhone, iPad — port 62078 is more specific for iOS
	{"samsung", "Samsung Device"},
	{"oneplus", "Phone"},
	{"motorola", "Phone"},
	{"xiaomi", "Phone"},
	{"huawei", "Phone"},
	{"realme", "Phone"},
	{"oppo", "Phone"},

	// ---- Smart Speakers ----
	{"amazon technologies", "Smart Speaker"},
	{"amazon", "Smart Speaker"},
	{"sonos", "Speaker"},
	{"harman", "Speaker"}, // JBL / Harman Kardon
	{"bose", "Speaker"},

	// ---- Streaming / Google ecosystem ----
	{"google", "Google Device"}, // Chromecast, Nest, Home — port 8009 is more specific

	// ---- Smart TVs ----
	{"lg electronics", "Smart TV"},
	{"vizio", "Smart TV"},
	{"hisense", "Smart TV"},
	{"tcl", "Smart TV"},
	{"sony", "Smart TV"}, // Bravia TVs — "sony mobile" is matched above this

	// ---- Printers ----
	{"brother", "Printer"},
	{"canon", "Printer"},
	{"epson", "Printer"},
	{"lexmark", "Printer"},
	{"xerox", "Printer"},
	{"ricoh", "Printer"},
	{"konica", "Printer"},

	// ---- NAS / Storage ----
	{"synology", "NAS"},
	{"qnap", "NAS"},
	{"western digital", "NAS"},
	{"buffalo", "NAS"},

	// ---- Home Appliances ----
	{"siemens", "Appliance"},
	{"bosch", "Appliance"},
	{"miele", "Appliance"},

	// ---- Gaming Consoles ----
	{"nintendo", "Gaming Console"},
	{"sony interactive", "Gaming Console"}, // PlayStation — checked before "sony" above
	{"valve", "Gaming Console"},

	// ---- Security Cameras ----
	{"hikvision", "IP Camera"},
	{"dahua", "IP Camera"},
	{"axis communications", "IP Camera"},
	{"hanwha", "IP Camera"},
	{"reolink", "IP Camera"},
	{"amcrest", "IP Camera"},
	{"foscam", "IP Camera"},
	{"uniview", "IP Camera"},
	{"vivotek", "IP Camera"},
	{"pelco", "IP Camera"},
	{"avigilon", "IP Camera"},
	{"bosch security", "IP Camera"},

	// ---- Phones (less common vendors) ----
	{"vivo mobile", "Phone"},
	{"hmd global", "Phone"},  // Nokia phones
	{"sony mobile", "Phone"}, // Xperia — must come before generic "sony"
}
