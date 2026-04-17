package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

	incoming := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			// Empty secret → preserve old-secret.
			{ID: "p1", URL: "https://example.com/hook", Secret: "", Enabled: true},
		},
	}
	body, _ := json.Marshal(incoming)
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

	incoming := appconfig.ExecutionConfig{
		Enabled: true,
		Plugins: []appconfig.PluginConfig{
			{ID: "p1", URL: "https://example.com/hook", Secret: "rotated", Enabled: true},
		},
	}
	body, _ := json.Marshal(incoming)
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
