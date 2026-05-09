package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

// ── exchange tokens ──────────────────────────────────────────────────────────

func TestExchangeToken_RoundTrip(t *testing.T) {
	s := &Server{}
	tok := s.newExchangeToken(42)
	if tok == "" {
		t.Fatal("newExchangeToken returned empty string")
	}
	userID, ok := s.consumeExchangeToken(tok)
	if !ok {
		t.Fatal("consumeExchangeToken: want ok=true")
	}
	if userID != 42 {
		t.Errorf("userID: want 42, got %d", userID)
	}
}

func TestExchangeToken_OneTimeUse(t *testing.T) {
	s := &Server{}
	tok := s.newExchangeToken(1)
	s.consumeExchangeToken(tok)
	_, ok := s.consumeExchangeToken(tok)
	if ok {
		t.Error("exchange token should only be valid once")
	}
}

func TestExchangeToken_Expired(t *testing.T) {
	s := &Server{}
	s.exchangeTokens.Store("expired-tok", exchangeEntry{
		expiry: time.Now().Add(-time.Second),
		userID: 99,
	})
	_, ok := s.consumeExchangeToken("expired-tok")
	if ok {
		t.Error("expired exchange token should not be valid")
	}
}

func TestExchangeToken_Missing(t *testing.T) {
	s := &Server{}
	_, ok := s.consumeExchangeToken("not-a-real-token")
	if ok {
		t.Error("unknown exchange token should not be valid")
	}
}

// ── OAuth state tokens ───────────────────────────────────────────────────────

func TestOAuthState_Expired(t *testing.T) {
	s := &Server{}
	s.oauthStates.Store("expired-state", time.Now().Add(-time.Second))
	if s.validOAuthState("expired-state") {
		t.Error("expired OAuth state should not be valid")
	}
}

func TestOAuthState_Missing(t *testing.T) {
	s := &Server{}
	if s.validOAuthState("nonexistent-state") {
		t.Error("missing OAuth state should not be valid")
	}
}

// ── sessionStore ─────────────────────────────────────────────────────────────

func TestSessionStore_UserID(t *testing.T) {
	ss := newSessionStore()
	tok := ss.create(77)
	if id := ss.userID(tok); id != 77 {
		t.Errorf("userID: want 77, got %d", id)
	}
}

func TestSessionStore_Delete(t *testing.T) {
	ss := newSessionStore()
	tok := ss.create(1)
	ss.delete(tok)
	if ss.valid(tok) {
		t.Error("deleted token should be invalid")
	}
}

func TestSessionStore_ExpiredToken(t *testing.T) {
	ss := newSessionStore()
	ss.mu.Lock()
	ss.tokens["expired"] = sessionEntry{expiry: time.Now().Add(-time.Second), userID: 1}
	ss.mu.Unlock()
	if ss.valid("expired") {
		t.Error("expired token should not be valid")
	}
}

// ── OAuth callback error paths ────────────────────────────────────────────────

// oauthErrTransport always returns a network error — used to make cfg.Exchange() fail.
type oauthErrTransport struct{}

func (o *oauthErrTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated token exchange failure")
}

func TestHandleOAuthCallback_ExchangeError(t *testing.T) {
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	broker := NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, nil }
	s := NewServer(Config{
		Iface:  "eth0",
		Subnet: "192.168.1.0/24",
		GitHub: OAuthConfig{ClientID: "test-client", ClientSecret: "test-secret"},
	}, store, store, broker, scanFn)

	// Generate a valid state token so the handler passes the CSRF check.
	state := s.newOAuthState()

	// Inject a failing HTTP client into the request context so cfg.Exchange() fails.
	failClient := &http.Client{Transport: &oauthErrTransport{}}
	r := httptest.NewRequest("GET", "/api/auth/github/callback?state="+state+"&code=bad-code", nil)
	r = r.WithContext(context.WithValue(r.Context(), oauth2.HTTPClient, failClient))
	w := httptest.NewRecorder()

	s.handleOAuthCallback("github")(w, r)

	result := w.Result()
	if result.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", result.StatusCode)
	}
	loc := result.Header.Get("Location")
	if !strings.Contains(loc, "auth_error") {
		t.Errorf("exchange error: want auth_error in redirect, got %q", loc)
	}
}

func TestOAuthConfig_UnknownProvider(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "/", nil)
	cfg := s.oauthConfig(r, "unknown-provider")
	if cfg != nil {
		t.Error("unknown provider should return nil config")
	}
}

