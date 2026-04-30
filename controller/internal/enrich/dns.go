// Package enrich layers additional intelligence onto raw scan results.
// The Rust scanner produces IP/MAC/port/TTL data; this package turns
// that into human-readable context. Enrichers run in pipeline order:
//
//  1. Hostnames        — reverse-DNS PTR lookup (16 workers, 300 ms each)
//  2. NetBIOSNames     — NetBIOS Node Status UDP 137 for hosts DNS missed (16 workers, 1 s each)
//  3. Vendor           — MAC OUI → manufacturer name (vendor package, inline)
//  4. Classify         — vendor + ports → device category (classify package, inline)
//  5. BannerDeviceType — HTTP Server header + <title> fingerprinting (8 workers, 800 ms)
//  6. SNMPDeviceType   — SNMP sysDescr + sysName via UDP 161 (16 workers, 800 ms)
//  7. RTSPDeviceType   — RTSP OPTIONS handshake on port 554 (8 workers, 600 ms)
//  8. ApplyMDNS        — overlay authoritative mDNS service-type categories
//  9. AnnotateOS       — TTL + hostname + device-type → OS family
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
