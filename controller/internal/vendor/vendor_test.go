package vendor

import (
	"strings"
	"testing"
)

// Known OUI prefixes from the IEEE database. Names use Contains to stay
// resilient to IEEE renaming (e.g. "Apple, Inc." → "Apple Inc.").
var knownOUIs = []struct {
	mac, want string
}{
	{"3C:22:FB:00:00:00", "Apple"},                // common Apple prefix
	{"B8:27:EB:11:22:33", "Raspberry Pi"},         // original Raspberry Pi Foundation
	{"DC:A6:32:aa:bb:cc", "Raspberry Pi"},         // newer Pi prefix, lowercase
	{"00-50-56-AB-CD-EF", "VMware"},               // VMware (dash format)
	{"00:1B:63:00:00:00", "Apple"},                // Apple, alternate prefix
}

func TestLookup_KnownVendors(t *testing.T) {
	for _, tc := range knownOUIs {
		got := Lookup(tc.mac)
		if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.want)) {
			t.Errorf("Lookup(%q) = %q, want it to contain %q", tc.mac, got, tc.want)
		}
	}
}

func TestLookup_UnknownPrefix(t *testing.T) {
	// 02:00:00 is locally administered — not in the IEEE registry.
	if got := Lookup("02:00:00:11:22:33"); got != "" {
		t.Errorf("expected empty string for unknown prefix, got %q", got)
	}
}

func TestLookup_Malformed(t *testing.T) {
	cases := []string{
		"",                        // empty
		"not-a-mac",               // garbage
		"GG:HH:II:JJ:KK:LL",       // non-hex
		"12:34",                   // too short
	}
	for _, mac := range cases {
		if got := Lookup(mac); got != "" {
			t.Errorf("Lookup(%q) = %q, want empty", mac, got)
		}
	}
}

func TestLookup_FormatTolerance(t *testing.T) {
	// Same Apple OUI in three formats — all must resolve to the same vendor.
	expected := Lookup("3C:22:FB:00:00:00")
	if expected == "" {
		t.Fatal("baseline lookup returned empty; OUI database may be missing the prefix")
	}

	other := []string{
		"3c-22-fb-00-00-00",
		"3c22fb000000",
	}
	for _, m := range other {
		if got := Lookup(m); got != expected {
			t.Errorf("Lookup(%q) = %q, want %q", m, got, expected)
		}
	}
}

// Ensures the embedded database actually loaded with a sane number of entries.
// The IEEE registry has tens of thousands of OUIs — anything dramatically
// lower means the parser is broken.
func TestDatabaseLoaded(t *testing.T) {
	_ = Lookup("00:00:00:00:00:00") // force lazy load
	if len(ouiMap) < 10000 {
		t.Errorf("OUI map only has %d entries, expected > 10000", len(ouiMap))
	}
}
