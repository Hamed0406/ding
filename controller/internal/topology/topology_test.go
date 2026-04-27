package topology

import (
	"testing"

	"github.com/ding/ding/internal/scanner"
)

func strPtr(s string) *string { return &s }

// twoHosts returns a gateway + one device, sharing the same gateway field.
func twoHosts() []scanner.Result {
	return []scanner.Result{
		{IP: "192.168.1.1", Gateway: strPtr("192.168.1.1"), Alive: true, MAC: strPtr("aa:bb:cc:dd:ee:01")},
		{IP: "192.168.1.10", Gateway: strPtr("192.168.1.1"), Alive: true, MAC: strPtr("aa:bb:cc:dd:ee:02")},
	}
}

func TestBuild_BasicStarTopology(t *testing.T) {
	g := Build(twoHosts())

	if len(g.Nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(g.Edges))
	}

	edge := g.Edges[0]
	if edge.From != "192.168.1.1" || edge.To != "192.168.1.10" {
		t.Errorf("unexpected edge %s→%s", edge.From, edge.To)
	}
}

func TestBuild_GatewayNodeType(t *testing.T) {
	g := Build(twoHosts())

	var gw *Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "192.168.1.1" {
			gw = &g.Nodes[i]
			break
		}
	}
	if gw == nil {
		t.Fatal("gateway node not found")
	}
	if gw.Type != NodeGateway {
		t.Errorf("want NodeGateway, got %q", gw.Type)
	}
	if !gw.Alive {
		t.Error("gateway found in results should be alive=true")
	}
}

// Gateway is in the results for the gateway field but was not scanned directly.
func TestBuild_GatewayNotInResults_AliveIsFalse(t *testing.T) {
	results := []scanner.Result{
		// Only one device; its gateway field points to an IP not in results.
		{IP: "192.168.1.10", Gateway: strPtr("192.168.1.1"), Alive: true},
	}
	g := Build(results)

	var gw *Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "192.168.1.1" {
			gw = &g.Nodes[i]
			break
		}
	}
	if gw == nil {
		t.Fatal("gateway node not found")
	}
	if gw.Alive {
		t.Error("gateway not in scan results should be alive=false")
	}
}

func TestBuild_NoGateway_NoEdges(t *testing.T) {
	results := []scanner.Result{
		{IP: "10.0.0.2", Alive: true},
		{IP: "10.0.0.3", Alive: false},
	}
	g := Build(results)

	if len(g.Edges) != 0 {
		t.Errorf("want 0 edges when no gateway, got %d", len(g.Edges))
	}
	if len(g.Nodes) != 2 {
		t.Errorf("want 2 device nodes, got %d", len(g.Nodes))
	}
}

func TestBuild_EmptyResults(t *testing.T) {
	g := Build(nil)

	if g.Nodes == nil || len(g.Nodes) != 0 {
		t.Errorf("want empty nodes slice, got %v", g.Nodes)
	}
	if g.Edges == nil || len(g.Edges) != 0 {
		t.Errorf("want empty edges slice, got %v", g.Edges)
	}
}

func TestBuild_GatewayDeduplication(t *testing.T) {
	// Gateway IP appears both as a host result and is referenced in Gateway fields.
	// It should only appear once in the node list.
	g := Build(twoHosts())

	seen := map[string]int{}
	for _, n := range g.Nodes {
		seen[n.ID]++
	}
	if seen["192.168.1.1"] != 1 {
		t.Errorf("gateway should appear exactly once, got %d times", seen["192.168.1.1"])
	}
}

func TestBuild_HostnameUsedAsLabel(t *testing.T) {
	results := []scanner.Result{
		{IP: "192.168.1.1", Gateway: strPtr("192.168.1.1"), Alive: true, Hostname: strPtr("router.lan")},
		{IP: "192.168.1.5", Gateway: strPtr("192.168.1.1"), Alive: true, Hostname: strPtr("laptop.lan")},
	}
	g := Build(results)

	for _, n := range g.Nodes {
		switch n.ID {
		case "192.168.1.1":
			if n.Label != "router.lan" {
				t.Errorf("gateway label: want router.lan, got %q", n.Label)
			}
		case "192.168.1.5":
			if n.Label != "laptop.lan" {
				t.Errorf("device label: want laptop.lan, got %q", n.Label)
			}
		}
	}
}
