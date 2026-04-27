package enrich

import (
	"testing"

	"github.com/ding/ding/internal/scanner"
)

func strp(s string) *string { return &s }

func TestApplyMDNS_SetsDeviceType(t *testing.T) {
	results := []scanner.Result{
		{IP: "192.168.1.5", OpenPorts: []uint16{}},
	}
	events := []scanner.MdnsEvent{
		{IP: "192.168.1.5", Service: "_googlecast._tcp", Name: "Bedroom TV"},
	}
	ApplyMDNS(results, events)
	if results[0].DeviceType == nil || *results[0].DeviceType != "Streaming Dongle" {
		t.Errorf("want Streaming Dongle, got %v", results[0].DeviceType)
	}
}

func TestApplyMDNS_HigherPriorityWins(t *testing.T) {
	// A HomePod advertises both _raop._tcp (Speaker) and _airplay._tcp (Apple TV).
	// _airplay._tcp has higher priority so it should win.
	results := []scanner.Result{
		{IP: "192.168.1.10", OpenPorts: []uint16{}},
	}
	events := []scanner.MdnsEvent{
		{IP: "192.168.1.10", Service: "_raop._tcp", Name: "HomePod"},
		{IP: "192.168.1.10", Service: "_airplay._tcp", Name: "HomePod"},
	}
	ApplyMDNS(results, events)
	if results[0].DeviceType == nil || *results[0].DeviceType != "Apple TV" {
		t.Errorf("want Apple TV (airplay wins), got %v", results[0].DeviceType)
	}
}

func TestApplyMDNS_OverridesExistingType(t *testing.T) {
	prev := "Computer"
	results := []scanner.Result{
		{IP: "192.168.1.7", DeviceType: &prev, OpenPorts: []uint16{}},
	}
	events := []scanner.MdnsEvent{
		{IP: "192.168.1.7", Service: "_printer._tcp", Name: "HP LaserJet"},
	}
	ApplyMDNS(results, events)
	if results[0].DeviceType == nil || *results[0].DeviceType != "Printer" {
		t.Errorf("mDNS should override classify guess; got %v", results[0].DeviceType)
	}
}

func TestApplyMDNS_UnknownServiceIgnored(t *testing.T) {
	results := []scanner.Result{
		{IP: "192.168.1.20", OpenPorts: []uint16{}},
	}
	events := []scanner.MdnsEvent{
		{IP: "192.168.1.20", Service: "_unknown._tcp", Name: "Mystery Box"},
	}
	ApplyMDNS(results, events)
	if results[0].DeviceType != nil {
		t.Errorf("unknown service should leave DeviceType nil, got %v", results[0].DeviceType)
	}
}

func TestApplyMDNS_NoEvents(t *testing.T) {
	dt := "Router"
	results := []scanner.Result{
		{IP: "192.168.1.1", DeviceType: &dt, OpenPorts: []uint16{}},
	}
	ApplyMDNS(results, nil)
	if results[0].DeviceType == nil || *results[0].DeviceType != "Router" {
		t.Errorf("empty events should leave results unchanged")
	}
}
