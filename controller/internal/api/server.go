// ============================================================
// controller/internal/api/server.go — HTTP server setup
//
// This file wires everything together:
//   - Embeds the React UI files into the Go binary (go:embed)
//   - Registers all URL routes (which function handles which URL)
//   - Runs scans and publishes the results via SSE
//   - Manages session-based authentication and OAuth flows
// ============================================================

package api

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"

	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

//go:embed static
var staticFiles embed.FS

// ScanFunc is the type of function that runs one network scan.
type ScanFunc func() ([]scanner.Result, []diff.Change, error)

// OAuthConfig holds credentials for a single OAuth provider.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
}

// Config holds the info the API layer needs.
type Config struct {
	Iface     string
	Subnet    string
	Ports     string
	TimeoutMs int
	BaseURL   string      // e.g. "https://ding.montra.me" — set when behind a reverse proxy
	Google    OAuthConfig // empty = Google OAuth disabled
	GitHub    OAuthConfig // empty = GitHub OAuth disabled
}

// sseEvent is the shape of every message we push to the browser.
type sseEvent struct {
	Type      string           `json:"type"`
	Error     string           `json:"error,omitempty"`
	Devices   []scanner.Result `json:"devices,omitempty"`
	Changes   []diff.Change    `json:"changes,omitempty"`
	ScannedAt string           `json:"scanned_at,omitempty"`
	IP        string           `json:"ip,omitempty"`
	MAC       string           `json:"mac,omitempty"`
	Kind      diff.ChangeKind  `json:"kind,omitempty"`
}

// sessionEntry holds the expiry and the ID of the user who owns the session.
type sessionEntry struct {
	expiry time.Time
	userID int64
}

// sessionStore holds active login tokens (random 32-byte hex, 24h expiry).
type sessionStore struct {
	mu     sync.Mutex
	tokens map[string]sessionEntry
}

func newSessionStore() *sessionStore {
	return &sessionStore{tokens: make(map[string]sessionEntry)}
}

func (ss *sessionStore) create(userID int64) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	t := hex.EncodeToString(b)
	ss.mu.Lock()
	ss.tokens[t] = sessionEntry{expiry: time.Now().Add(24 * time.Hour), userID: userID}
	ss.mu.Unlock()
	return t
}

func (ss *sessionStore) valid(token string) bool {
	ss.mu.Lock()
	e, ok := ss.tokens[token]
	ss.mu.Unlock()
	return ok && time.Now().Before(e.expiry)
}

func (ss *sessionStore) userID(token string) int64 {
	ss.mu.Lock()
	e := ss.tokens[token]
	ss.mu.Unlock()
	return e.userID
}

func (ss *sessionStore) delete(token string) {
	ss.mu.Lock()
	delete(ss.tokens, token)
	ss.mu.Unlock()
}

// Server is the main HTTP handler.
type Server struct {
	cfg            Config
	store          storage.Store
	users          storage.UserStore
	broker         *Broker
	scanFn         ScanFunc
	scanning       atomic.Bool
	mux            *http.ServeMux
	sessions       *sessionStore
	oauthStates    sync.Map // map[string]time.Time — short-lived CSRF state tokens
	exchangeTokens sync.Map // map[string]time.Time — one-time tokens for OAuth handoff
}

// DeviceSeenEvent builds an sseEvent for a passively detected device.
func DeviceSeenEvent(ip, mac string, kind diff.ChangeKind) sseEvent {
	return sseEvent{Type: "device_seen", IP: ip, MAC: mac, Kind: kind}
}

// sessionToken extracts the session token from the request.
// It checks (in order): HttpOnly cookie, Authorization: Bearer header, ?token= query param.
// The query param is only used by EventSource (SSE) which cannot send custom headers.
func (s *Server) sessionToken(r *http.Request) string {
	if c, err := r.Cookie("ding_session"); err == nil {
		return c.Value
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

// requireAuth returns 401 if the request carries no valid session.
// Accepts cookie, Authorization: Bearer header, or ?token= query param (for SSE).
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tok := s.sessionToken(r); tok != "" && s.sessions.valid(tok) {
			next(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
}

// oauthConfig builds an *oauth2.Config for the named provider.
// If DING_BASE_URL is set it is used for the redirect URI (required behind Cloudflare Tunnel
// or any reverse proxy that terminates TLS before the Go server).
func (s *Server) oauthConfig(r *http.Request, provider string) *oauth2.Config {
	base := s.cfg.BaseURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	redirectURL := strings.TrimRight(base, "/") + "/api/auth/" + provider + "/callback"

	switch provider {
	case "google":
		return &oauth2.Config{
			ClientID:     s.cfg.Google.ClientID,
			ClientSecret: s.cfg.Google.ClientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email"},
			Endpoint:     google.Endpoint,
		}
	case "github":
		return &oauth2.Config{
			ClientID:     s.cfg.GitHub.ClientID,
			ClientSecret: s.cfg.GitHub.ClientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"user:email"},
			Endpoint:     github.Endpoint,
		}
	}
	return nil
}

// exchangeEntry bundles the expiry and owner for a one-time OAuth exchange token.
type exchangeEntry struct {
	expiry time.Time
	userID int64
}

// newExchangeToken creates a one-time token valid for 60 seconds.
// The OAuth callback redirects to /#exchange=TOKEN instead of setting a cookie directly,
// because Cloudflare Tunnel strips Set-Cookie headers from redirect responses.
// The React app then POSTs the token to /api/auth/exchange which sets the cookie via
// a regular fetch response that Cloudflare does not modify.
func (s *Server) newExchangeToken(userID int64) string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	token := hex.EncodeToString(b)
	s.exchangeTokens.Store(token, exchangeEntry{expiry: time.Now().Add(60 * time.Second), userID: userID})
	return token
}

// consumeExchangeToken validates and deletes an exchange token (one-time use).
// Returns the userID and true on success, or 0 and false if invalid/expired.
func (s *Server) consumeExchangeToken(token string) (int64, bool) {
	val, ok := s.exchangeTokens.LoadAndDelete(token)
	if !ok {
		return 0, false
	}
	e := val.(exchangeEntry)
	if time.Now().After(e.expiry) {
		return 0, false
	}
	return e.userID, true
}

// newOAuthState generates a random state token and stores it for 10 minutes.
func (s *Server) newOAuthState() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	state := hex.EncodeToString(b)
	s.oauthStates.Store(state, time.Now().Add(10*time.Minute))
	return state
}