// TestHandleOAuthCallback_HappyPath exercises the full OAuth callback success flow by
// injecting a mock HTTP client via oauth2.HTTPClient context key for the token exchange,
// and replacing oauthHTTPClient for the user-info fetch.
func TestHandleOAuthCallback_HappyPath(t *testing.T) {
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	broker := NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, nil }
	s := NewServer(Config{
		Iface:  "eth0",
		Subnet: "192.168.1.0/24",
		GitHub: OAuthConfig{ClientID: "test-client", ClientSecret: "test-secret"},
	}, store, store, broker, scanFn)

	state := s.newOAuthState()

	// Mock client used by oauth2.Config.Exchange() to exchange the auth code for a token.
	// golang.org/x/oauth2 picks up this client from the context via oauth2.HTTPClient.
	tokenClient := &http.Client{Transport: &mockRoundTripper{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"access_token": "fake-gh-token",
				"token_type":   "bearer",
				"expires_in":   3600,
			})
		}),
	}}

	// Mock oauthHTTPClient so fetchGitHubUser returns a real-looking user.
	origOAuthClient := oauthHTTPClient
	oauthHTTPClient = &http.Client{Transport: &mockRoundTripper{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"id": 12345, "email": "cbuser@example.com"}) //nolint:errcheck
		}),
	}}
	t.Cleanup(func() { oauthHTTPClient = origOAuthClient })

	r := httptest.NewRequest("GET", "/api/auth/github/callback?state="+state+"&code=valid-code", nil)
	r = r.WithContext(context.WithValue(r.Context(), oauth2.HTTPClient, tokenClient))
	w := httptest.NewRecorder()

	s.handleOAuthCallback("github")(w, r)

	result := w.Result()
	if result.StatusCode != http.StatusFound {
		t.Fatalf("happy path: want 302, got %d (body: %s)", result.StatusCode, w.Body.String())
	}
	loc := result.Header.Get("Location")
	if !strings.Contains(loc, "#exchange=") {
		t.Errorf("happy path: want /#exchange= in redirect, got %q", loc)
	}
}

// TestHandleOAuthCallback_ExistingUser exercises the branch where the user already
// exists in the DB (FindUserByProvider returns the user directly).
func TestHandleOAuthCallback_ExistingUser(t *testing.T) {
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	// Pre-create a user and link it to GitHub.
	user, err := store.CreateUser("existing@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := store.LinkProvider(user.ID, "github", "gh-999"); err != nil {
		t.Fatalf("LinkProvider: %v", err)
	}

	broker := NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, nil }
	s := NewServer(Config{
		Iface:  "eth0",
		Subnet: "192.168.1.0/24",
		GitHub: OAuthConfig{ClientID: "test-client", ClientSecret: "test-secret"},
	}, store, store, broker, scanFn)

	state := s.newOAuthState()

	tokenClient := &http.Client{Transport: &mockRoundTripper{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"access_token": "fake-gh-token",
				"token_type":   "bearer",
			})
		}),
	}}

	origOAuthClient := oauthHTTPClient
	// Return the provider ID that matches the linked user (gh-999).
	oauthHTTPClient = &http.Client{Transport: &mockRoundTripper{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"id": 999, "email": "existing@example.com"}) //nolint:errcheck
		}),
	}}
	t.Cleanup(func() { oauthHTTPClient = origOAuthClient })

	r := httptest.NewRequest("GET", "/api/auth/github/callback?state="+state+"&code=valid-code", nil)
	r = r.WithContext(context.WithValue(r.Context(), oauth2.HTTPClient, tokenClient))
	w := httptest.NewRecorder()

	s.handleOAuthCallback("github")(w, r)

	result := w.Result()
	if result.StatusCode != http.StatusFound {
		t.Fatalf("existing user: want 302, got %d", result.StatusCode)
	}
	if loc := result.Header.Get("Location"); !strings.Contains(loc, "#exchange=") {
		t.Errorf("existing user: want /#exchange=, got %q", loc)
	}
}

// TestHandleOAuthCallback_EmailLookup exercises the branch where FindUserByProvider
// returns nil but FindUserByEmail finds a matching local account.
func TestHandleOAuthCallback_EmailLookup(t *testing.T) {
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	// Pre-create a local user with no OAuth provider linked.
	if _, err := store.CreateUser("local@example.com", "hash"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	broker := NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, nil }
	s := NewServer(Config{
		Iface:  "eth0",
		Subnet: "192.168.1.0/24",
		GitHub: OAuthConfig{ClientID: "test-client", ClientSecret: "test-secret"},
	}, store, store, broker, scanFn)

	state := s.newOAuthState()

	tokenClient := &http.Client{Transport: &mockRoundTripper{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"access_token": "fake-gh-token",
				"token_type":   "bearer",
			})
		}),
	}}

	origOAuthClient := oauthHTTPClient
	// Return a provider ID that has no existing link, but an email matching a local user.
	oauthHTTPClient = &http.Client{Transport: &mockRoundTripper{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"id": 77777, "email": "local@example.com"}) //nolint:errcheck
		}),
	}}
	t.Cleanup(func() { oauthHTTPClient = origOAuthClient })

	r := httptest.NewRequest("GET", "/api/auth/github/callback?state="+state+"&code=valid-code", nil)
	r = r.WithContext(context.WithValue(r.Context(), oauth2.HTTPClient, tokenClient))
	w := httptest.NewRecorder()

	s.handleOAuthCallback("github")(w, r)

	result := w.Result()
	if result.StatusCode != http.StatusFound {
		t.Fatalf("email lookup: want 302, got %d", result.StatusCode)
	}
	if loc := result.Header.Get("Location"); !strings.Contains(loc, "#exchange=") {
		t.Errorf("email lookup: want /#exchange=, got %q", loc)
	}
}

