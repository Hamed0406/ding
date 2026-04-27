// ============================================================
// controller/internal/api/handlers.go — REST API endpoints
//
// Each function here handles one URL:
//   GET  /api/status   → what interface/subnet are we on, last scan time
//   GET  /api/devices  → list of devices from the most recent scan
//   GET  /api/history  → last 20 scan records
//   GET  /api/topology → network graph (nodes + edges) for the topology map
//   POST /api/scan     → start a new scan right now
// ============================================================

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
	"github.com/ding/ding/internal/topology"
)

// writeJSON is a helper that sends any value as a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// handleStatus responds to GET /api/status
// Returns the network interface, subnet, and when the last scan ran.
// The UI uses this to populate the header bar.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	type response struct {
		Iface    string  `json:"iface"`     // e.g. "wlp0s20f3"
		Subnet   string  `json:"subnet"`    // e.g. "192.168.1.0/24"
		LastScan *string `json:"last_scan"` // ISO timestamp, or null if no scan yet
	}
	resp := response{Iface: s.cfg.Iface, Subnet: s.cfg.Subnet}

	// Look up when the last scan happened (nil if no scan has run yet)
	if rec := s.store.LatestRecord(); rec != nil {
		t := rec.ScannedAt.Format(time.RFC3339)
		resp.LastScan = &t
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleDevices responds to GET /api/devices
// Returns all devices ever seen, with the most recent data per device.
// alive=true only for devices present in the latest scan — gives a stable
// registry view so devices don't vanish after a single missed scan.
func (s *Server) handleDevices(w http.ResponseWriter, _ *http.Request) {
	devices := s.store.AllDevices()
	if devices == nil {
		devices = []scanner.Result{} // return an empty array, not null
	}
	writeJSON(w, http.StatusOK, devices)
}

// handleHistory responds to GET /api/history
// Returns the last 20 scan records (each with a timestamp and device list).
// Useful for seeing how your network looked in the past.
func (s *Server) handleHistory(w http.ResponseWriter, _ *http.Request) {
	records := s.store.History(20)
	if records == nil {
		records = []storage.Record{} // return an empty array, not null
	}
	writeJSON(w, http.StatusOK, records)
}

// handleScan responds to POST /api/scan
// Starts a new scan immediately. Returns 202 (Accepted) straight away —
// the actual results arrive later via the SSE stream (/api/events).
// Returns 409 (Conflict) if a scan is already running.
func (s *Server) handleScan(w http.ResponseWriter, _ *http.Request) {
	// Try to claim the "scanning" lock atomically.
	// If another scan is already running, CompareAndSwap returns false.
	if !s.scanning.CompareAndSwap(false, true) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "scan already in progress"})
		return
	}
	go s.runScan()                     // start scan in the background
	w.WriteHeader(http.StatusAccepted) // 202 = "got it, working on it"
}

// handleTopology responds to GET /api/topology
// Returns a graph of nodes and edges built from all known devices.
// Devices not in the latest scan are shown as offline (alive=false).
func (s *Server) handleTopology(w http.ResponseWriter, _ *http.Request) {
	devices := s.store.AllDevices()
	if devices == nil {
		devices = []scanner.Result{}
	}
	graph := topology.Build(devices)
	writeJSON(w, http.StatusOK, graph)
}
