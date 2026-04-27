// ============================================================
// controller/internal/api/handlers.go — REST API endpoints
//
// Each function here handles one URL:
//   GET    /api/status                    → interface/subnet, last scan time
//   GET    /api/devices                   → all known devices (stable registry view)
//   GET    /api/devices/{ip}/history      → per-device scan history (last 100 scans)
//   GET    /api/history                   → last 20 scan records
//   GET    /api/topology                  → network graph (nodes + edges)
//   POST   /api/scan                      → trigger a new scan immediately
//   PUT    /api/devices/{ip}/label        → set a custom name for a device
//   DELETE /api/devices/{ip}/label        → remove a custom name
// ============================================================

package api

import (
	"encoding/json"
	"net/http"
	"strings"
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

// handleSetLabel responds to PUT /api/devices/{ip}/label
// Body: {"name": "Living Room Router"}
// Sets a persistent human-readable name for the device at {ip}.
func (s *Server) handleSetLabel(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must not be empty"})
		return
	}
	if err := s.store.SetLabel(ip, name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDelLabel responds to DELETE /api/devices/{ip}/label
// Removes any custom name previously set for the device at {ip}.
func (s *Server) handleDelLabel(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	if err := s.store.DeleteLabel(ip); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeviceHistory responds to GET /api/devices/{ip}/history
// Returns the last 100 scan entries for the given IP, oldest first.
func (s *Server) handleDeviceHistory(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	entries := s.store.DeviceHistory(ip, 100)
	if entries == nil {
		entries = []storage.DeviceHistoryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
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
