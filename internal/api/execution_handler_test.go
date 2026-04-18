package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/execution"
	_ "modernc.org/sqlite"
)

// ── Fakes ─────────────────────────────────────────────────────────────────────

// fakeReleaser counts Release() calls so tests can assert terminal-status
// feedback frees the ActiveCount slot.
type fakeReleaser struct {
	released int
	active   int64
}

func (f *fakeReleaser) Release()            { f.released++ }
func (f *fakeReleaser) ActiveCount() int64  { return f.active }

// fakeFeedback implements FeedbackRecorder with an in-memory seen-set so we
// can test the 409 duplicate path without a real DB.
type fakeFeedback struct {
	seen map[string]bool
	err  error
}

func (f *fakeFeedback) RecordOnce(_ context.Context, pluginID, signalID, orderID, status, _, _ string, _ time.Time) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.seen == nil {
		f.seen = make(map[string]bool)
	}
	key := pluginID + "|" + signalID + "|" + orderID + "|" + status
	if f.seen[key] {
		return false, nil
	}
	f.seen[key] = true
	return true, nil
}

// newExecTestServer builds a Server wired with an ExecutionHolder loaded from
// a temp execution.yaml file, plus fake dispatcher/feedback stubs.
func newExecTestServer(t *testing.T, cfg appconfig.ExecutionConfig) (*Server, *fakeReleaser, *fakeFeedback, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")
	if err := appconfig.SaveExecutionConfig(path, cfg); err != nil {
		t.Fatalf("SaveExecutionConfig: %v", err)
	}
	holder := appconfig.NewExecutionHolder(path, cfg)
	releaser := &fakeReleaser{}
	fb := &fakeFeedback{}
	s := &Server{}
	s.WithExecutionHolder(holder, path)
	s.WithExecutionDispatcher(releaser)
	s.WithExecutionFeedback(fb)
	return s, releaser, fb, path
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// GET redacts secrets (A5): the wire response must never contain the real secret.
func TestGetExecutionConfig_RedactsSecrets(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://example.com/hook", Secret: "super-secret", Enabled: true},
		},
	}
	s, _, _, _ := newExecTestServer(t, cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/execution/config", nil)
	s.getExecutionConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("super-secret")) {
		t.Fatal("response leaked real secret")
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("***")) {
		t.Fatal("response missing redaction marker")
	}
}

// PUT preserves existing secrets when the incoming payload sends "" or "***".
func TestUpdateExecutionConfig_PreservesSecrets(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://example.com/hook", Secret: "old-secret", Enabled: true},
		},
	}
	s, _, _, path := newExecTestServer(t, cfg)

	// newExecTestServer has no execState, so readConfigVersion returns 1.
	type incomingWithVersion struct {
		appconfig.ExecutionConfig
		Version int `json:"version"`
	}
	payload := incomingWithVersion{
		ExecutionConfig: appconfig.ExecutionConfig{
			Enabled: true,
			Plugins: []appconfig.PluginConfig{
				// Empty secret → preserve old-secret.
				{ID: "p1", URL: "https://example.com/hook", Secret: "", Enabled: true},
			},
		},
		Version: 1,
	}
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/execution/config", bytes.NewReader(body))
	s.updateExecutionConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	// Reload from disk to verify the real secret is still "old-secret".
	loaded, err := appconfig.LoadExecutionConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(loaded.Plugins) != 1 || loaded.Plugins[0].Secret != "old-secret" {
		t.Fatalf("secret not preserved on disk; got %+v", loaded.Plugins)
	}
}

