// Package scanner is the Go side of the scanner bridge.
//
// It defines the shared data types (Result, ArpEvent, MdnsEvent) that
// the Rust binary serialises as JSON on stdout and Go deserialises here,
// and provides three runner functions:
//
//   Run()     — active scan (ARP + ICMP + TCP); returns []Result
//   Listen()  — passive ARP listener; streams ArpEvent via a channel
//   RunMDNS() — mDNS discovery; returns []MdnsEvent after timeout
package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	IP         string     `json:"ip"`
	MAC        *string    `json:"mac"`
	Hostname   *string    `json:"hostname"`
	Vendor     *string    `json:"vendor"`
	DeviceType *string    `json:"device_type,omitempty"`
	OS         *string    `json:"os,omitempty"`
	Label      *string    `json:"label,omitempty"`
	Notify     bool       `json:"notify"`
	OpenPorts  []uint16   `json:"open_ports"`
	Alive      bool       `json:"alive"`
	Gateway    *string    `json:"gateway,omitempty"`
	TTL        *uint8     `json:"ttl,omitempty"`
	FirstSeen   *time.Time `json:"first_seen,omitempty"`   // wall-clock time this IP was first recorded alive
	LastSeen    *time.Time `json:"last_seen,omitempty"`    // most recent scan in which the device was alive
	MACConflict *string    `json:"mac_conflict,omitempty"` // previous MAC if current MAC differs — possible ARP spoofing
}

// ArpEvent is emitted by the scanner in --mode listen, one JSON line per event.
// It carries only the information visible in a passive ARP packet — no port data.
type ArpEvent struct {
	IP  string `json:"ip"`
	MAC string `json:"mac"`
}

// MdnsEvent is emitted by the scanner in --mode mdns, one JSON line per resolved
// service. It pairs an IP with the mDNS service type and the device's chosen name.
type MdnsEvent struct {
	IP      string `json:"ip"`
	Service string `json:"service"` // e.g. "_googlecast._tcp"
	Name    string `json:"name"`    // e.g. "Bedroom TV"
}

// Run invokes the Rust scanner binary in scan mode and returns parsed results.
// Set DING_SCANNER_BIN to override the default binary path.
func Run(iface, subnet, ports string, timeoutMs int) ([]Result, error) {
	bin := scannerBin()

	cmd := exec.Command(bin,
		"--interface", iface,
		"--subnet", subnet,
		"--ports", ports,
		"--timeout-ms", fmt.Sprintf("%d", timeoutMs),
	)

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("scanner exited %d: %s", ee.ExitCode(), ee.Stderr)
		}
		return nil, fmt.Errorf("scanner: %w", err)
	}

	var results []Result
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parse scanner output: %w", err)
	}
	return results, nil
}

// Listen starts the scanner binary in passive ARP listen mode and streams
// ArpEvent values to the returned channel. It stops when ctx is cancelled.
// The caller should drain the channel until it is closed.
func Listen(ctx context.Context, iface string) (<-chan ArpEvent, error) {
	cmd := exec.CommandContext(ctx, scannerBin(),
		"--interface", iface,
		"--mode", "listen",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("passive listener pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("passive listener start: %w", err)
	}

	events := make(chan ArpEvent, 64)
	go func() {
		defer close(events)
		defer cmd.Wait() //nolint:errcheck
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			var ev ArpEvent
			if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
				continue
			}
			select {
			case events <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, nil
}

// RunMDNS runs the scanner in --mode mdns for timeoutMs milliseconds.
// It sends PTR queries for common service types, collects responses, and
// returns the resolved events. Errors are non-fatal — callers should log
// them and continue without mDNS data rather than aborting the scan.
func RunMDNS(iface string, timeoutMs int) ([]MdnsEvent, error) {
	// Give the process a generous extra budget beyond the mDNS timeout.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(timeoutMs+3000)*time.Millisecond,
	)
	defer cancel()

	cmd := exec.CommandContext(ctx, scannerBin(),
		"--interface", iface,
		"--mode", "mdns",
		"--timeout-ms", fmt.Sprintf("%d", timeoutMs),
	)

	out, err := cmd.Output()
	if err != nil && ctx.Err() == nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("mdns scanner exited %d: %s", ee.ExitCode(), ee.Stderr)
		}
		return nil, fmt.Errorf("mdns scanner: %w", err)
	}

	var events []MdnsEvent
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev MdnsEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil && ev.IP != "" {
			events = append(events, ev)
		}
	}
	return events, nil
}

func scannerBin() string {
	if v := os.Getenv("DING_SCANNER_BIN"); v != "" {
		return v
	}
	return "/usr/local/bin/scanner"
}
