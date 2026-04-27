package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type Result struct {
	IP         string   `json:"ip"`
	MAC        *string  `json:"mac"`
	Hostname   *string  `json:"hostname"`
	Vendor     *string  `json:"vendor"`
	DeviceType *string  `json:"device_type,omitempty"`
	OpenPorts  []uint16 `json:"open_ports"`
	Alive      bool     `json:"alive"`
	Gateway    *string  `json:"gateway,omitempty"`
	TTL        *uint8   `json:"ttl,omitempty"`
}

// ArpEvent is emitted by the scanner in --mode listen, one JSON line per event.
// It carries only the information visible in a passive ARP packet — no port data.
type ArpEvent struct {
	IP  string `json:"ip"`
	MAC string `json:"mac"`
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

func scannerBin() string {
	if v := os.Getenv("DING_SCANNER_BIN"); v != "" {
		return v
	}
	return "/usr/local/bin/scanner"
}
