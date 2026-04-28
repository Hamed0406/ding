// ============================================================
// controller/internal/api/handlers.go — REST API endpoints
//
// Each function here handles one URL:
//   GET    /api/auth/providers            → which OAuth providers are configured
//   POST   /api/auth/register             → create a new account (email + password)
//   POST   /api/auth/login                → sign in with email + password
//   POST   /api/auth/logout               → invalidate session cookie
//   GET    /api/auth/{provider}           → start OAuth flow (google / github)
//   GET    /api/auth/{provider}/callback  → OAuth callback
//   GET    /api/status                    → interface/subnet, last scan time
//   GET    /api/devices                   → all known devices (stable registry view)
//   GET    /api/devices/{ip}/history      → per-device scan history (last 100 scans)
//   GET    /api/history                   → last 20 scan records
//   GET    /api/topology                  → network graph (nodes + edges)
//   POST   /api/scan                      → trigger a new scan immediately
//   POST   /api/devices/{ip}/wake         → send a Wake-on-LAN magic packet
//   PUT    /api/devices/{ip}/label        → set a custom name for a device
//   DELETE /api/devices/{ip}/label        → remove a custom name
// ============================================================

package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

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

// handleAuthProviders responds to GET /api/auth/providers
// Returns which OAuth providers are configured so the UI can show the right buttons.
// Also returns whether any users exist (for first-run "create account" detection).
func (s *Server) handleAuthProviders(w http.ResponseWriter, _ *http.Request) {
	count, _ := s.users.UserCount()
	writeJSON(w, http.StatusOK, map[string]any{
		"google":     s.cfg.Google.ClientID != "",
		"github":     s.cfg.GitHub.ClientID != "",
		"has_users":  count > 0,
	})
}

// handleRegister responds to POST /api/auth/register
// Body: {"email": "...", "password": "..."}
// Creates a new account, bcrypt-hashes the password, and sets a session cookie.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
		return
	}
	if len(body.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}
	// Check duplicate
	existing, err := s.users.FindUserByEmail(body.Email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "an account with that email already exists"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 12)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if _, err := s.users.CreateUser(body.Email, string(hash)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create account"})
		return
	}
	tok := s.createSession(w, r)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "token": tok})
}

// handleLogin responds to POST /api/auth/login
// Body: {"email": "...", "password": "..."}
// Verifies bcrypt hash and sets a session cookie on success.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	user, err := s.users.FindUserByEmail(body.Email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if user == nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
		time.Sleep(500 * time.Millisecond) // slow brute-force
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "incorrect email or password"})
		return
	}
	tok := s.createSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": tok})
}

// handleExchange responds to POST /api/auth/exchange
// Consumes a one-time exchange token (set in the URL by the OAuth callback) and
// sets a session cookie. This indirection exists because Cloudflare Tunnel strips
// Set-Cookie headers from redirect responses; a regular fetch response is unaffected.
func (s *Server) handleExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}
	if !s.consumeExchangeToken(body.Token) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}
	tok := s.createSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": tok})
}

// handleLogout responds to POST /api/auth/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if tok := s.sessionToken(r); tok != "" {
		s.sessions.delete(tok)
	}
	http.SetCookie(w, &http.Cookie{Name: "ding_session", Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// createSession creates a session token, sets the HttpOnly cookie, and returns the token
// so callers can also include it in the JSON response body.
// Returning it in the body lets the React app store it in localStorage and send it as
// Authorization: Bearer — a fallback for Cloudflare Tunnel, which strips Set-Cookie
// from certain response types before they reach the browser.
func (s *Server) createSession(w http.ResponseWriter, r *http.Request) string {
	w.Header().Set("Cache-Control", "no-store, private")
	token := s.sessions.create()
	// Set Secure flag when the request arrived over HTTPS (direct TLS or Cloudflare Tunnel).
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "ding_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	return token
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

// handleDeviceScan responds to POST /api/devices/{ip}/scan
// Runs an immediate parallel TCP port scan against the single device and
// returns the open ports as JSON — no Rust subprocess, no SSE, result is instant.
func (s *Server) handleDeviceScan(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	ports := parsePorts(s.cfg.Ports)
	timeout := time.Duration(s.cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}

	open := tcpScanOne(ip, ports, timeout)

	type response struct {
		IP        string   `json:"ip"`
		OpenPorts []uint16 `json:"open_ports"`
	}
	writeJSON(w, http.StatusOK, response{IP: ip, OpenPorts: open})
}

// tcpScanOne probes all ports on a single IP in parallel and returns the open ones sorted.
func tcpScanOne(ip string, ports []uint16, timeout time.Duration) []uint16 {
	var mu sync.Mutex
	var open []uint16
	var wg sync.WaitGroup
	sem := make(chan struct{}, 50) // max 50 concurrent dials

	for _, port := range ports {
		wg.Add(1)
		sem <- struct{}{}
		go func(p uint16) {
			defer wg.Done()
			defer func() { <-sem }()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, p), timeout)
			if err == nil {
				conn.Close()
				mu.Lock()
				open = append(open, p)
				mu.Unlock()
			}
		}(port)
	}
	wg.Wait()

	sort.Slice(open, func(i, j int) bool { return open[i] < open[j] })
	return open
}

// parsePorts converts a comma-separated port string into a []uint16.
func parsePorts(s string) []uint16 {
	var ports []uint16
	for _, p := range strings.Split(s, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(p), 10, 16)
		if err == nil {
			ports = append(ports, uint16(n))
		}
	}
	return ports
}

// handleWake responds to POST /api/devices/{ip}/wake
// Looks up the device's MAC address and sends a Wake-on-LAN magic packet via UDP broadcast.
func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")

	var mac string
	for _, d := range s.store.AllDevices() {
		if d.IP == ip && d.MAC != nil && *d.MAC != "" {
			mac = *d.MAC
			break
		}
	}
	if mac == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "MAC address unknown for this device"})
		return
	}
	if err := sendMagicPacket(mac); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sendMagicPacket builds a 102-byte WoL magic packet and broadcasts it on UDP port 9.
// Format: 6 bytes of 0xFF followed by the target MAC repeated 16 times.
func sendMagicPacket(mac string) error {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return fmt.Errorf("invalid MAC %q: %w", mac, err)
	}
	var packet [102]byte
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 1; i <= 16; i++ {
		copy(packet[i*6:(i+1)*6], hw)
	}
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4bcast, Port: 9})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(packet[:])
	return err
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
