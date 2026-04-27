// ============================================================
// controller/internal/enrich/rtsp.go — RTSP application-layer probe
//
// Port 554 being open only proves TCP connectivity. Some cameras need
// the RTSP OPTIONS handshake to respond, and others accept TCP on 554
// but aren't IP cameras (e.g. some NAS boxes use 554 for media sharing).
//
// RTSPDeviceType connects to port 554 on devices that don't yet have a
// DeviceType, sends a minimal RTSP OPTIONS request, and marks them as
// "IP Camera" if the server replies with a valid RTSP/1.x response line.
//
// This runs after classify (which detects 554 open = "IP Camera" by port
// rule) so in practice it only fires for devices where 554 is open but
// classify somehow missed them, or when a future rule change removes the
// port-based rule. It is safe to run as a belt-and-suspenders check.
// ============================================================

package enrich

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ding/ding/internal/scanner"
)

// RTSPDeviceType probes port 554 on devices without a DeviceType and marks
// confirmed RTSP servers as "IP Camera". workers controls parallelism;
// timeout applies per connection + read.
func RTSPDeviceType(results []scanner.Result, workers int, timeout time.Duration) {
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range results {
		r := &results[i]
		if r.DeviceType != nil {
			continue
		}
		if !hasPort(r.OpenPorts, 554) {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(r *scanner.Result) {
			defer wg.Done()
			defer func() { <-sem }()

			if probeRTSP(r.IP, timeout) {
				dt := "IP Camera"
				mu.Lock()
				r.DeviceType = &dt
				mu.Unlock()
			}
		}(r)
	}
	wg.Wait()
}

// probeRTSP dials port 554 and sends OPTIONS; returns true if the server
// replies with an RTSP/1.x status line.
func probeRTSP(ip string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", ip+":554", timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout)) //nolint:errcheck

	fmt.Fprint(conn, "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: Ding/1.0\r\n\r\n")

	line, err := bufio.NewReader(conn).ReadString('\n')
	return err == nil && strings.HasPrefix(strings.TrimSpace(line), "RTSP/")
}

// hasPort reports whether port p is in the slice.
func hasPort(ports []uint16, p uint16) bool {
	for _, pp := range ports {
		if pp == p {
			return true
		}
	}
	return false
}
