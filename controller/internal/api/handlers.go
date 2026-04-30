// ============================================================
// controller/internal/api/handlers.go — REST API endpoints
//
// Each function here handles one URL:
//   GET    /api/auth/providers              → which OAuth providers are configured
//   POST   /api/auth/register              → create a new account (email + password)
//   POST   /api/auth/login                 → sign in with email + password
//   POST   /api/auth/logout                → invalidate session cookie
//   GET    /api/auth/{provider}            → start OAuth flow (google / github)
//   GET    /api/auth/{provider}/callback   → OAuth callback
//   POST   /api/auth/exchange              → consume one-time OAuth exchange token
//   GET    /api/status                     → interface/subnet, last scan time
//   GET    /api/devices                    → all known devices (stable registry view)
//   GET    /api/devices/export             → download device list as CSV or JSON
//   GET    /api/devices/{ip}/history       → per-device scan history (last 100 scans)
//   GET    /api/changes                    → persistent change log (last 200 events)
//   GET    /api/history                    → last 20 scan records
//   GET    /api/topology                   → network graph (nodes + edges)
//   POST   /api/scan                       → trigger a new scan immediately
//   POST   /api/devices/{ip}/scan          → scan one device's ports immediately
//   POST   /api/devices/{ip}/wake          → send a Wake-on-LAN magic packet
//   PUT    /api/devices/{ip}/label         → set a custom name for a device
//   DELETE /api/devices/{ip}/label         → remove a custom name
//   PUT    /api/devices/{ip}/notify        → toggle per-device alerts
//   GET    /api/settings/telegram          → get current user's Telegram config
//   PUT    /api/settings/telegram          → save current user's Telegram token + chat ID
//   POST   /api/settings/telegram/test     → send a test Telegram message
//   GET    /api/settings/webhook           → get current user's webhook URL
//   PUT    /api/settings/webhook           → save current user's webhook URL
//   POST   /api/settings/webhook/test      → send a test webhook payload
//   POST   /api/speedtest                  → run an internet speed test (5–30 s)
//   GET    /api/speedtest/history          → last 20 speed test results
// ============================================================

package api

import (
	"encoding/csv"
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

	"github.com/ding/ding/internal/alert"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/speedtest"
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
	user, err := s.users.CreateUser(body.Email, string(hash))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create account"})
		return
	}
	tok := s.createSession(w, r, user.ID)
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
	tok := s.createSession(w, r, user.ID)
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
	userID, ok := s.consumeExchangeToken(body.Token)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}
	tok := s.createSession(w, r, userID)
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

// currentUserID returns the user ID for the authenticated request.
func (s *Server) currentUserID(r *http.Request) int64 {
	return s.sessions.userID(s.sessionToken(r))
}

// createSession creates a session token, sets the HttpOnly cookie, and returns the token
// so callers can also include it in the JSON response body.
// Returning it in the body lets the React app store it in localStorage and send it as
// Authorization: Bearer — a fallback for Cloudflare Tunnel, which strips Set-Cookie
// from certain response types before they reach the browser.
func (s *Server) createSession(w http.ResponseWriter, r *http.Request, userID int64) string {
	w.Header().Set("Cache-Control", "no-store, private")
	token := s.sessions.create(userID)
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

// handleExport responds to GET /api/devices/export
// Query param: format=csv (default) or format=json
// Returns all known devices as a downloadable file.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	devices := s.store.AllDevices()
	if devices == nil {
		devices = []scanner.Result{}
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}

	ts := time.Now().UTC().Format("2006-01-02")

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ding-devices-%s.json"`, ts))
		json.NewEncoder(w).Encode(devices) //nolint:errcheck
	default: // csv
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ding-devices-%s.csv"`, ts))

		cw := csv.NewWriter(w)
		cw.Write([]string{"IP", "MAC", "Hostname", "Label", "Vendor", "Device Type", "OS", "Open Ports", "Alive", "First Seen", "Last Seen"}) //nolint:errcheck
		for _, d := range devices {
			ports := make([]string, len(d.OpenPorts))
			for i, p := range d.OpenPorts {
				ports[i] = strconv.Itoa(int(p))
			}
			alive := "no"
			if d.Alive {
				alive = "yes"
			}
			firstSeen, lastSeen := "", ""
			if d.FirstSeen != nil {
				firstSeen = d.FirstSeen.UTC().Format(time.RFC3339)
			}
			if d.LastSeen != nil {
				lastSeen = d.LastSeen.UTC().Format(time.RFC3339)
			}
			cw.Write([]string{ //nolint:errcheck
				d.IP,
				strVal(d.MAC),
				strVal(d.Hostname),
				strVal(d.Label),
				strVal(d.Vendor),
				strVal(d.DeviceType),
				strVal(d.OS),
				strings.Join(ports, " "),
				alive,
				firstSeen,
				lastSeen,
			})
		}
		cw.Flush()
	}
}

