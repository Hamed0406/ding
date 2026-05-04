package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockRoundTripper intercepts all HTTP calls and delegates to an http.Handler,
// allowing tests to replace oauthHTTPClient without changing hardcoded provider URLs.
type mockRoundTripper struct {
	handler http.Handler
}

func (m *mockRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	rr := httptest.NewRecorder()
	m.handler.ServeHTTP(rr, r)
	return rr.Result(), nil
}

// errorTransport always returns a network error from RoundTrip.
type errorTransport struct{}

func (e *errorTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated network error")
}

func withMockOAuthClient(t *testing.T, handler http.Handler) {
	t.Helper()
	orig := oauthHTTPClient
	oauthHTTPClient = &http.Client{Transport: &mockRoundTripper{handler: handler}}
	t.Cleanup(func() { oauthHTTPClient = orig })
}

func withErrorOAuthClient(t *testing.T) {
	t.Helper()
	orig := oauthHTTPClient
	oauthHTTPClient = &http.Client{Transport: &errorTransport{}}
	t.Cleanup(func() { oauthHTTPClient = orig })
}

// ── fetchOAuthUser branches ──────────────────────────────────────────────────

func TestFetchOAuthUser_UnknownProvider(t *testing.T) {
	email, id, err := fetchOAuthUser("unknown-provider", "any-token")
	if email != "" || id != "" || err != nil {
		t.Errorf("unknown provider: want ('','',nil), got (%q,%q,%v)", email, id, err)
	}
}

func TestFetchOAuthUser_Google(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"sub": "g-1", "email": "g@example.com"}) //nolint:errcheck
	}))
	email, id, err := fetchOAuthUser("google", "fake-token")
	if err != nil {
		t.Fatalf("fetchOAuthUser(google): %v", err)
	}
	if email != "g@example.com" || id != "g-1" {
		t.Errorf("got email=%q id=%q", email, id)
	}
}

func TestFetchOAuthUser_GitHub(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 99, "email": "gh@example.com"}) //nolint:errcheck
	}))
	email, id, err := fetchOAuthUser("github", "fake-token")
	if err != nil {
		t.Fatalf("fetchOAuthUser(github): %v", err)
	}
	if email != "gh@example.com" || id != "99" {
		t.Errorf("got email=%q id=%q", email, id)
	}
}

// ── fetchGoogleUser ──────────────────────────────────────────────────────────

func TestFetchGoogleUser_Success(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"sub": "g-123", "email": "guser@example.com"}) //nolint:errcheck
	}))
	email, id, err := fetchGoogleUser("fake-token")
	if err != nil {
		t.Fatalf("fetchGoogleUser: %v", err)
	}
	if email != "guser@example.com" {
		t.Errorf("email: want guser@example.com, got %q", email)
	}
	if id != "g-123" {
		t.Errorf("id: want g-123, got %q", id)
	}
}

func TestFetchGoogleUser_BadJSON(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "not-json") //nolint:errcheck
	}))
	_, _, err := fetchGoogleUser("fake-token")
	if err == nil {
		t.Error("want error for malformed JSON response")
	}
}

func TestFetchGoogleUser_NetworkError(t *testing.T) {
	withErrorOAuthClient(t)
	_, _, err := fetchGoogleUser("token")
	if err == nil {
		t.Error("want error on network failure")
	}
}

// ── fetchGitHubUser ──────────────────────────────────────────────────────────

func TestFetchGitHubUser_PublicEmail(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/user") {
			json.NewEncoder(w).Encode(map[string]any{"id": 42, "email": "pub@example.com"}) //nolint:errcheck
		}
	}))
	email, id, err := fetchGitHubUser("fake-token")
	if err != nil {
		t.Fatalf("fetchGitHubUser (public email): %v", err)
	}
	if email != "pub@example.com" || id != "42" {
		t.Errorf("got email=%q id=%q", email, id)
	}
}

func TestFetchGitHubUser_PrivatePrimaryEmail(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/user/emails"):
			json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
				{"email": "priv@example.com", "primary": true},
				{"email": "other@example.com", "primary": false},
			})
		default:
			json.NewEncoder(w).Encode(map[string]any{"id": 99, "email": ""}) //nolint:errcheck
		}
	}))
	email, id, err := fetchGitHubUser("fake-token")
	if err != nil {
		t.Fatalf("fetchGitHubUser (private email): %v", err)
	}
	if email != "priv@example.com" || id != "99" {
		t.Errorf("got email=%q id=%q", email, id)
	}
}

func TestFetchGitHubUser_FallbackFirstEmail(t *testing.T) {
	// No primary — should fall back to emails[0].
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/user/emails"):
			json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
				{"email": "fallback@example.com", "primary": false},
			})
		default:
			json.NewEncoder(w).Encode(map[string]any{"id": 7, "email": ""}) //nolint:errcheck
		}
	}))
	email, _, err := fetchGitHubUser("fake-token")
	if err != nil {
		t.Fatalf("fetchGitHubUser (fallback email): %v", err)
	}
	if email != "fallback@example.com" {
		t.Errorf("email: want fallback@example.com, got %q", email)
	}
}

func TestFetchGitHubUser_EmptyEmailsList(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/user/emails"):
			json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
		default:
			json.NewEncoder(w).Encode(map[string]any{"id": 5, "email": ""}) //nolint:errcheck
		}
	}))
	email, id, err := fetchGitHubUser("fake-token")
	if err != nil {
		t.Fatalf("fetchGitHubUser (empty emails): %v", err)
	}
	if email != "" || id != "5" {
		t.Errorf("got email=%q id=%q", email, id)
	}
}

func TestFetchGitHubUser_BadJSON(t *testing.T) {
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "not-json") //nolint:errcheck
	}))
	_, _, err := fetchGitHubUser("fake-token")
	if err == nil {
		t.Error("want error for malformed /user JSON")
	}
}

func TestFetchGitHubUser_NetworkError(t *testing.T) {
	withErrorOAuthClient(t)
	_, _, err := fetchGitHubUser("token")
	if err == nil {
		t.Error("want error on network failure")
	}
}

func TestFetchGitHubUser_EmailsBadJSON(t *testing.T) {
	call := 0
	withMockOAuthClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			// First call: /user with no public email
			json.NewEncoder(w).Encode(map[string]any{"id": 3, "email": ""}) //nolint:errcheck
		} else {
			// Second call: /user/emails with bad JSON
			io.WriteString(w, "bad-json") //nolint:errcheck
		}
	}))
	_, _, err := fetchGitHubUser("token")
	if err == nil {
		t.Error("want error for malformed /user/emails JSON")
	}
}
