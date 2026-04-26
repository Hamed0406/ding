// Package topology builds a network graph from scan results.
// It identifies the gateway, groups devices, and produces
// a node+edge structure suitable for visualization.
package topology

import (
	"github.com/ding/ding/internal/scanner"
)

// NodeType classifies a device in the topology.
type NodeType string

const (
	NodeGateway NodeType = "gateway"
	NodeDevice  NodeType = "device"
)

// Node represents one device in the network graph.
type Node struct {
	ID        string   `json:"id"`                   // IP address (unique key)
	Label     string   `json:"label"`                // display name (hostname or IP)
	Type      NodeType `json:"type"`                 // "gateway" or "device"
	MAC       string   `json:"mac,omitempty"`        // hardware address
	Vendor    string   `json:"vendor,omitempty"`     // manufacturer
	OpenPorts []uint16 `json:"open_ports,omitempty"` // open TCP ports
	Alive     bool     `json:"alive"`                // responded to ping
}

// Edge represents a connection between two nodes.
type Edge struct {
	From string `json:"from"` // source node ID (usually gateway)
	To   string `json:"to"`   // target node ID
}

// Graph is the complete topology response.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Build constructs a topology graph from scan results.
// All devices on a LAN subnet connect to the gateway (star topology).
func Build(results []scanner.Result) Graph {
	var gatewayIP string
	nodeMap := make(map[string]bool)

	// Find the gateway IP from any result that has it set
	for _, r := range results {
		if r.Gateway != nil && *r.Gateway != "" {
			gatewayIP = *r.Gateway
			break
		}
	}

	var nodes []Node
	var edges []Edge

	// Add the gateway node if we found one
	if gatewayIP != "" {
		gw := Node{
			ID:    gatewayIP,
			Label: gatewayIP,
			Type:  NodeGateway,
			Alive: true,
		}
		// Check if the gateway itself was scanned (it often is)
		for _, r := range results {
			if r.IP == gatewayIP {
				gw.OpenPorts = r.OpenPorts
				gw.Alive = r.Alive
				if r.Hostname != nil {
					gw.Label = *r.Hostname
				}
				if r.MAC != nil {
					gw.MAC = *r.MAC
				}
				if r.Vendor != nil {
					gw.Vendor = *r.Vendor
				}
				break
			}
		}
		nodes = append(nodes, gw)
		nodeMap[gatewayIP] = true
	}

	// Add all device nodes and connect them to the gateway
	for _, r := range results {
		if nodeMap[r.IP] {
			continue // skip gateway, already added
		}

		label := r.IP
		if r.Hostname != nil {
			label = *r.Hostname
		}

		node := Node{
			ID:        r.IP,
			Label:     label,
			Type:      NodeDevice,
			OpenPorts: r.OpenPorts,
			Alive:     r.Alive,
		}
		if r.MAC != nil {
			node.MAC = *r.MAC
		}
		if r.Vendor != nil {
			node.Vendor = *r.Vendor
		}

		nodes = append(nodes, node)

		// Connect device to gateway (star topology)
		if gatewayIP != "" {
			edges = append(edges, Edge{From: gatewayIP, To: r.IP})
		}
	}

	// Handle no-gateway case: if no gateway detected, show devices without edges
	if nodes == nil {
		nodes = []Node{}
	}
	if edges == nil {
		edges = []Edge{}
	}

	return Graph{Nodes: nodes, Edges: edges}
}
