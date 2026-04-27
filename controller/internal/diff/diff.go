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
	KindBack  ChangeKind = "BACK" // device returned after being absent in the previous scan
)

type Change struct {
	Kind ChangeKind `json:"kind"`
	IP   string     `json:"ip"`
	Desc string     `json:"desc"`
}

func (c Change) String() string {
	return fmt.Sprintf("[%s] %s — %s", c.Kind, c.IP, c.Desc)
}

// Compare produces a list of changes between two consecutive scans.
// known is the set of all IPs ever recorded (from store.AllKnownIPs).
// An IP absent from previous but present in current is NEW only if it has
// never been seen before; otherwise it is BACK (returned after absence).
func Compare(previous, current []scanner.Result, known map[string]bool) []Change {
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
			vendorDesc := ""
			if r.Vendor != nil {
				vendorDesc = " vendor=" + *r.Vendor
			}
			kind := KindNew
			if known[ip] {
				kind = KindBack
			}
			changes = append(changes, Change{
				Kind: kind,
				IP:   ip,
				Desc: fmt.Sprintf("mac=%s%s%s ports=%v", mac, host, vendorDesc, r.OpenPorts),
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
