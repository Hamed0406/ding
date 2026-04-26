package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type Result struct {
	IP        string   `json:"ip"`
	MAC       *string  `json:"mac"`
	Hostname  *string  `json:"hostname"`
	Vendor    *string  `json:"vendor"`
	OpenPorts []uint16 `json:"open_ports"`
	Alive     bool     `json:"alive"`
}

// Run invokes the Rust scanner binary and returns parsed results.
// Set DING_SCANNER_BIN to override the default binary path.
func Run(iface, subnet, ports string, timeoutMs int) ([]Result, error) {
	bin := "/usr/local/bin/scanner"
	if v := os.Getenv("DING_SCANNER_BIN"); v != "" {
		bin = v
	}

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
