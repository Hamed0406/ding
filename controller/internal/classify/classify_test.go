package classify

import (
	"testing"

	"github.com/ding/ding/internal/scanner"
)

func strp(s string) *string { return &s }

func TestClassify_PortRules(t *testing.T) {
	cases := []struct {
		ports []uint16
		want  string
	}{
		{[]uint16{8009}, "Streaming Dongle"}, // Chromecast
		{[]uint16{8060}, "Streaming Dongle"}, // Roku
		{[]uint16{62078}, "Phone"},           // iOS lockdown
		{[]uint16{9100}, "Printer"},          // JetDirect
		{[]uint16{554}, "IP Camera"},         // RTSP
		{[]uint16{1883}, "IoT Hub"},          // MQTT
		{[]uint16{7000}, "Apple TV"},
		{[]uint16{22, 80, 9100}, "Printer"}, // port rule wins over vendor
	}
	for _, tc := range cases {
		r := scanner.Result{OpenPorts: tc.ports}
		results := []scanner.Result{r}
		Annotate(results)
		if got := results[0].DeviceType; got == nil || *got != tc.want {
			t.Errorf("ports %v: want %q, got %v", tc.ports, tc.want, got)
		}
	}
}

func TestClassify_VendorRules(t *testing.T) {
	cases := []struct {
		vendor string
		want   string
	}{
		{"Teltonika Networks UAB", "Router"},
		{"Espressif Inc.", "Smart Device"},
		{"Tuya Smart", "Smart Device"},
		{"AMPAK Technology, Inc.", "Smart Device"},
		{"Raspberry Pi Trading Ltd", "Computer"},
		{"Synology Incorporated", "NAS"},
		{"Brother Industries, Ltd.", "Printer"},
		{"Siemens AG", "Appliance"},
		{"Nintendo Co., Ltd.", "Gaming Console"},
		{"Hikvision Digital Technology Co., Ltd.", "IP Camera"},
		{"Amazon Technologies Inc.", "Smart Speaker"},
		{"Sonos, Inc.", "Speaker"},
		{"Apple, Inc.", "Apple Device"},
	}
	for _, tc := range cases {
		r := scanner.Result{Vendor: strp(tc.vendor), OpenPorts: []uint16{}}
		results := []scanner.Result{r}
		Annotate(results)
		if got := results[0].DeviceType; got == nil || *got != tc.want {
			t.Errorf("vendor %q: want %q, got %v", tc.vendor, tc.want, got)
		}
	}
}

func TestClassify_Unknown(t *testing.T) {
	r := scanner.Result{OpenPorts: []uint16{22, 80}}
	results := []scanner.Result{r}
	Annotate(results)
	if results[0].DeviceType != nil {
		t.Errorf("unknown device should have nil DeviceType, got %q", *results[0].DeviceType)
	}
}

func TestClassify_PortBeatsVendor(t *testing.T) {
	// Port 62078 (iOS) should win even if vendor says "Apple Device"
	r := scanner.Result{
		Vendor:    strp("Apple, Inc."),
		OpenPorts: []uint16{62078},
	}
	results := []scanner.Result{r}
	Annotate(results)
	if got := results[0].DeviceType; got == nil || *got != "Phone" {
		t.Errorf("want Phone (port wins), got %v", got)
	}
}
