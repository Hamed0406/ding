package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ding/ding/internal/api"
	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

// noRedirectClient does not follow HTTP redirects — used to inspect OAuth redirect responses.
var noRedirectClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// newOAuthEnv creates a test environment with both GitHub and Google OAuth configured.
func newOAuthEnv(t *testing.T) *testEnv {
	t.Helper()
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	broker := api.NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, nil }
	srv := api.NewServer(
		api.Config{
			Iface:  "eth0",
			Subnet: "192.168.1.0/24",
			GitHub: api.OAuthConfig{ClientID: "test-client", ClientSecret: "test-secret"},
			Google: api.OAuthConfig{ClientID: "google-client", ClientSecret: "google-secret"},
		},
		store, store, broker, scanFn,
	)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &testEnv{ts: ts, store: store, srv: srv}
}

// ── OAuth redirect ────────────────────────────────────────────────────────────

func TestOAuthRedirect_GitHub(t *testing.T) {
	e := newOAuthEnv(t)
	resp, err := noRedirectClient.Get(e.ts.URL + "/api/auth/github")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "github.com") {
		t.Errorf("redirect should point to github.com, got %q", loc)
	}
	u, _ := url.Parse(loc)
	if u.Query().Get("state") == "" {
		t.Error("redirect URL missing state parameter")
	}
}

func TestOAuthRedirect_Google(t *testing.T) {
	e := newOAuthEnv(t)
	resp, err := noRedirectClient.Get(e.ts.URL + "/api/auth/google")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(strings.ToLower(loc), "google") {
		t.Errorf("redirect should point to Google, got %q", loc)
	}
}

// ── OAuth callback error paths ────────────────────────────────────────────────

func TestOAuthCallback_ErrorParam(t *testing.T) {
	e := newOAuthEnv(t)
	resp, err := noRedirectClient.Get(e.ts.URL + "/api/auth/github/callback?error=access_denied")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "auth_error") || !strings.Contains(loc, "access_denied") {
		t.Errorf("redirect should contain auth_error=access_denied, got %q", loc)
	}
}

func TestOAuthCallback_InvalidState(t *testing.T) {
	e := newOAuthEnv(t)
	resp, err := noRedirectClient.Get(e.ts.URL + "/api/auth/github/callback?state=invalid-state-xyz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "auth_error") {
		t.Errorf("invalid state: redirect should contain auth_error, got %q", loc)
	}
}

// ── oauthConfig BaseURL and X-Forwarded-Proto ─────────────────────────────────

func TestOAuthConfig_BaseURL(t *testing.T) {
	store, _ := storage.NewSQLite(":memory:")
	broker := api.NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, nil }
	srv := api.NewServer(api.Config{
		Iface:   "eth0",
		Subnet:  "192.168.1.0/24",
		BaseURL: "https://ding.example.com",
		GitHub:  api.OAuthConfig{ClientID: "test", ClientSecret: "secret"},
	}, store, store, broker, scanFn)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := noRedirectClient.Get(ts.URL + "/api/auth/github")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, url.QueryEscape("https://ding.example.com")) {
		t.Errorf("redirect_uri should use configured BaseURL, got %q", loc)
	}
}

func TestOAuthConfig_XForwardedProto(t *testing.T) {
	store, _ := storage.NewSQLite(":memory:")
	broker := api.NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, nil }
	srv := api.NewServer(api.Config{
		Iface:  "eth0",
		Subnet: "192.168.1.0/24",
		GitHub: api.OAuthConfig{ClientID: "test", ClientSecret: "secret"},
	}, store, store, broker, scanFn)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/auth/github", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	// redirect_uri must use https:// when X-Forwarded-Proto is https
	if !strings.Contains(loc, url.QueryEscape("https://")) {
		t.Errorf("redirect_uri should use https when X-Forwarded-Proto: https; got %q", loc)
	}
}

// ── runScan error path ────────────────────────────────────────────────────────

type errScan struct{ msg string }

func (e *errScan) Error() string { return e.msg }

func TestRunScan_Error(t *testing.T) {
	store, _ := storage.NewSQLite(":memory:")
	broker := api.NewBroker()
	scanErr := &errScan{"simulated scan failure"}
	scanFn := func() ([]scanner.Result, []diff.Change, error) { return nil, nil, scanErr }
	srv := api.NewServer(
		api.Config{Iface: "eth0", Subnet: "192.168.1.0/24"},
		store, store, broker, scanFn,
	)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	regResp, _ := http.Post(ts.URL+"/api/auth/register", "application/json",
		strings.NewReader(`{"email":"scan@example.com","password":"password123"}`))
	var reg map[string]any
	json.NewDecoder(regResp.Body).Decode(&reg) //nolint:errcheck
	regResp.Body.Close()
	tok, _ := reg["token"].(string)

	req, _ := http.NewRequest("POST", ts.URL+"/api/scan", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("want 202, got %d", resp.StatusCode)
	}
	// Give the scan goroutine time to finish and publish the scan_error event.
	time.Sleep(100 * time.Millisecond)
}

// ── sessionToken query param ──────────────────────────────────────────────────

func TestSessionToken_QueryParam(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// EventSource (SSE) clients cannot set headers, so they pass the token as ?token=.
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/status?token="+tok, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("query-param token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query-param token: want 200, got %d", resp.StatusCode)
	}
}

// ── StartCleanup goroutines ───────────────────────────────────────────────────

func TestStartCleanup_ContextCancel(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	e.srv.StartCleanup(ctx)
	// Canceling the context lets all cleanup goroutines exit cleanly.
	cancel()
	time.Sleep(10 * time.Millisecond)
}

// ── sessionToken cookie ───────────────────────────────────────────────────────

func TestSessionToken_Cookie(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// The session token returned in the body equals the cookie value — use it directly.
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "ding_session", Value: tok})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("cookie auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cookie auth: want 200, got %d", resp.StatusCode)
	}
}

// ── SPA fallback ──────────────────────────────────────────────────────────────

func TestSPAFallback_UnknownPath(t *testing.T) {
	e := newEnv(t)
	// A path not in the static directory triggers the SPA handler to fall back to index.html.
	resp, err := http.Get(e.ts.URL + "/devices/192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("SPA fallback: want 200 (index.html), got %d", resp.StatusCode)
	}
}