// ── tcpScanOne: open port path ────────────────────────────────────────────────

func TestTcpScanOne_OpenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)

	open := tcpScanOne("127.0.0.1", []uint16{port, port + 1}, 500*time.Millisecond)
	if len(open) != 1 || open[0] != port {
		t.Errorf("want [%d], got %v", port, open)
	}
}

func TestTcpScanOne_NoPorts(t *testing.T) {
	open := tcpScanOne("127.0.0.1", nil, 100*time.Millisecond)
	if len(open) != 0 {
		t.Errorf("want empty, got %v", open)
	}
}

// ── parsePorts ─────────────────────────────────────────────────────────────────

func TestParsePorts_InvalidEntry(t *testing.T) {
	// Invalid entries are skipped; only valid ports returned.
	ports := parsePorts("22,notaport,80,99999")
	if len(ports) != 2 {
		t.Errorf("want 2 valid ports, got %v", ports)
	}
}

// ── sendMagicPacket: invalid MAC ──────────────────────────────────────────────

func TestSendMagicPacket_InvalidMAC(t *testing.T) {
	err := sendMagicPacket("not-a-mac")
	if err == nil {
		t.Error("want error for invalid MAC address")
	}
}

// ── responseRecorder ──────────────────────────────────────────────────────────

func TestResponseRecorder_InitialStatusIsZero(t *testing.T) {
	rr := &responseRecorder{ResponseWriter: httptest.NewRecorder()}
	if rr.status != 0 {
		t.Errorf("want initial status 0, got %d", rr.status)
	}
}

func TestResponseRecorder_ExplicitStatus(t *testing.T) {
	rr := &responseRecorder{ResponseWriter: httptest.NewRecorder()}
	rr.WriteHeader(http.StatusNotFound)
	if rr.status != http.StatusNotFound {
		t.Errorf("want %d, got %d", http.StatusNotFound, rr.status)
	}
}

func TestResponseRecorder_ImplicitOKOnWrite(t *testing.T) {
	rr := &responseRecorder{ResponseWriter: httptest.NewRecorder()}
	rr.Write([]byte("hello")) //nolint:errcheck
	if rr.status != http.StatusOK {
		t.Errorf("Write without WriteHeader should set status 200, got %d", rr.status)
	}
}

func TestResponseRecorder_WriteHeaderNotOverwritten(t *testing.T) {
	rr := &responseRecorder{ResponseWriter: httptest.NewRecorder()}
	rr.WriteHeader(http.StatusCreated)
	rr.Write([]byte("body")) //nolint:errcheck
	if rr.status != http.StatusCreated {
		t.Errorf("Write should not overwrite explicit WriteHeader; want 201, got %d", rr.status)
	}
}

// ── accessLog middleware ──────────────────────────────────────────────────────

// captureLog replaces the default slog handler with one writing to a buffer,
// and restores the original on test cleanup.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &buf
}

func TestAccessLog_LogsMethodAndPath(t *testing.T) {
	buf := captureLog(t)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/devices", nil))

	out := buf.String()
	if !strings.Contains(out, "GET") {
		t.Errorf("log missing method GET: %q", out)
	}
	if !strings.Contains(out, "/api/devices") {
		t.Errorf("log missing path /api/devices: %q", out)
	}
}

func TestAccessLog_LogsExplicitStatus(t *testing.T) {
	buf := captureLog(t)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/scan", nil))

	if !strings.Contains(buf.String(), "201") {
		t.Errorf("log missing status 201: %q", buf.String())
	}
}

func TestAccessLog_DefaultsTo200(t *testing.T) {
	buf := captureLog(t)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/status", nil))

	if !strings.Contains(buf.String(), "200") {
		t.Errorf("implicit 200 not logged: %q", buf.String())
	}
}

func TestAccessLog_LogsRemoteAddr(t *testing.T) {
	buf := captureLog(t)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:12345"
	h.ServeHTTP(httptest.NewRecorder(), r)

	if !strings.Contains(buf.String(), "10.0.0.5:12345") {
		t.Errorf("log missing remote addr: %q", buf.String())
	}
}

func TestAccessLog_LogsLatencyField(t *testing.T) {
	buf := captureLog(t)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	if !strings.Contains(buf.String(), "ms=") {
		t.Errorf("log missing ms latency field: %q", buf.String())
	}
}

// ── speedtesting conflict path ────────────────────────────────────────────────

func TestHandleSpeedtest_Conflict(t *testing.T) {
	s := &Server{sessions: newSessionStore()}
	// Simulate a speed test already in progress.
	s.speedtesting.Store(true)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/speedtest", nil)
	s.handleSpeedtest(w, r)

	if w.Code != 409 {
		t.Errorf("conflict path: want 409, got %d", w.Code)
	}
}