// validOAuthState checks and consumes a state token.
func (s *Server) validOAuthState(state string) bool {
	val, ok := s.oauthStates.LoadAndDelete(state)
	if !ok {
		return false
	}
	return time.Now().Before(val.(time.Time))
}

// NewServer creates the server, registers all routes, and returns it.
func NewServer(cfg Config, store storage.Store, users storage.UserStore, broker *Broker, scanFn ScanFunc) *Server {
	s := &Server{
		cfg:      cfg,
		store:    store,
		users:    users,
		broker:   broker,
		scanFn:   scanFn,
		sessions: newSessionStore(),
	}
	s.mux = http.NewServeMux()

	// Auth endpoints — no session required
	s.mux.HandleFunc("GET /api/auth/providers", s.handleAuthProviders)
	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("POST /api/auth/exchange", s.handleExchange)

	// OAuth redirect + callback (only active when provider credentials are set)
	if cfg.Google.ClientID != "" {
		s.mux.HandleFunc("GET /api/auth/google", s.handleOAuthRedirect("google"))
		s.mux.HandleFunc("GET /api/auth/google/callback", s.handleOAuthCallback("google"))
	}
	if cfg.GitHub.ClientID != "" {
		s.mux.HandleFunc("GET /api/auth/github", s.handleOAuthRedirect("github"))
		s.mux.HandleFunc("GET /api/auth/github/callback", s.handleOAuthCallback("github"))
	}

	// Protected API routes — all require a valid session
	s.mux.HandleFunc("GET /api/status", s.requireAuth(s.handleStatus))
	s.mux.HandleFunc("GET /api/devices", s.requireAuth(s.handleDevices))
	s.mux.HandleFunc("GET /api/devices/{ip}/history", s.requireAuth(s.handleDeviceHistory))
	s.mux.HandleFunc("GET /api/history", s.requireAuth(s.handleHistory))
	s.mux.HandleFunc("GET /api/topology", s.requireAuth(s.handleTopology))
	s.mux.HandleFunc("POST /api/scan", s.requireAuth(s.handleScan))
	s.mux.HandleFunc("POST /api/devices/{ip}/scan", s.requireAuth(s.handleDeviceScan))
	s.mux.HandleFunc("POST /api/devices/{ip}/wake", s.requireAuth(s.handleWake))
	s.mux.HandleFunc("PUT /api/devices/{ip}/label", s.requireAuth(s.handleSetLabel))
	s.mux.HandleFunc("DELETE /api/devices/{ip}/label", s.requireAuth(s.handleDelLabel))
	s.mux.HandleFunc("PUT /api/devices/{ip}/notify", s.requireAuth(s.handleSetNotify))
	s.mux.HandleFunc("GET /api/settings/telegram", s.requireAuth(s.handleGetTelegram))
	s.mux.HandleFunc("PUT /api/settings/telegram", s.requireAuth(s.handleSaveTelegram))
	s.mux.HandleFunc("POST /api/settings/telegram/test", s.requireAuth(s.handleTestTelegram))
	s.mux.HandleFunc("GET /api/events", s.requireAuth(s.broker.serveSSE))

	// Static files — always served so the React app loads on the login page too
	sub, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("/", spaHandler(sub))

	return s
}

// ServeHTTP makes *Server satisfy http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// TriggerScan starts a scan in the background if one isn't already running.
func (s *Server) TriggerScan() {
	if !s.scanning.CompareAndSwap(false, true) {
		return
	}
	go s.runScan()
}

func (s *Server) runScan() {
	defer s.scanning.Store(false)
	s.broker.Publish(sseEvent{Type: "scan_start"})
	_, changes, err := s.scanFn()
	if err != nil {
		s.broker.Publish(sseEvent{Type: "scan_error", Error: err.Error()})
		return
	}
	s.broker.Publish(sseEvent{
		Type:      "scan_result",
		Devices:   s.store.AllDevices(),
		Changes:   changes,
		ScannedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		_, err := fsys.Open(path)
		if errors.Is(err, fs.ErrNotExist) {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
