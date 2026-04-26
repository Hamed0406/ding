// Package enrich adds extra information to scan results that the
// low-level Rust scanner does not collect itself.
//
// Right now this means reverse-DNS lookups: given an IP address like
// 192.168.1.42, we ask the system resolver "what hostname goes with
// this IP?" and store the answer (e.g. "printer.local") on the result.
package enrich

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ding/ding/internal/scanner"
)

// LookupFunc is a swappable hostname resolver. The real implementation
// uses the OS resolver; tests inject a fake to avoid hitting the network.
type LookupFunc func(ctx context.Context, ip string) ([]string, error)

// defaultLookup uses Go's built-in resolver, which respects the system
// /etc/resolv.conf — so it works inside containers without extra setup.
func defaultLookup(ctx context.Context, ip string) ([]string, error) {
	return net.DefaultResolver.LookupAddr(ctx, ip)
}

// Hostnames fills in the Hostname field for every alive result that
// doesn't already have one. Lookups run concurrently with a small worker
// pool so a single slow/missing PTR record can't stall the whole scan.
//
//   - parallelism: max simultaneous DNS queries (e.g. 16)
//   - perLookup:   timeout per individual query (e.g. 300ms)
//
// Results are mutated in place. Failed lookups leave Hostname as nil.
func Hostnames(results []scanner.Result, parallelism int, perLookup time.Duration) {
	HostnamesWith(results, defaultLookup, parallelism, perLookup)
}

// HostnamesWith is the test-friendly version of Hostnames that takes an
// explicit lookup function. Production code should call Hostnames.
func HostnamesWith(results []scanner.Result, lookup LookupFunc, parallelism int, perLookup time.Duration) {
	if parallelism <= 0 {
		parallelism = 1
	}
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for i := range results {
		r := &results[i]
		// Skip if we already have a hostname or the host wasn't reachable.
		if r.Hostname != nil || !r.Alive {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(r *scanner.Result) {
			defer wg.Done()
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), perLookup)
			defer cancel()

			names, err := lookup(ctx, r.IP)
			if err != nil || len(names) == 0 {
				return
			}
			// LookupAddr returns FQDNs with a trailing dot ("router.lan.") —
			// strip it so the UI shows the cleaner "router.lan".
			name := strings.TrimSuffix(names[0], ".")
			if name == "" {
				return
			}
			r.Hostname = &name
		}(r)
	}
	wg.Wait()
}
