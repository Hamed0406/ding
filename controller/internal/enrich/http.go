// ============================================================
// controller/internal/enrich/http.go — HTTP banner fingerprinting
//
// Many embedded devices (cameras, routers, NAS boxes) serve a small
// HTTP management UI on port 80, 8000, or 8080. Their web servers
// identify themselves with distinctive Server headers (e.g.
// "Hikvision-Webs", "GoAhead-Webs", "Boa/0.94") and page titles that
// vendor/port classification can't see.
//
// BannerDeviceType probes each device that has port 80, 8000, or 8080
// open, reads the Server header and the first 2 KB of the body (for
// the <title> tag), and matches them against a fingerprint table.
//
// Port priority: 80 → 8000 → 8080. Only the first responding port is
// used (8000 is Hikvision's alternate HTTP management port).
//
// It only fills in DeviceType when the result has none yet, so it
// acts as a fallback beneath vendor/port classification. mDNS
// (applied after this in the pipeline) can still override the result.
// ============================================================

package enrich

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ding/ding/internal/scanner"
)

var titleRe = regexp.MustCompile(`(?i)<title[^>]*>([^<]{1,120})</title>`)

// serverFingerprints maps substrings of the HTTP Server header to device types.
// Checked case-insensitively. More specific strings must come first.
var serverFingerprints = []struct {
	sub   string
	dtype string
}{
	{"hikvision", "IP Camera"},
	{"dnvrs-webs", "IP Camera"},    // Dahua NVR/DVR
	{"dahua", "IP Camera"},
	{"goahead", "IP Camera"},       // GoAhead — used by Foscam, Reolink, and others
	{"app-webs", "IP Camera"},
	{"axis", "IP Camera"},
	{"vivotek", "IP Camera"},
	{"mjpg-streamer", "IP Camera"},
	{"ipcamera", "IP Camera"},
	{"surveillance", "IP Camera"},
	{"boa", "IP Camera"},           // Boa/0.94 — Foscam and many budget cameras
	{"cross web server", "IP Camera"},
	{"mini_httpd", "IP Camera"},    // mini_httpd — embedded camera web server
	{"yawcam", "IP Camera"},        // Yet Another Webcam
	{"openwrt", "Router"},
	{"uhttpd", "Router"},           // OpenWrt default server
	{"dd-wrt", "Router"},
	{"routeros", "Router"},         // MikroTik
	{"tomato", "Router"},
	{"synology", "NAS"},
	{"qnap", "NAS"},
	{"freenas", "NAS"},
	{"truenas", "NAS"},
	{"pfsense", "Firewall"},
	{"opnsense", "Firewall"},
}

// titleFingerprints maps substrings of the HTML <title> to device types.
var titleFingerprints = []struct {
	sub   string
	dtype string
}{
	{"ip camera", "IP Camera"},
	{"network camera", "IP Camera"},
	{"web camera", "IP Camera"},
	{"ipcam", "IP Camera"},
	{"dvr login", "IP Camera"},
	{"nvr login", "IP Camera"},
	{"digital video recorder", "IP Camera"},
	{"hikvision", "IP Camera"},
	{"dahua", "IP Camera"},
	{"openwrt", "Router"},
	{"dd-wrt", "Router"},
	{"routeros", "Router"},
	{"synology", "NAS"},
	{"qnap", "NAS"},
	{"freenas", "NAS"},
	{"truenas", "NAS"},
	{"pfsense", "Firewall"},
	{"opnsense", "Firewall"},
}

// BannerDeviceType probes HTTP on open ports and updates DeviceType for devices
// that couldn't be classified by vendor or port rules alone.
// workers controls parallelism; timeout applies per HTTP request.
func BannerDeviceType(results []scanner.Result, workers int, timeout time.Duration) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
		// Don't follow redirects — the initial response Server header is what we want.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range results {
		r := &results[i]
		// Only probe devices that don't yet have a DeviceType and have an HTTP port open.
		if r.DeviceType != nil {
			continue
		}
		url := httpURL(r.OpenPorts, r.IP)
		if url == "" {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(r *scanner.Result, url string) {
			defer wg.Done()
			defer func() { <-sem }()

			dt := probeHTTP(client, url)
			if dt == "" {
				return
			}
			mu.Lock()
			r.DeviceType = &dt
			mu.Unlock()
		}(r, url)
	}
	wg.Wait()
}

// httpURL returns the first HTTP URL to try for the given open ports.
// Priority: 80 → 8000 (Hikvision alternate) → 8080. Returns "" if none open.
func httpURL(ports []uint16, ip string) string {
	for _, p := range ports {
		if p == 80 {
			return "http://" + ip
		}
	}
	for _, p := range ports {
		if p == 8000 {
			return "http://" + ip + ":8000"
		}
	}
	for _, p := range ports {
		if p == 8080 {
			return "http://" + ip + ":8080"
		}
	}
	return ""
}

// probeHTTP makes one GET request and matches the response against fingerprints.
func probeHTTP(client *http.Client, url string) string {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Ding/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// Match on Server header first — most specific signal.
	server := strings.ToLower(resp.Header.Get("Server"))
	for _, fp := range serverFingerprints {
		if strings.Contains(server, fp.sub) {
			return fp.dtype
		}
	}

	// Fall back to HTML <title> — read at most 2 KB to keep things fast.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if m := titleRe.FindSubmatch(body); len(m) > 1 {
		title := strings.ToLower(strings.TrimSpace(string(m[1])))
		for _, fp := range titleFingerprints {
			if strings.Contains(title, fp.sub) {
				return fp.dtype
			}
		}
	}

	return ""
}
