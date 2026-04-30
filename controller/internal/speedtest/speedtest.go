// controller/internal/speedtest/speedtest.go — internet speed test
//
// Uses speed.cloudflare.com (no extra dependencies):
//   Ping     — HEAD /__down?bytes=0, averaged over 3 requests
//   Download — GET  /__down?bytes=25000000 (25 MB), measure throughput
//   Upload   — POST /__up with 10 MB body, measure throughput

package speedtest

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// Result holds the output of one speed test run.
type Result struct {
	DownloadMbps float64 `json:"download_mbps"`
	UploadMbps   float64 `json:"upload_mbps"`
	PingMs       float64 `json:"ping_ms"`
	Server       string  `json:"server"`
	TestedAt     string  `json:"tested_at"`
}

const (
	downloadURL = "https://speed.cloudflare.com/__down?bytes=25000000"
	uploadURL   = "https://speed.cloudflare.com/__up"
	pingURL     = "https://speed.cloudflare.com/__down?bytes=0"
	uploadSize  = 10 * 1024 * 1024 // 10 MB
)

var httpClient = &http.Client{Timeout: 90 * time.Second}

// Run performs a full speed test and returns the result.
// Typical duration: 5–30 s depending on line speed and latency.
func Run() (Result, error) {
	pingMs, err := measurePing()
	if err != nil {
		return Result{}, fmt.Errorf("ping: %w", err)
	}

	downMbps, err := measureDownload()
	if err != nil {
		return Result{}, fmt.Errorf("download: %w", err)
	}

	upMbps, err := measureUpload()
	if err != nil {
		return Result{}, fmt.Errorf("upload: %w", err)
	}

	return Result{
		DownloadMbps: round2(downMbps),
		UploadMbps:   round2(upMbps),
		PingMs:       round2(pingMs),
		Server:       "speed.cloudflare.com",
		TestedAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func measurePing() (float64, error) {
	const n = 3
	var total float64
	for i := 0; i < n; i++ {
		start := time.Now()
		resp, err := httpClient.Head(pingURL)
		if err != nil {
			return 0, err
		}
		resp.Body.Close()
		total += float64(time.Since(start).Milliseconds())
	}
	return total / n, nil
}

func measureDownload() (float64, error) {
	start := time.Now()
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}
	return toMbps(n, time.Since(start).Seconds()), nil
}

func measureUpload() (float64, error) {
	buf := make([]byte, uploadSize)
	if _, err := rand.Read(buf); err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := httpClient.Post(uploadURL, "application/octet-stream", bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	return toMbps(uploadSize, time.Since(start).Seconds()), nil
}

func toMbps(b int64, seconds float64) float64 {
	return (float64(b) * 8) / (seconds * 1e6)
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