// PUT replaces the secret when a real value is provided.
func TestUpdateExecutionConfig_AcceptsNewSecret(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://example.com/hook", Secret: "old-secret", Enabled: true},
		},
	}
	s, _, _, path := newExecTestServer(t, cfg)

	// newExecTestServer has no execState, so readConfigVersion returns 1.
	type incomingWithVersion struct {
		appconfig.ExecutionConfig
		Version int `json:"version"`
	}
	payload := incomingWithVersion{
		ExecutionConfig: appconfig.ExecutionConfig{
			Enabled: true,
			Plugins: []appconfig.PluginConfig{
				{ID: "p1", URL: "https://example.com/hook", Secret: "rotated", Enabled: true},
			},
		},
		Version: 1,
	}
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/execution/config", bytes.NewReader(body))
	s.updateExecutionConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := appconfig.LoadExecutionConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Plugins[0].Secret != "rotated" {
		t.Fatalf("secret should be rotated; got %q", loaded.Plugins[0].Secret)
	}
}

// Kill switch persists to disk first, then flips memory (Codex #10).
func TestToggleExecutionKill_Persists(t *testing.T) {
	cfg := appconfig.ExecutionConfig{Enabled: true}
	s, _, _, path := newExecTestServer(t, cfg)

	body := []byte(`{"on":true}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/execution/kill", bytes.NewReader(body))
	s.toggleExecutionKill(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	// Verify disk state reflects the toggle.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	if !bytes.Contains(raw, []byte("kill_switch: true")) {
		t.Fatalf("yaml does not show kill_switch: true; got %s", raw)
	}
}

// Feedback rejects requests missing auth headers with 401.
func TestPostExecutionFeedback_MissingHeaders(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://x/y", Secret: "s", Enabled: true},
		},
	}
	s, _, _, _ := newExecTestServer(t, cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/execution/feedback", bytes.NewReader([]byte(`{}`)))
	s.postExecutionFeedback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

// Feedback rejects bad signature with 401.
func TestPostExecutionFeedback_BadSignature(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://x/y", Secret: "real-secret", Enabled: true},
		},
	}
	s, _, _, _ := newExecTestServer(t, cfg)

	body := []byte(`{"signal_id":"sig","status":"FILLED"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execution/feedback", bytes.NewReader(body))
	now := time.Now().Unix()
	// Sign with the wrong secret.
	sig := execution.Sign("wrong-secret", "p1", now, http.MethodPost, "/api/execution/feedback", body)
	req.Header.Set(execution.SignatureHeader, sig)
	req.Header.Set(execution.TimestampHeader, strconv.FormatInt(now, 10))
	req.Header.Set(execution.PluginIDHeader, "p1")
	rec := httptest.NewRecorder()
	s.postExecutionFeedback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

// Feedback happy path: valid HMAC, terminal status → Release() called.
func TestPostExecutionFeedback_Accept_TerminalCallsRelease(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://x/y", Secret: "secret", Enabled: true},
		},
	}
	s, releaser, fb, _ := newExecTestServer(t, cfg)

	body := []byte(`{"signal_id":"sig-1","order_id":"ord-1","status":"FILLED"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execution/feedback", bytes.NewReader(body))
	now := time.Now().Unix()
	sig := execution.Sign("secret", "p1", now, http.MethodPost, "/api/execution/feedback", body)
	req.Header.Set(execution.SignatureHeader, sig)
	req.Header.Set(execution.TimestampHeader, strconv.FormatInt(now, 10))
	req.Header.Set(execution.PluginIDHeader, "p1")
	rec := httptest.NewRecorder()
	s.postExecutionFeedback(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if releaser.released != 1 {
		t.Errorf("Release() called %d times, want 1", releaser.released)
	}
	if len(fb.seen) != 1 {
		t.Errorf("expected one idempotency record, got %d", len(fb.seen))
	}
}

// Feedback replay (Codex #4): second identical POST returns 409 and does NOT
// call Release() a second time.
func TestPostExecutionFeedback_DuplicateReturns409(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://x/y", Secret: "secret", Enabled: true},
		},
	}
	s, releaser, _, _ := newExecTestServer(t, cfg)

	body := []byte(`{"signal_id":"sig-1","order_id":"ord-1","status":"FILLED"}`)
	buildReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/execution/feedback", bytes.NewReader(body))
		now := time.Now().Unix()
		sig := execution.Sign("secret", "p1", now, http.MethodPost, "/api/execution/feedback", body)
		req.Header.Set(execution.SignatureHeader, sig)
		req.Header.Set(execution.TimestampHeader, strconv.FormatInt(now, 10))
		req.Header.Set(execution.PluginIDHeader, "p1")
		return req
	}

	rec := httptest.NewRecorder()
	s.postExecutionFeedback(rec, buildReq())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("first call status %d, want 202", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.postExecutionFeedback(rec, buildReq())
	if rec.Code != http.StatusConflict {
		t.Fatalf("replay status %d, want 409", rec.Code)
	}
	if releaser.released != 1 {
		t.Errorf("Release() should fire only on first accept; got %d", releaser.released)
	}
}

// Non-terminal status (ACK) is accepted but does NOT free ActiveCount.
func TestPostExecutionFeedback_NonTerminal_NoRelease(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://x/y", Secret: "secret", Enabled: true},
		},
	}
	s, releaser, _, _ := newExecTestServer(t, cfg)

	body := []byte(`{"signal_id":"sig-1","status":"RECEIVED"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execution/feedback", bytes.NewReader(body))
	now := time.Now().Unix()
	sig := execution.Sign("secret", "p1", now, http.MethodPost, "/api/execution/feedback", body)
	req.Header.Set(execution.SignatureHeader, sig)
	req.Header.Set(execution.TimestampHeader, strconv.FormatInt(now, 10))
	req.Header.Set(execution.PluginIDHeader, "p1")
	rec := httptest.NewRecorder()
	s.postExecutionFeedback(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202", rec.Code)
	}
	if releaser.released != 0 {
		t.Errorf("non-terminal must not call Release(); got %d", releaser.released)
	}
}

// Timestamp outside skew window → 401.
func TestPostExecutionFeedback_OutsideSkew(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled:          true,
		TimestampSkewSec: 60,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://x/y", Secret: "secret", Enabled: true},
		},
	}
	s, _, _, _ := newExecTestServer(t, cfg)

	body := []byte(`{"signal_id":"sig","status":"FILLED"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execution/feedback", bytes.NewReader(body))
	// 10 minutes in the past — way beyond the 60s window.
	old := time.Now().Unix() - 600
	sig := execution.Sign("secret", "p1", old, http.MethodPost, "/api/execution/feedback", body)
	req.Header.Set(execution.SignatureHeader, sig)
	req.Header.Set(execution.TimestampHeader, strconv.FormatInt(old, 10))
	req.Header.Set(execution.PluginIDHeader, "p1")
	rec := httptest.NewRecorder()
	s.postExecutionFeedback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
}

// newFeedbackTestDB opens an in-memory SQLite with the feedback_idempotency
// schema used by FeedbackIdempotency, so TestFeedback_PersistsSymbolAndMessage
// can query the row directly.
func newFeedbackTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := `
	CREATE TABLE feedback_idempotency (
		plugin_id    TEXT    NOT NULL,
		signal_id    TEXT    NOT NULL,
		order_id     TEXT    NOT NULL DEFAULT '',
		status       TEXT    NOT NULL,
		received_at  INTEGER NOT NULL,
		symbol       TEXT    NOT NULL DEFAULT '',
		message      TEXT    NOT NULL DEFAULT '',
		UNIQUE(plugin_id, signal_id, order_id, status)
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// TestFeedback_PersistsSymbolAndMessage verifies that the handler parses
// symbol and message from the feedback JSON and forwards them to RecordOnce,
// which persists them in the feedback_idempotency table (Task 4).
func TestFeedback_PersistsSymbolAndMessage(t *testing.T) {
	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "alpaca-paper", URL: "https://x/y", Secret: "secret", Enabled: true},
		},
	}
	db := newFeedbackTestDB(t)
	idem := execution.NewFeedbackIdempotency(db)

	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")
	if err := appconfig.SaveExecutionConfig(path, cfg); err != nil {
		t.Fatalf("SaveExecutionConfig: %v", err)
	}
	holder := appconfig.NewExecutionHolder(path, cfg)
	releaser := &fakeReleaser{}

	s := &Server{}
	s.WithExecutionHolder(holder, path)
	s.WithExecutionDispatcher(releaser)
	s.WithExecutionFeedback(idem)

	body := []byte(`{"signal_id":"sig-99","plugin_name":"alpaca-paper","status":"FILLED","symbol":"tsla","message":"OK"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execution/feedback", bytes.NewReader(body))
	now := time.Now().Unix()
	sig := execution.Sign("secret", "alpaca-paper", now, http.MethodPost, "/api/execution/feedback", body)
	req.Header.Set(execution.SignatureHeader, sig)
	req.Header.Set(execution.TimestampHeader, strconv.FormatInt(now, 10))
	req.Header.Set(execution.PluginIDHeader, "alpaca-paper")

	rec := httptest.NewRecorder()
	s.postExecutionFeedback(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status: %d; body=%s", rec.Code, rec.Body.String())
	}

	var sym, msg string
	row := db.QueryRow(`SELECT symbol, message FROM feedback_idempotency WHERE signal_id = 'sig-99'`)
	if err := row.Scan(&sym, &msg); err != nil {
		t.Fatalf("row: %v", err)
	}
	// symbol must be uppercased by the handler; message preserved verbatim.
	if sym != "TSLA" || msg != "OK" {
		t.Fatalf("got (%q, %q), want (TSLA, OK)", sym, msg)
	}
}

// ── ListExecutionFeedback test helpers ────────────────────────────────────────

const testAPIToken = "test-bearer-token"

// executionHandlerTestServer wraps a Server and a real SQLite DB so that
// TestListFeedback_* tests can seed rows and make authenticated requests.
type executionHandlerTestServer struct {
	srv *Server
	db  *sql.DB
	ts  *httptest.Server
}

// newExecutionHandlerTestServer creates a test server with a real SQLite DB
// (the feedback_idempotency schema + execution_state schema), a wired execDB,
// execState, and an API token.
func newExecutionHandlerTestServer(t *testing.T) (*executionHandlerTestServer, func()) {
	t.Helper()
	db := newFeedbackTestDB(t)

	// Add the execution_state table (Task 2 schema).
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS execution_state (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0
		);`); err != nil {
		t.Fatalf("execution_state schema: %v", err)
	}

	cfg := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "alpaca-paper", URL: "https://x/y", Secret: "secret", Enabled: true},
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")
	if err := appconfig.SaveExecutionConfig(path, cfg); err != nil {
		t.Fatalf("SaveExecutionConfig: %v", err)
	}
	holder := appconfig.NewExecutionHolder(path, cfg)

	execState := execution.NewStateStore(db)

	s := &Server{}
	s.WithExecutionHolder(holder, path)
	s.WithExecutionDB(db)
	s.WithAPIToken(testAPIToken)
	s.WithExecutionState(execState)

	ts := httptest.NewServer(s.Handler())
	cleanup := func() { ts.Close() }
	return &executionHandlerTestServer{srv: s, db: db, ts: ts}, cleanup
}

