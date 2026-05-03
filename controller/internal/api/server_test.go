package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding/ding/internal/api"
	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

// testEnv holds a live test server and its in-memory store.
type testEnv struct {
	ts    *httptest.Server
	store *storage.SQLiteStore
	srv   *api.Server
}

// newEnv spins up a real HTTP server backed by in-memory SQLite.
func newEnv(t *testing.T) *testEnv {
	t.Helper()
	store, err := storage.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	broker := api.NewBroker()
	scanFn := func() ([]scanner.Result, []diff.Change, error) {
		return nil, nil, nil
	}
	srv := api.NewServer(
		api.Config{Iface: "eth0", Subnet: "192.168.1.0/24"},
		store, store, broker, scanFn,
	)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &testEnv{ts: ts, store: store, srv: srv}
}

// register creates a new user and returns the bearer token.
func (e *testEnv) register(t *testing.T) string {
	t.Helper()
	body := `{"email":"test@example.com","password":"password123"}`
	resp, err := http.Post(e.ts.URL+"/api/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("register POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("register: want 201, got %d: %s", resp.StatusCode, data)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	tok, _ := result["token"].(string)
	if tok == "" {
		t.Fatal("register: no token in response")
	}
	return tok
}

// login authenticates an existing user and returns the bearer token.
func (e *testEnv) login(t *testing.T) string {
	t.Helper()
	body := `{"email":"test@example.com","password":"password123"}`
	resp, err := http.Post(e.ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("login POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("login: want 200, got %d: %s", resp.StatusCode, data)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	tok, _ := result["token"].(string)
	if tok == "" {
		t.Fatal("login: no token in response")
	}
	return tok
}

func (e *testEnv) authGet(t *testing.T, token, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", e.ts.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (e *testEnv) authPost(t *testing.T, token, path, body string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, _ := http.NewRequest("POST", e.ts.URL+path, bodyReader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func (e *testEnv) authPut(t *testing.T, token, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("PUT", e.ts.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	return resp
}

func (e *testEnv) authDelete(t *testing.T, token, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("DELETE", e.ts.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

// ── Auth tests ────────────────────────────────────────────────────────────────

func TestProviders_NoUsers(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.ts.URL + "/api/auth/providers")
	if err != nil {
		t.Fatalf("GET providers: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["has_users"] != false {
		t.Errorf("has_users: want false, got %v", result["has_users"])
	}
}

func TestRegister_Success(t *testing.T) {
	e := newEnv(t)
	body := `{"email":"newuser@example.com","password":"securepass"}`
	resp, err := http.Post(e.ts.URL+"/api/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 201, got %d: %s", resp.StatusCode, data)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["token"]; !ok {
		t.Error("response should contain token")
	}
}

func TestRegister_Duplicate(t *testing.T) {
	e := newEnv(t)
	e.register(t) // first registration
	// second registration with same email
	body := `{"email":"test@example.com","password":"password123"}`
	resp, err := http.Post(e.ts.URL+"/api/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("want 409, got %d", resp.StatusCode)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	e := newEnv(t)
	body := `{"email":"user@example.com","password":"short"}`
	resp, err := http.Post(e.ts.URL+"/api/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestRegister_MissingFields(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Post(e.ts.URL+"/api/auth/register", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestLogin_Success(t *testing.T) {
	e := newEnv(t)
	e.register(t)
	tok := e.login(t)
	if tok == "" {
		t.Error("expected non-empty token on login")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	e := newEnv(t)
	e.register(t)
	body := `{"email":"test@example.com","password":"wrongpassword"}`
	resp, err := http.Post(e.ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestLogout(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	// Logout
	resp := e.authPost(t, tok, "/api/auth/logout", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: want 204, got %d", resp.StatusCode)
	}

	// Token should now be invalid
	resp2 := e.authGet(t, tok, "/api/status")
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout GET /api/status: want 401, got %d", resp2.StatusCode)
	}
}

// ── Unauthorized tests ────────────────────────────────────────────────────────

func TestUnauthorized(t *testing.T) {
	e := newEnv(t)
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/status"},
		{"GET", "/api/devices"},
		{"GET", "/api/history"},
		{"GET", "/api/topology"},
		{"POST", "/api/scan"},
		{"GET", "/api/events"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, e.ts.URL+ep.path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("want 401, got %d", resp.StatusCode)
			}
		})
	}
}

// ── Status & Devices ──────────────────────────────────────────────────────────

func TestStatus(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/status")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["iface"]; !ok {
		t.Error("response missing iface field")
	}
	if _, ok := result["subnet"]; !ok {
		t.Error("response missing subnet field")
	}
	if result["iface"] != "eth0" {
		t.Errorf("iface: want eth0, got %v", result["iface"])
	}
	if result["subnet"] != "192.168.1.0/24" {
		t.Errorf("subnet: want 192.168.1.0/24, got %v", result["subnet"])
	}
}

func TestDevices_Empty(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/devices")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var devices []any
	json.NewDecoder(resp.Body).Decode(&devices)
	if len(devices) != 0 {
		t.Errorf("want empty array, got %d items", len(devices))
	}
}

func TestDevices_AfterSave(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	results := []scanner.Result{
		{IP: "192.168.1.1", Alive: true, OpenPorts: []uint16{22, 80}},
		{IP: "192.168.1.2", Alive: true, OpenPorts: []uint16{443}},
	}
	if err := e.store.Save(results); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	resp := e.authGet(t, tok, "/api/devices")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var devices []any
	json.NewDecoder(resp.Body).Decode(&devices)
	if len(devices) != 2 {
		t.Errorf("want 2 devices, got %d", len(devices))
	}
}

// ── Labels ────────────────────────────────────────────────────────────────────

func TestSetLabel(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPut(t, tok, "/api/devices/192.168.1.1/label", `{"name":"Router"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
}

func TestSetLabel_Empty(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPut(t, tok, "/api/devices/192.168.1.1/label", `{"name":""}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestDeleteLabel(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// Set first
	e.authPut(t, tok, "/api/devices/192.168.1.1/label", `{"name":"Router"}`).Body.Close()
	// Then delete
	resp := e.authDelete(t, tok, "/api/devices/192.168.1.1/label")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
}

// ── Notify ────────────────────────────────────────────────────────────────────

func TestSetNotify(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPut(t, tok, "/api/devices/192.168.1.1/notify", `{"enabled":true}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
}

// ── Scan ──────────────────────────────────────────────────────────────────────

func TestTriggerScan(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPost(t, tok, "/api/scan", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("want 202, got %d", resp.StatusCode)
	}
}

func TestTriggerScan_Conflict(t *testing.T) {
	// Use a slow scanFn that blocks until we release it.
	store, _ := storage.NewSQLite(":memory:")
	broker := api.NewBroker()

	// ready signals that the slow scan has started; release unblocks it.
	ready := make(chan struct{})
	release := make(chan struct{})
	slowScanFn := func() ([]scanner.Result, []diff.Change, error) {
		close(ready)
		<-release
		return nil, nil, nil
	}

	srv := api.NewServer(
		api.Config{Iface: "eth0", Subnet: "192.168.1.0/24"},
		store, store, broker, slowScanFn,
	)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	defer close(release)

	// Register a user.
	regBody := `{"email":"test@example.com","password":"password123"}`
	regResp, _ := http.Post(ts.URL+"/api/auth/register", "application/json", strings.NewReader(regBody))
	var regResult map[string]any
	json.NewDecoder(regResp.Body).Decode(&regResult)
	regResp.Body.Close()
	tok, _ := regResult["token"].(string)

	authPost := func(path string) *http.Response {
		req, _ := http.NewRequest("POST", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := http.DefaultClient.Do(req)
		return resp
	}

	// First scan — should be accepted.
	resp1 := authPost("/api/scan")
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusAccepted {
		t.Fatalf("first scan: want 202, got %d", resp1.StatusCode)
	}

	// Wait until the slow scan has actually started.
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("slow scan never started")
	}

	// Second scan while first is running — should be 409.
	resp2 := authPost("/api/scan")
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("second scan: want 409, got %d", resp2.StatusCode)
	}
}

// ── History ───────────────────────────────────────────────────────────────────

func TestHistory_Empty(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/history")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var records []any
	json.NewDecoder(resp.Body).Decode(&records)
	if len(records) != 0 {
		t.Errorf("want empty array, got %d items", len(records))
	}
}

func TestDeviceHistory_Empty(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/devices/192.168.1.1/history")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var entries []any
	json.NewDecoder(resp.Body).Decode(&entries)
	if len(entries) != 0 {
		t.Errorf("want empty array, got %d items", len(entries))
	}
}

// ── Topology ──────────────────────────────────────────────────────────────────

func TestTopology(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/topology")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["nodes"]; !ok {
		t.Error("topology response missing nodes field")
	}
	if _, ok := result["edges"]; !ok {
		t.Error("topology response missing edges field")
	}
}

// ── Telegram settings ─────────────────────────────────────────────────────────

func TestGetTelegram_Default(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/settings/telegram")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["token_set"] != false {
		t.Errorf("token_set: want false, got %v", result["token_set"])
	}
}

func TestSaveTelegram(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	// Save config
	resp := e.authPut(t, tok, "/api/settings/telegram", `{"token":"tok123","chat_id":"456"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT telegram: want 204, got %d", resp.StatusCode)
	}

	// Verify via GET
	resp2 := e.authGet(t, tok, "/api/settings/telegram")
	defer resp2.Body.Close()
	var result map[string]any
	json.NewDecoder(resp2.Body).Decode(&result)
	if result["token_set"] != true {
		t.Errorf("token_set: want true, got %v", result["token_set"])
	}
}

func TestSaveTelegram_PreservesToken(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	// Save initial config with a token
	resp := e.authPut(t, tok, "/api/settings/telegram", `{"token":"mytoken","chat_id":"111"}`)
	resp.Body.Close()

	// Update with empty token but new chat_id — should preserve token
	resp2 := e.authPut(t, tok, "/api/settings/telegram", `{"token":"","chat_id":"222"}`)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT telegram (preserve): want 204, got %d", resp2.StatusCode)
	}

	// Verify token is still set
	resp3 := e.authGet(t, tok, "/api/settings/telegram")
	defer resp3.Body.Close()
	var result map[string]any
	json.NewDecoder(resp3.Body).Decode(&result)
	if result["token_set"] != true {
		t.Error("token should still be set after update with empty token")
	}
}

// ── Email settings ────────────────────────────────────────────────────────────

func TestGetEmail_Default(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/settings/email")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["host"] != "" {
		t.Errorf("host: want empty string, got %v", result["host"])
	}
	// Port defaults to 587
	port, _ := result["port"].(float64)
	if port != 587 {
		t.Errorf("port: want 587, got %v", port)
	}
}

func TestSaveEmail(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	cfg := `{"host":"smtp.example.com","port":587,"username":"user","password":"pass","from":"from@example.com","to":"to@example.com"}`
	resp := e.authPut(t, tok, "/api/settings/email", cfg)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT email: want 204, got %d", resp.StatusCode)
	}

	resp2 := e.authGet(t, tok, "/api/settings/email")
	defer resp2.Body.Close()
	var result map[string]any
	json.NewDecoder(resp2.Body).Decode(&result)
	if result["host"] != "smtp.example.com" {
		t.Errorf("host: want smtp.example.com, got %v", result["host"])
	}
	if result["to"] != "to@example.com" {
		t.Errorf("to: want to@example.com, got %v", result["to"])
	}
}

func TestSaveEmail_EmptyPasswordPreserves(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	// Save with a password
	cfg := `{"host":"smtp.example.com","port":587,"username":"user","password":"secret","from":"from@example.com","to":"to@example.com"}`
	e.authPut(t, tok, "/api/settings/email", cfg).Body.Close()

	// Update with empty password — should keep existing
	cfg2 := `{"host":"smtp.example.com","port":587,"username":"user","password":"","from":"from@example.com","to":"to@example.com"}`
	resp := e.authPut(t, tok, "/api/settings/email", cfg2)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT email (preserve password): want 204, got %d", resp.StatusCode)
	}

	// Verify password is still set (password_set should be true)
	resp2 := e.authGet(t, tok, "/api/settings/email")
	defer resp2.Body.Close()
	var result map[string]any
	json.NewDecoder(resp2.Body).Decode(&result)
	if result["password_set"] != true {
		t.Error("password should still be set after update with empty password")
	}
}

// ── Speedtest history ─────────────────────────────────────────────────────────

func TestSpeedtestHistory_Empty(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/speedtest/history")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var results []any
	json.NewDecoder(resp.Body).Decode(&results)
	if len(results) != 0 {
		t.Errorf("want empty array, got %d items", len(results))
	}
}

func TestSpeedtestHistory_AfterSave(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	err := e.store.SaveSpeedtest(storage.SpeedtestResult{
		TestedAt:     time.Now(),
		DownloadMbps: 100.5,
		UploadMbps:   50.2,
		PingMs:       12.3,
		Server:       "cloudflare",
	})
	if err != nil {
		t.Fatalf("SaveSpeedtest: %v", err)
	}

	resp := e.authGet(t, tok, "/api/speedtest/history")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var results []any
	json.NewDecoder(resp.Body).Decode(&results)
	if len(results) != 1 {
		t.Errorf("want 1 result, got %d", len(results))
	}
}

// ── Security headers ──────────────────────────────────────────────────────────

func TestSecurityHeaders(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()

	cases := []struct{ header, want string }{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "same-origin"},
	}
	for _, c := range cases {
		if got := resp.Header.Get(c.header); got != c.want {
			t.Errorf("%s: want %q, got %q", c.header, c.want, got)
		}
	}
}

// ── IP validation ─────────────────────────────────────────────────────────────

func TestInvalidIP_DeviceScan(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// Only values that actually route to the handler — path traversal strings
	// are cleaned by the HTTP mux before they reach any handler.
	for _, bad := range []string{"not-an-ip", "999.999.999.999", "hostname"} {
		path := "/api/devices/" + bad + "/scan"
		resp := e.authPost(t, tok, path, "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("POST %s with bad IP %q: want 400, got %d", path, bad, resp.StatusCode)
		}
	}
}

func TestInvalidIP_Label(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPut(t, tok, "/api/devices/not-an-ip/label", `{"name":"test"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid IP in label path, got %d", resp.StatusCode)
	}
}

func TestInvalidIP_History(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/devices/not-an-ip/history")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid IP in history path, got %d", resp.StatusCode)
	}
}

// ── Webhook URL validation ────────────────────────────────────────────────────

func TestSaveWebhook_ValidURL(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	for _, good := range []string{"http://example.com/hook", "https://hooks.example.com/path"} {
		body := fmt.Sprintf(`{"url":%q}`, good)
		resp := e.authPut(t, tok, "/api/settings/webhook", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("PUT webhook with valid URL %q: want 200, got %d", good, resp.StatusCode)
		}
	}
}

func TestSaveWebhook_InvalidScheme(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// Validation blocks any scheme that isn't http or https.
	// http://169.254.169.254 is a valid http URL and is not blocked — LAN webhook
	// targets are legitimate for home users (e.g. Home Assistant).
	for _, bad := range []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"javascript:alert(1)",
	} {
		body := fmt.Sprintf(`{"url":%q}`, bad)
		resp := e.authPut(t, tok, "/api/settings/webhook", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("PUT webhook with bad URL %q: want 400, got %d", bad, resp.StatusCode)
		}
	}
}

func TestSaveWebhook_ClearURL(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// Empty string is allowed (means "clear the webhook")
	resp := e.authPut(t, tok, "/api/settings/webhook", `{"url":""}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT webhook with empty URL: want 200, got %d", resp.StatusCode)
	}
}

// ── Body size limit ───────────────────────────────────────────────────────────

func TestBodySizeLimit(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	// Build a payload just over the 1 MB limit.
	large := bytes.Repeat([]byte("x"), (1<<20)+1)
	req, _ := http.NewRequest("PUT", e.ts.URL+"/api/settings/webhook", bytes.NewReader(large))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: want 400 or 413, got %d", resp.StatusCode)
	}
}

// ── Exchange token ────────────────────────────────────────────────────────────

func TestExchange_InvalidToken(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Post(e.ts.URL+"/api/auth/exchange", "application/json",
		strings.NewReader(`{"token":"not-a-real-token"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestExchange_MissingToken(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Post(e.ts.URL+"/api/auth/exchange", "application/json",
		strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

// ── Export ────────────────────────────────────────────────────────────────────

func TestExport_CSV(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	mac := "aa:bb:cc:dd:ee:ff"
	host := "router"
	e.store.Save([]scanner.Result{{IP: "192.168.1.1", Alive: true, MAC: &mac, Hostname: &host}}) //nolint:errcheck

	resp := e.authGet(t, tok, "/api/devices/export")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("want text/csv, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "192.168.1.1") {
		t.Error("CSV should contain device IP")
	}
}

func TestExport_JSON(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/devices/export?format=json")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("want application/json, got %q", ct)
	}
}

// ── Changes & ARPWatch ────────────────────────────────────────────────────────

func TestChanges_Empty(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/changes")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result []any
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("want empty array, got %d items", len(result))
	}
}

func TestARPWatch_Empty(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/arpwatch")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result []any
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("want empty array, got %d items", len(result))
	}
}

// ── Device scan ───────────────────────────────────────────────────────────────

func TestDeviceScan_ReturnsOpenPorts(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// Scanning a valid IP returns 200 with open_ports field (may be empty).
	resp := e.authPost(t, tok, "/api/devices/127.0.0.1/scan", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["open_ports"]; !ok {
		t.Error("response missing open_ports field")
	}
	if result["ip"] != "127.0.0.1" {
		t.Errorf("ip: want 127.0.0.1, got %v", result["ip"])
	}
}

// ── Wake-on-LAN ───────────────────────────────────────────────────────────────

func TestWake_UnknownDevice(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	// Device not in store → no MAC → 422
	resp := e.authPost(t, tok, "/api/devices/192.168.1.99/wake", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}
}

func TestWake_KnownDevice(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)

	mac := "aa:bb:cc:dd:ee:01"
	e.store.Save([]scanner.Result{{IP: "192.168.1.50", Alive: true, MAC: &mac}}) //nolint:errcheck

	resp := e.authPost(t, tok, "/api/devices/192.168.1.50/wake", "")
	defer resp.Body.Close()
	// Should succeed (UDP broadcast) or fail with 500 if broadcast is blocked in CI;
	// either way, it must not return 422 (MAC is known).
	if resp.StatusCode == http.StatusUnprocessableEntity {
		t.Fatal("wake: 422 unexpected — MAC is known")
	}
}

// ── Webhook GET ───────────────────────────────────────────────────────────────

func TestGetWebhook_Default(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/settings/webhook")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["url_set"] != false {
		t.Errorf("url_set: want false, got %v", result["url_set"])
	}
}

// ── Test-send endpoints (no-config error path) ────────────────────────────────

func TestTestWebhook_NoConfig(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPost(t, tok, "/api/settings/webhook/test", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 when no webhook configured, got %d", resp.StatusCode)
	}
}

func TestTestTelegram_NoConfig(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPost(t, tok, "/api/settings/telegram/test", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 when no Telegram configured, got %d", resp.StatusCode)
	}
}

func TestTestEmail_NoConfig(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPost(t, tok, "/api/settings/email/test", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 when no email configured, got %d", resp.StatusCode)
	}
}

// ── Status with a scan record ─────────────────────────────────────────────────

func TestStatus_WithLastScan(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	e.store.Save(nil) //nolint:errcheck

	resp := e.authGet(t, tok, "/api/status")
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["last_scan"] == nil {
		t.Error("last_scan should be set after a scan")
	}
}

// ── Webhook GET after save ────────────────────────────────────────────────────

func TestGetWebhook_AfterSave(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	e.authPut(t, tok, "/api/settings/webhook", `{"url":"https://example.com/hook"}`).Body.Close()

	resp := e.authGet(t, tok, "/api/settings/webhook")
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["url_set"] != true {
		t.Errorf("url_set: want true after save, got %v", result["url_set"])
	}
}

// ── Changes with limit param ──────────────────────────────────────────────────

func TestChanges_LimitParam(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authGet(t, tok, "/api/changes?limit=10")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ── Register bad JSON ─────────────────────────────────────────────────────────

func TestRegister_BadJSON(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Post(e.ts.URL+"/api/auth/register", "application/json",
		strings.NewReader("not-json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

// ── Label too long ────────────────────────────────────────────────────────────

func TestSetLabel_TooLong(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	name := strings.Repeat("x", 256)
	resp := e.authPut(t, tok, "/api/devices/192.168.1.1/label",
		fmt.Sprintf(`{"name":%q}`, name))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for label > 255 chars, got %d", resp.StatusCode)
	}
}

// ── SetNotify bad JSON ────────────────────────────────────────────────────────

func TestSetNotify_BadJSON(t *testing.T) {
	e := newEnv(t)
	tok := e.register(t)
	resp := e.authPut(t, tok, "/api/devices/192.168.1.1/notify", "not-json")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for bad JSON, got %d", resp.StatusCode)
	}
}

// ── DeviceSeenEvent ───────────────────────────────────────────────────────────

func TestDeviceSeenEvent(t *testing.T) {
	ev := api.DeviceSeenEvent("192.168.1.1", "aa:bb:cc:dd:ee:ff", diff.KindNew)
	if ev.IP != "192.168.1.1" {
		t.Errorf("IP: want 192.168.1.1, got %q", ev.IP)
	}
	if ev.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC: want aa:bb:cc:dd:ee:ff, got %q", ev.MAC)
	}
}

// ── TriggerScan ───────────────────────────────────────────────────────────────

func TestTriggerScan_DoesNotPanic(t *testing.T) {
	e := newEnv(t)
	// TriggerScan starts a background scan; calling it twice is safe (second is a no-op).
	e.srv.TriggerScan()
	e.srv.TriggerScan()
	// Wait for the fast no-op scan to finish.
	time.Sleep(50 * time.Millisecond)
}

// compile-time assertion: these imports are used
var (
	_ = bytes.NewReader
	_ = fmt.Sprintf
	_ sync.Mutex
)
