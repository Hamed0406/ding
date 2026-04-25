package iface

import (
	"fmt"
	"net"
	"strings"
)

type Interface struct {
	Name   string
	Subnet string // network CIDR, e.g. "192.168.1.0/24"
}

// All returns every up, non-loopback, non-virtual interface that has an IPv4 address.
func All() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	var out []Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtual(iface.Name) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			// Normalize host address to network address (e.g. 192.168.1.175/24 → 192.168.1.0/24)
			_, network, err := net.ParseCIDR(ipNet.String())
			if err != nil {
				continue
			}
			out = append(out, Interface{Name: iface.Name, Subnet: network.String()})
		}
	}
	return out, nil
}

// Detect returns the first suitable interface. Use All() if you need the full list.
func Detect() (Interface, error) {
	ifaces, err := All()
	if err != nil {
		return Interface{}, err
	}
	if len(ifaces) == 0 {
		return Interface{}, fmt.Errorf("no suitable network interface found (all are loopback, virtual, or have no IPv4)")
	}
	return ifaces[0], nil
}

// isVirtual returns true for loopback, docker bridges, veth pairs, tunnels, etc.
func isVirtual(name string) bool {
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "tun", "tap", "dummy", "bond"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