// execConfigResponse is the shape returned by GET /api/execution/config for
// version-related assertions. Other fields are ignored.
type execConfigResponse struct {
	Version  int    `json:"version"`
	KilledAt string `json:"killed_at"`
}

// getConfig sends a GET to /api/execution/config and decodes the version fields.
func (h *executionHandlerTestServer) getConfig(t *testing.T) execConfigResponse {
	t.Helper()
	resp := h.get(t, "/api/execution/config")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getConfig: status %d", resp.StatusCode)
	}
	var cfg execConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("getConfig decode: %v", err)
	}
	return cfg
}

// put sends an authenticated PUT to the test server at the given path with body.
func (h *executionHandlerTestServer) put(t *testing.T, path string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, h.ts.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	return resp
}

// seedFeedback inserts a row directly into feedback_idempotency.
func (h *executionHandlerTestServer) seedFeedback(t *testing.T, pluginID, signalID, orderID, status, symbol, message string) {
	t.Helper()
	_, err := h.db.Exec(
		`INSERT INTO feedback_idempotency(plugin_id, signal_id, order_id, status, received_at, symbol, message) VALUES(?,?,?,?,?,?,?)`,
		pluginID, signalID, orderID, status, time.Now().Unix(), symbol, message,
	)
	if err != nil {
		t.Fatalf("seedFeedback: %v", err)
	}
}

