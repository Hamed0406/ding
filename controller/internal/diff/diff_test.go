package diff

import (
	"testing"

	"github.com/ding/ding/internal/scanner"
)

func r(ip string) scanner.Result {
	return scanner.Result{IP: ip, Alive: true, OpenPorts: []uint16{}}
}

func known(ips ...string) map[string]bool {
	m := make(map[string]bool, len(ips))
	for _, ip := range ips {
		m[ip] = true
	}
	return m
}

func TestCompare_NewDevice(t *testing.T) {
	changes := Compare(nil, []scanner.Result{r("192.168.1.5")}, known())
	if len(changes) != 1 || changes[0].Kind != KindNew {
		t.Errorf("expected NEW, got %v", changes)
	}
}

func TestCompare_ReturningDevice_IsBack(t *testing.T) {
	// Device was seen before (in known), absent last scan, present now.
	prev := []scanner.Result{}
	curr := []scanner.Result{r("192.168.1.5")}
	changes := Compare(prev, curr, known("192.168.1.5"))
	if len(changes) != 1 || changes[0].Kind != KindBack {
		t.Errorf("expected BACK for known returning device, got %v", changes)
	}
}

func TestCompare_GoneDevice(t *testing.T) {
	prev := []scanner.Result{r("192.168.1.5")}
	curr := []scanner.Result{}
	changes := Compare(prev, curr, known("192.168.1.5"))
	if len(changes) != 1 || changes[0].Kind != KindGone {
		t.Errorf("expected GONE, got %v", changes)
	}
}

func TestCompare_PortChange(t *testing.T) {
	prev := []scanner.Result{{IP: "192.168.1.5", Alive: true, OpenPorts: []uint16{22}}}
	curr := []scanner.Result{{IP: "192.168.1.5", Alive: true, OpenPorts: []uint16{22, 80}}}
	changes := Compare(prev, curr, known("192.168.1.5"))
	if len(changes) != 1 || changes[0].Kind != KindPorts {
		t.Errorf("expected PORTS, got %v", changes)
	}
}

func TestCompare_NoChange(t *testing.T) {
	prev := []scanner.Result{r("192.168.1.5")}
	curr := []scanner.Result{r("192.168.1.5")}
	changes := Compare(prev, curr, known("192.168.1.5"))
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %v", changes)
	}
}

func TestCompare_EmptyBothSides(t *testing.T) {
	changes := Compare(nil, nil, known())
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %v", changes)
	}
}