// strVal dereferences a *string, returning "" if nil.
func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// handleChanges responds to GET /api/changes
// Returns the last n change-log entries (newest first). Default 200; override with ?limit=N.
func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	entries := s.store.Changes(limit)
	if entries == nil {
		entries = []storage.ChangeLogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
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

// handleSetNotify responds to PUT /api/devices/{ip}/notify
// Body: {"enabled": true|false}
// Enables or disables change alerts for the device at {ip}.
func (s *Server) handleSetNotify(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.store.SetNotify(ip, body.Enabled); err != nil {
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

// handleGetTelegram responds to GET /api/settings/telegram
// Returns the current user's Telegram bot token and chat ID (token masked for display).
func (s *Server) handleGetTelegram(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.users.GetTelegramConfig(s.currentUserID(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	// Mask the token: show only the first 10 chars so the user knows it's set,
	// without sending the full secret to the browser.
	masked := cfg.Token
	if len(masked) > 10 {
		masked = masked[:10] + strings.Repeat("•", len(masked)-10)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token_set": cfg.Token != "",
		"token_preview": masked,
		"chat_id":  cfg.ChatID,
	})
}

// handleSaveTelegram responds to PUT /api/settings/telegram
// Body: {"token": "...", "chat_id": "..."}
// If token is an empty string the existing stored token is preserved (allows updating
// only the chat ID without re-sending the secret).
func (s *Server) handleSaveTelegram(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token  string `json:"token"`
		ChatID string `json:"chat_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	userID := s.currentUserID(r)
	token := strings.TrimSpace(body.Token)
	chatID := strings.TrimSpace(body.ChatID)
	// Empty token + non-empty chatID means "keep the existing token, only update chat ID".
	// Empty token + empty chatID means "clear everything".
	if token == "" && chatID != "" {
		if existing, err := s.users.GetTelegramConfig(userID); err == nil {
			token = existing.Token
		}
	}
	if err := s.users.SaveTelegramConfig(userID, token, chatID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save settings"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestTelegram responds to POST /api/settings/telegram/test
// Sends a test message using the current user's saved Telegram config.
func (s *Server) handleTestTelegram(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.users.GetTelegramConfig(s.currentUserID(r))
	if err != nil || cfg.Token == "" || cfg.ChatID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no Telegram config saved — save your token and chat ID first"})
		return
	}
	testChange := []struct {
		s string
	}{{s: "test"}}
	_ = testChange
	if err := alert.SendTest(alert.Config{TelegramToken: cfg.Token, TelegramChatID: cfg.ChatID}); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "test message sent"})
}

// handleGetWebhook responds to GET /api/settings/webhook
// Returns the current user's webhook URL (masked to just the host for display).
func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	u, err := s.users.GetWebhookURL(s.currentUserID(r))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"url": "", "url_set": false})
		return
	}
	set := u != ""
	writeJSON(w, http.StatusOK, map[string]any{"url": u, "url_set": set})
}

// handleSaveWebhook responds to PUT /api/settings/webhook
// Body: {"url": "https://..."} — pass empty string to clear.
func (s *Server) handleSaveWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.users.SaveWebhookURL(s.currentUserID(r), body.URL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "saved"})
}

// handleTestWebhook responds to POST /api/settings/webhook/test
// Fires a test payload to the user's configured webhook URL.
func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	u, err := s.users.GetWebhookURL(s.currentUserID(r))
	if err != nil || u == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no webhook URL configured"})
		return
	}
	if err := alert.SendTestWebhook(u); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "test payload sent"})
}

// handleSpeedtest responds to POST /api/speedtest
// Runs a full speed test (ping + download + upload) against speed.cloudflare.com.
// Takes 5–30 s; returns 409 if a test is already running.
// Result is persisted in SQLite and returned in the response body.
func (s *Server) handleSpeedtest(w http.ResponseWriter, _ *http.Request) {
	if !s.speedtesting.CompareAndSwap(false, true) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "speed test already in progress"})
		return
	}
	defer s.speedtesting.Store(false)

	result, err := speedtest.Run()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	testedAt, _ := time.Parse(time.RFC3339, result.TestedAt)
	_ = s.store.SaveSpeedtest(storage.SpeedtestResult{
		TestedAt:     testedAt,
		DownloadMbps: result.DownloadMbps,
		UploadMbps:   result.UploadMbps,
		PingMs:       result.PingMs,
		Server:       result.Server,
	})

	writeJSON(w, http.StatusOK, result)
}

// handleSpeedtestHistory responds to GET /api/speedtest/history
// Returns the last 20 speed test results, newest first.
func (s *Server) handleSpeedtestHistory(w http.ResponseWriter, _ *http.Request) {
	results := s.store.SpeedtestHistory(20)
	if results == nil {
		results = []storage.SpeedtestResult{}
	}
	writeJSON(w, http.StatusOK, results)
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