// get sends an authenticated GET to the test server at the given path.
func (h *executionHandlerTestServer) get(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.ts.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// getNoAuth sends an unauthenticated GET to the test server at the given path.
func (h *executionHandlerTestServer) getNoAuth(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.ts.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// ── ListExecutionFeedback tests ───────────────────────────────────────────────

func TestListFeedback_NoFilterReturnsRecent(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	srv.seedFeedback(t, "alpaca-paper", "sig-1", "ord-1", "FILLED", "AAPL", "ok")
	srv.seedFeedback(t, "alpaca-paper", "sig-2", "ord-2", "REJECTED", "TSLA", "bad")

	resp := srv.get(t, "/api/execution/feedback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out struct {
		Items []struct{ SignalID, Status, Symbol, Message string } `json:"items"`
		Count int                                                  `json:"count"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Count != 2 {
		t.Fatalf("count: %d", out.Count)
	}
}

func TestListFeedback_FilterByStatus(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	srv.seedFeedback(t, "alpaca-paper", "s1", "o1", "FILLED", "AAPL", "")
	srv.seedFeedback(t, "alpaca-paper", "s2", "o2", "REJECTED", "TSLA", "")

	resp := srv.get(t, "/api/execution/feedback?status=FILLED")
	var out struct{ Count int `json:"count"` }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Count != 1 {
		t.Fatalf("count: %d", out.Count)
	}
}

func TestListFeedback_SymbolFilterUppercase(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	srv.seedFeedback(t, "alpaca-paper", "s1", "o1", "FILLED", "AAPL", "")

	resp := srv.get(t, "/api/execution/feedback?symbol=aapl")
	var out struct{ Count int `json:"count"` }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Count != 1 {
		t.Fatalf("lowercase query should match uppercase storage, got %d", out.Count)
	}
}

func TestListFeedback_LimitBounds(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	cases := []struct {
		q          string
		wantStatus int
	}{
		{"?limit=0", http.StatusOK},        // treat as default
		{"?limit=-1", http.StatusBadRequest},
		{"?limit=501", http.StatusBadRequest},
		{"?limit=500", http.StatusOK},
	}
	for _, c := range cases {
		resp := srv.get(t, "/api/execution/feedback"+c.q)
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s: got %d want %d", c.q, resp.StatusCode, c.wantStatus)
		}
	}
}

func TestListFeedback_Unauthorized(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	resp := srv.getNoAuth(t, "/api/execution/feedback")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

// seedFeedbackAt inserts a row with an explicit timestamp into feedback_idempotency.
func (h *executionHandlerTestServer) seedFeedbackAt(t *testing.T, pluginID, signalID, orderID, status, symbol, message string, at time.Time) {
	t.Helper()
	_, err := h.db.Exec(
		`INSERT INTO feedback_idempotency(plugin_id, signal_id, order_id, status, received_at, symbol, message) VALUES(?,?,?,?,?,?,?)`,
		pluginID, signalID, orderID, status, at.Unix(), symbol, message,
	)
	if err != nil {
		t.Fatalf("seedFeedbackAt: %v", err)
	}
}

// ── PluginStats tests ─────────────────────────────────────────────────────────

func TestPluginStats_Aggregates24hCounts(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	now := time.Now()
	failAt := now.Add(-3 * time.Hour)
	srv.seedFeedbackAt(t, "alpaca", "s1", "o1", "FILLED", "AAPL", "", now.Add(-1*time.Hour))
	srv.seedFeedbackAt(t, "alpaca", "s2", "o2", "FILLED", "AAPL", "", now.Add(-2*time.Hour))
	srv.seedFeedbackAt(t, "alpaca", "s3", "o3", "REJECTED", "TSLA", "denied", failAt)
	srv.seedFeedbackAt(t, "alpaca", "s4", "o4", "FILLED", "AAPL", "", now.Add(-25*time.Hour)) // outside window

	resp := srv.get(t, "/api/execution/plugins/stats?window=24h")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "max-age=60" {
		t.Errorf("Cache-Control = %q, want max-age=60", cc)
	}

	// Capture raw body for JSON key checks later.
	rawBody, _ := io.ReadAll(resp.Body)

	var out struct {
		Plugins []struct {
			PluginID       string `json:"plugin_id"`
			Submitted      int    `json:"submitted"`
			Filled         int    `json:"filled"`
			Rejected       int    `json:"rejected"`
			LastFailureAt  *int64 `json:"last_failure_at,omitempty"`
			LastFailureMsg string `json:"last_failure_msg"`
		} `json:"plugins"`
	}
	_ = json.Unmarshal(rawBody, &out)
	if len(out.Plugins) != 1 {
		t.Fatalf("plugins: %d", len(out.Plugins))
	}
	p := out.Plugins[0]
	if p.Filled != 2 || p.Rejected != 1 || p.LastFailureMsg != "denied" {
		t.Fatalf("aggregation wrong: %+v", p)
	}

	// LastFailureAt must be non-nil (has a REJECTED row) and within ±1s of failAt.
	if p.LastFailureAt == nil {
		t.Fatal("LastFailureAt should be non-nil for plugin with REJECTED rows")
	}
	diff := *p.LastFailureAt - failAt.Unix()
	if diff < -1 || diff > 1 {
		t.Fatalf("LastFailureAt = %d, want ~%d (±1s)", *p.LastFailureAt, failAt.Unix())
	}
	// Raw JSON must contain the key.
	if !bytes.Contains(rawBody, []byte(`"last_failure_at"`)) {
		t.Fatal("JSON missing last_failure_at key for plugin with failures")
	}
}

// TestPluginStats_NoFailuresOmitsLastFailureAt verifies that when a plugin has
// no REJECTED or ERROR rows, last_failure_at is nil and absent from the JSON.
func TestPluginStats_NoFailuresOmitsLastFailureAt(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	now := time.Now()
	srv.seedFeedbackAt(t, "alpaca", "s1", "o1", "FILLED", "AAPL", "", now.Add(-1*time.Hour))
	srv.seedFeedbackAt(t, "alpaca", "s2", "o2", "FILLED", "TSLA", "", now.Add(-2*time.Hour))

	resp := srv.get(t, "/api/execution/plugins/stats?window=24h")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	rawBody, _ := io.ReadAll(resp.Body)

	var out struct {
		Plugins []struct {
			PluginID      string `json:"plugin_id"`
			Filled        int    `json:"filled"`
			LastFailureAt *int64 `json:"last_failure_at,omitempty"`
		} `json:"plugins"`
	}
	_ = json.Unmarshal(rawBody, &out)
	if len(out.Plugins) != 1 {
		t.Fatalf("plugins: %d", len(out.Plugins))
	}
	p := out.Plugins[0]
	if p.LastFailureAt != nil {
		t.Fatalf("LastFailureAt should be nil for plugin with no failures; got %d", *p.LastFailureAt)
	}
	// The key must be absent from the raw JSON (omitempty).
	if bytes.Contains(rawBody, []byte(`"last_failure_at"`)) {
		t.Fatal("JSON must not contain last_failure_at key when there are no failures")
	}
}

func TestPluginStats_ZeroActivityPluginOmitted(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	resp := srv.get(t, "/api/execution/plugins/stats?window=24h")
	var out struct{ Plugins []any `json:"plugins"` }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Plugins) != 0 {
		t.Fatalf("expected empty, got %d", len(out.Plugins))
	}
}

func TestPluginStats_Unauthorized(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	resp := srv.getNoAuth(t, "/api/execution/plugins/stats?window=24h")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

// ── Config version tests ──────────────────────────────────────────────────────

func TestUpdateConfig_VersionMatchBumps(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	cfg := srv.getConfig(t)
	if cfg.Version < 1 {
		t.Fatalf("initial version: %d", cfg.Version)
	}

	body, _ := json.Marshal(map[string]any{
		"version":        cfg.Version,
		"enabled":        true,
		"max_dispatched": 5,
	})
	resp := srv.put(t, "/api/execution/config", body)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d; body=%s", resp.StatusCode, b)
	}

	cfg2 := srv.getConfig(t)
	if cfg2.Version != cfg.Version+1 {
		t.Fatalf("version did not bump: %d → %d", cfg.Version, cfg2.Version)
	}
}

func TestUpdateConfig_VersionMismatchReturns409(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	cfg := srv.getConfig(t)

	body, _ := json.Marshal(map[string]any{
		"version": cfg.Version + 99,
		"enabled": true,
	})
	resp := srv.put(t, "/api/execution/config", body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var body409 struct {
		Error          string `json:"error"`
		CurrentVersion int    `json:"current_version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body409)
	if body409.Error != "version_conflict" || body409.CurrentVersion != cfg.Version {
		t.Fatalf("bad 409 body: %+v", body409)
	}
}

func TestUpdateConfig_MissingVersionRejected(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	body, _ := json.Marshal(map[string]any{"enabled": true})
	resp := srv.put(t, "/api/execution/config", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

// TestUpdateConfig_ConcurrentWritesRaceFree verifies that the TOCTOU window in
// the version-check/save/bump sequence is closed by the mutex. Ten goroutines
// all claim the same initial version; exactly one must succeed (200) and the
// remaining nine must be rejected with 409 (version_conflict). After all
// goroutines finish the persisted version must be exactly initialVersion+1.
func TestUpdateConfig_ConcurrentWritesRaceFree(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	cfg := srv.getConfig(t)

	// Fire N concurrent PUTs all claiming the same initial version.
	const N = 10
	var wg sync.WaitGroup
	statuses := make(chan int, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{
				"version":        cfg.Version,
				"enabled":        true,
				"max_dispatched": 3,
			})
			resp := srv.put(t, "/api/execution/config", body)
			_ = resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(statuses)

	var ok, conflict int
	for s := range statuses {
		switch s {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Errorf("unexpected status: %d", s)
		}
	}
	if ok != 1 || conflict != N-1 {
		t.Fatalf("ok=%d conflict=%d (want 1/%d)", ok, conflict, N-1)
	}

	final := srv.getConfig(t)
	if final.Version != cfg.Version+1 {
		t.Fatalf("version after %d concurrent PUTs: %d → %d (want +1)", N, cfg.Version, final.Version)
	}
}

// isTerminalFeedbackStatus coverage.
func TestIsTerminalFeedbackStatus(t *testing.T) {
	for _, s := range []string{"FILLED", "filled", " REJECTED ", "CANCELLED", "CANCELED", "ERROR"} {
		if !isTerminalFeedbackStatus(s) {
			t.Errorf("%q should be terminal", s)
		}
	}
	for _, s := range []string{"RECEIVED", "SUBMITTED", "PARTIAL_FILL", ""} {
		if isTerminalFeedbackStatus(s) {
			t.Errorf("%q should NOT be terminal", s)
		}
	}
}
