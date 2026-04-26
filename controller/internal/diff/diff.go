package diff

import (
	"fmt"
	"sort"

	"github.com/ding/ding/internal/scanner"
)

type ChangeKind string

const (
	KindNew   ChangeKind = "NEW"
	KindGone  ChangeKind = "GONE"
	KindPorts ChangeKind = "PORTS"
)

type Change struct {
	Kind ChangeKind `json:"kind"`
	IP   string     `json:"ip"`
	Desc string     `json:"desc"`
}

func (c Change) String() string {
	return fmt.Sprintf("[%s] %s — %s", c.Kind, c.IP, c.Desc)
}

func Compare(previous, current []scanner.Result) []Change {
	prev := index(previous)
	curr := index(current)
	var changes []Change

	for ip, r := range curr {
		if _, seen := prev[ip]; !seen {
			mac := ""
			if r.MAC != nil {
				mac = *r.MAC
			}
			host := ""
			if r.Hostname != nil {
				host = " host=" + *r.Hostname
			}
			changes = append(changes, Change{
				Kind: KindNew,
				IP:   ip,
				Desc: fmt.Sprintf("mac=%s%s ports=%v", mac, host, r.OpenPorts),
			})
			continue
		}
		if !portsEqual(prev[ip].OpenPorts, r.OpenPorts) {
			changes = append(changes, Change{
				Kind: KindPorts,
				IP:   ip,
				Desc: fmt.Sprintf("was=%v now=%v", prev[ip].OpenPorts, r.OpenPorts),
			})
		}
	}

	for ip := range prev {
		if _, still := curr[ip]; !still {
			changes = append(changes, Change{Kind: KindGone, IP: ip, Desc: "no longer visible"})
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].IP < changes[j].IP
	})
	return changes
}

func index(results []scanner.Result) map[string]scanner.Result {
	m := make(map[string]scanner.Result, len(results))
	for _, r := range results {
		m[r.IP] = r
	}
	return m
}

func portsEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[uint16]struct{}, len(a))
	for _, p := range a {
		set[p] = struct{}{}
	}
	for _, p := range b {
		if _, ok := set[p]; !ok {
			return false
		}
	}
	return true
}
