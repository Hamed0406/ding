package enrich

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ding/ding/internal/scanner"
)

func aliveResult(ip string) scanner.Result {
	return scanner.Result{IP: ip, Alive: true, OpenPorts: []uint16{}}
}

// Happy path: lookup returns a name, it should land on Hostname (no trailing dot).
func TestHostnamesWith_PopulatesHostname(t *testing.T) {
	results := []scanner.Result{aliveResult("192.168.1.1")}

	lookup := func(_ context.Context, ip string) ([]string, error) {
		if ip != "192.168.1.1" {
			t.Fatalf("unexpected ip: %s", ip)
		}
		return []string{"router.lan."}, nil
	}

	HostnamesWith(results, lookup, 4, time.Second)

	if results[0].Hostname == nil {
		t.Fatal("expected hostname to be set")
	}
	if got := *results[0].Hostname; got != "router.lan" {
		t.Fatalf("expected trailing dot stripped, got %q", got)
	}
}

// A lookup error must not panic and must leave Hostname nil.
func TestHostnamesWith_ErrorLeavesNil(t *testing.T) {
	results := []scanner.Result{aliveResult("10.0.0.5")}

	lookup := func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("nxdomain")
	}

	HostnamesWith(results, lookup, 4, time.Second)

	if results[0].Hostname != nil {
		t.Fatalf("expected nil hostname on error, got %q", *results[0].Hostname)
	}
}

// A lookup that returns no names must leave Hostname nil.
func TestHostnamesWith_EmptyResultLeavesNil(t *testing.T) {
	results := []scanner.Result{aliveResult("10.0.0.6")}

	lookup := func(_ context.Context, _ string) ([]string, error) {
		return []string{}, nil
	}

	HostnamesWith(results, lookup, 4, time.Second)

	if results[0].Hostname != nil {
		t.Fatal("expected nil hostname on empty result")
	}
}

// Dead hosts (Alive=false) and pre-populated hostnames should be skipped entirely.
func TestHostnamesWith_SkipsDeadAndPrefilled(t *testing.T) {
	preset := "already.local"
	results := []scanner.Result{
		{IP: "10.0.0.7", Alive: false},                      // dead → skip
		{IP: "10.0.0.8", Alive: true, Hostname: &preset},    // already named → skip
	}

	calls := 0
	lookup := func(_ context.Context, _ string) ([]string, error) {
		calls++
		return []string{"should-not-be-used."}, nil
	}

	HostnamesWith(results, lookup, 4, time.Second)

	if calls != 0 {
		t.Fatalf("lookup should not have been called, got %d calls", calls)
	}
	if results[0].Hostname != nil {
		t.Fatal("dead host should remain unnamed")
	}
	if *results[1].Hostname != "already.local" {
		t.Fatalf("pre-set hostname mutated: %q", *results[1].Hostname)
	}
}

// A slow lookup must be cancelled by the per-lookup timeout, leaving Hostname nil.
func TestHostnamesWith_TimeoutCancels(t *testing.T) {
	results := []scanner.Result{aliveResult("10.0.0.9")}

	lookup := func(ctx context.Context, _ string) ([]string, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return []string{"too-slow."}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	start := time.Now()
	HostnamesWith(results, lookup, 4, 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 300*time.Millisecond {
		t.Fatalf("timeout did not cancel slow lookup; took %v", elapsed)
	}
	if results[0].Hostname != nil {
		t.Fatal("expected nil hostname when lookup times out")
	}
}
