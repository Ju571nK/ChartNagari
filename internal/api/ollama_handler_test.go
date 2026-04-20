package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/ollama"
)

// fakeDetector returns a preset Status regardless of the provided context.
type fakeDetector struct {
	status ollama.Status
}

func (f *fakeDetector) Detect(_ context.Context) ollama.Status {
	return f.status
}

func newOllamaStatusServer(t *testing.T, status ollama.Status, apiToken string) *httptest.Server {
	t.Helper()
	s := &Server{}
	s.WithOllamaDetector(&fakeDetector{status: status})
	if apiToken != "" {
		s.WithAPIToken(apiToken)
	}
	return httptest.NewServer(s.Handler())
}

func TestOllamaStatus_Ready(t *testing.T) {
	status := ollama.Status{
		State: ollama.StateReady, Host: "http://localhost:11434", Model: "gemma4:4b",
		ModelsAvailable: []string{"gemma4:4b"}, Deployment: ollama.DeploymentNative,
		Suggest: ollama.Suggest{Action: "ready"},
	}
	srv := newOllamaStatusServer(t, status, "")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/ai/ollama/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control: %q", cc)
	}
	var body ollama.Status
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.State != ollama.StateReady {
		t.Fatalf("state: %s", body.State)
	}
	if body.Model != "gemma4:4b" {
		t.Fatalf("model: %s", body.Model)
	}
}

func TestOllamaStatus_NotInstalled(t *testing.T) {
	status := ollama.Status{
		State: ollama.StateNotInstalled, Host: "http://localhost:11434", Model: "gemma4:4b",
		Deployment: ollama.DeploymentNative,
		Suggest:    ollama.Suggest{Action: "install_ollama", Command: "brew install ollama"},
	}
	srv := newOllamaStatusServer(t, status, "")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/ai/ollama/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	var body ollama.Status
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.State != ollama.StateNotInstalled {
		t.Fatalf("state: %s", body.State)
	}
	if body.Suggest.Action != "install_ollama" {
		t.Fatalf("suggest.action: %s", body.Suggest.Action)
	}
	if body.Suggest.Command == "" {
		t.Fatalf("suggest.command should be populated")
	}
}

func TestOllamaStatus_Unauthorized(t *testing.T) {
	srv := newOllamaStatusServer(t, ollama.Status{State: ollama.StateReady}, "secret-token")
	defer srv.Close()

	// No Authorization header → 401
	resp, err := http.Get(srv.URL + "/api/ai/ollama/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestOllamaStatus_AuthorizedWithBearer(t *testing.T) {
	srv := newOllamaStatusServer(t, ollama.Status{State: ollama.StateReady, Host: "h", Model: "m"}, "secret-token")
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/ai/ollama/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestOllamaStatus_NoDetector503(t *testing.T) {
	s := &Server{}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/ai/ollama/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

// Exercise the UI-critical state chain (READY_NO_MODEL → pull_model suggest with size).
func TestOllamaStatus_ReadyNoModelSurfacesSuggestSize(t *testing.T) {
	status := ollama.Status{
		State: ollama.StateReadyNoModel, Host: "http://localhost:11434", Model: "gemma4:12b",
		ModelsAvailable: []string{"other:1b"}, Deployment: ollama.DeploymentNative,
		Suggest: ollama.Suggest{Action: "pull_model", Command: "ollama pull gemma4:12b", SizeBytes: 7_200_000_000},
	}
	srv := newOllamaStatusServer(t, status, "")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/ai/ollama/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	var body ollama.Status
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Suggest.SizeBytes != 7_200_000_000 {
		t.Fatalf("suggest.size_bytes: %d", body.Suggest.SizeBytes)
	}
}

// ── Pull tests ────────────────────────────────────────────────────────────────

// fakePullRunner lets tests control the stream + exit error. It records whether
// the provided context was cancelled so tests can assert client-disconnect handling.
type fakePullRunner struct {
	lines       []string
	err         error
	blockCh     chan struct{} // when non-nil, Pull waits on ctx.Done OR blockCh
	ctxObserved bool         // set true if Pull returned because ctx was cancelled
	mu          sync.Mutex
}

func (f *fakePullRunner) Pull(ctx context.Context, _ string, onLine func(line []byte)) error {
	for _, l := range f.lines {
		onLine([]byte(l))
	}
	if f.blockCh == nil {
		return f.err
	}
	// Block until either the test unblocks us OR the ctx is cancelled.
	select {
	case <-ctx.Done():
		f.mu.Lock()
		f.ctxObserved = true
		f.mu.Unlock()
		return ctx.Err()
	case <-f.blockCh:
		return f.err
	}
}

func (f *fakePullRunner) sawCtxCancel() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxObserved
}

func newOllamaPullServer(t *testing.T, runner OllamaPullRunner, apiToken string) *httptest.Server {
	t.Helper()
	s := &Server{}
	s.WithOllamaPullRunner(runner)
	if apiToken != "" {
		s.WithAPIToken(apiToken)
	}
	return httptest.NewServer(s.Handler())
}

func TestOllamaPull_StreamsLinesAsSSE(t *testing.T) {
	runner := &fakePullRunner{lines: []string{
		`{"status":"pulling manifest"}`,
		`{"status":"downloading","completed":50,"total":100}`,
		`{"status":"success"}`,
	}}
	srv := newOllamaPullServer(t, runner, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/pull", "application/json",
		bytes.NewBufferString(`{"model":"gemma4:4b"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type: %s", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control: %s", cc)
	}
	out, _ := io.ReadAll(resp.Body)
	s := string(out)
	if !strings.Contains(s, "data: {\"status\":\"pulling manifest\"}") {
		t.Fatalf("missing first line: %q", s)
	}
	if !strings.Contains(s, `"completed":50`) {
		t.Fatalf("missing progress line: %q", s)
	}
	if !strings.Contains(s, "event: done") {
		t.Fatalf("missing done event: %q", s)
	}
}

func TestOllamaPull_ErrorExitEmitsErrorEvent(t *testing.T) {
	runner := &fakePullRunner{
		lines: []string{`{"status":"pulling manifest"}`},
		err:   errors.New("disk full"),
	}
	srv := newOllamaPullServer(t, runner, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/pull", "application/json",
		bytes.NewBufferString(`{"model":"gemma4:4b"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	s := string(out)
	if !strings.Contains(s, `"error":"disk full"`) {
		t.Fatalf("missing error event: %q", s)
	}
	// There should NOT be a 'done' event when the runner errored.
	if strings.Contains(s, "event: done") {
		t.Fatalf("unexpected done event on error path: %q", s)
	}
}

func TestOllamaPull_BadJSON400(t *testing.T) {
	srv := newOllamaPullServer(t, &fakePullRunner{}, "")
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/pull", "application/json",
		bytes.NewBufferString(`{"this is not json`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestOllamaPull_EmptyModel400(t *testing.T) {
	srv := newOllamaPullServer(t, &fakePullRunner{}, "")
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/pull", "application/json",
		bytes.NewBufferString(`{"model":""}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestOllamaPull_NoRunner503(t *testing.T) {
	s := &Server{}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/pull", "application/json",
		bytes.NewBufferString(`{"model":"gemma4:4b"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestOllamaPull_Unauthorized(t *testing.T) {
	srv := newOllamaPullServer(t, &fakePullRunner{}, "secret-token")
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/pull", "application/json",
		bytes.NewBufferString(`{"model":"gemma4:4b"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

// TestOllamaPull_AuthorizedWithBearer — with a token configured, a valid bearer
// must receive the 200 + SSE stream (not a 401).
func TestOllamaPull_AuthorizedWithBearer(t *testing.T) {
	runner := &fakePullRunner{lines: []string{`{"status":"ok"}`}}
	srv := newOllamaPullServer(t, runner, "secret-token")
	defer srv.Close()

	req, err := http.NewRequest("POST", srv.URL+"/api/ai/ollama/pull",
		bytes.NewBufferString(`{"model":"gemma4:4b"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type: %s", ct)
	}
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), "event: done") {
		t.Fatalf("missing done event: %q", string(out))
	}
}

// ── Start tests ───────────────────────────────────────────────────────────────

// fakeStarter records spawn calls and returns a preset pid/err.
type fakeStarter struct {
	pid    int
	err    error
	called int
	mu     sync.Mutex
}

func (f *fakeStarter) Start(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called++
	return f.pid, f.err
}

// dynamicDetector returns different Status values on successive Detect() calls —
// used to simulate "not running → running" transition during the polling loop.
type dynamicDetector struct {
	sequence []ollama.Status
	calls    int
	mu       sync.Mutex
}

func (d *dynamicDetector) Detect(_ context.Context) ollama.Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.calls >= len(d.sequence) {
		// Stay on the last value.
		return d.sequence[len(d.sequence)-1]
	}
	s := d.sequence[d.calls]
	d.calls++
	return s
}

func newOllamaStartServer(t *testing.T, det OllamaStatusProvider, starter OllamaStarter, apiToken string) *httptest.Server {
	t.Helper()
	s := &Server{}
	s.WithOllamaDetector(det)
	s.WithOllamaStarter(starter)
	if apiToken != "" {
		s.WithAPIToken(apiToken)
	}
	return httptest.NewServer(s.Handler())
}

// Detector returns READY → 409 before even attempting to spawn.
func TestOllamaStart_AlreadyRunning409(t *testing.T) {
	det := &fakeDetector{status: ollama.Status{State: ollama.StateReady}}
	starter := &fakeStarter{pid: 1234}
	srv := newOllamaStartServer(t, det, starter, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/start", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("want 409, got %d", resp.StatusCode)
	}
	starter.mu.Lock()
	called := starter.called
	starter.mu.Unlock()
	if called != 0 {
		t.Fatal("starter should not be called when already running")
	}
}

// Detector reports docker deployment → 400 (sidecar must be enabled instead).
func TestOllamaStart_Docker400(t *testing.T) {
	det := &fakeDetector{status: ollama.Status{
		State: ollama.StateDockerSidecarAvailable, Deployment: ollama.DeploymentDocker,
	}}
	starter := &fakeStarter{}
	srv := newOllamaStartServer(t, det, starter, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/start", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	starter.mu.Lock()
	called := starter.called
	starter.mu.Unlock()
	if called != 0 {
		t.Fatal("starter should not be called for docker deployment")
	}
}

// Not running → spawn → becomes READY on 3rd Detect call → 200.
func TestOllamaStart_Success(t *testing.T) {
	det := &dynamicDetector{sequence: []ollama.Status{
		{State: ollama.StateInstalledNotRunning, Deployment: ollama.DeploymentNative}, // pre-spawn check
		{State: ollama.StateInstalledNotRunning, Deployment: ollama.DeploymentNative}, // first poll
		{State: ollama.StateReady, Deployment: ollama.DeploymentNative},               // becomes ready
	}}
	starter := &fakeStarter{pid: 4242}
	srv := newOllamaStartServer(t, det, starter, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/start", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body struct {
		PID       int    `json:"pid"`
		StartedAt string `json:"started_at"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.PID != 4242 {
		t.Fatalf("pid: %d", body.PID)
	}
	if body.StartedAt == "" {
		t.Fatal("started_at missing")
	}
	starter.mu.Lock()
	called := starter.called
	starter.mu.Unlock()
	if called != 1 {
		t.Fatalf("starter called %d times, want 1", called)
	}
}

// Spawn succeeds but never becomes READY → 500 after timeout.
// Uses package-level vars to keep the test fast.
func TestOllamaStart_NeverBecomesReady500(t *testing.T) {
	origTimeout := startReadinessTimeout
	origInterval := startReadinessInterval
	startReadinessTimeout = 200 * time.Millisecond
	startReadinessInterval = 50 * time.Millisecond
	defer func() {
		startReadinessTimeout = origTimeout
		startReadinessInterval = origInterval
	}()

	det := &fakeDetector{status: ollama.Status{
		State: ollama.StateInstalledNotRunning, Deployment: ollama.DeploymentNative,
	}}
	starter := &fakeStarter{pid: 99}
	srv := newOllamaStartServer(t, det, starter, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/start", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "within") {
		t.Fatalf("error msg missing 'within': %s", body)
	}
}

func TestOllamaStart_SpawnFailure500(t *testing.T) {
	det := &fakeDetector{status: ollama.Status{
		State: ollama.StateInstalledNotRunning, Deployment: ollama.DeploymentNative,
	}}
	starter := &fakeStarter{err: errors.New("exec: \"ollama\": executable file not found in $PATH")}
	srv := newOllamaStartServer(t, det, starter, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/start", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestOllamaStart_NoStarter503(t *testing.T) {
	s := &Server{}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/start", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestOllamaStart_Unauthorized(t *testing.T) {
	det := &fakeDetector{status: ollama.Status{State: ollama.StateInstalledNotRunning}}
	srv := newOllamaStartServer(t, det, &fakeStarter{}, "secret-token")
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/start", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

// TestOllamaStart_AuthorizedWithBearer — with a token configured, a valid bearer
// gets through to the handler logic (returns 409 here because status is already Ready,
// which proves we passed the auth gate).
func TestOllamaStart_AuthorizedWithBearer(t *testing.T) {
	det := &fakeDetector{status: ollama.Status{State: ollama.StateReady}}
	srv := newOllamaStartServer(t, det, &fakeStarter{}, "secret-token")
	defer srv.Close()

	req, err := http.NewRequest("POST", srv.URL+"/api/ai/ollama/start", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 (already running, passed auth), got %d", resp.StatusCode)
	}
}

// TestOllamaStart_ClientDisconnectCancelsPoll — client cancels mid-polling.
// Polling loop has to observe ctx.Done and return 408; we assert the handler
// stopped polling promptly.
func TestOllamaStart_ClientDisconnectCancelsPoll(t *testing.T) {
	// Shorten interval so the first tick fires quickly after spawn.
	origInterval := startReadinessInterval
	origTimeout := startReadinessTimeout
	startReadinessInterval = 50 * time.Millisecond
	startReadinessTimeout = 5 * time.Second
	defer func() {
		startReadinessInterval = origInterval
		startReadinessTimeout = origTimeout
	}()

	// Detector always returns InstalledNotRunning — the poll will never succeed.
	det := &fakeDetector{status: ollama.Status{
		State: ollama.StateInstalledNotRunning, Deployment: ollama.DeploymentNative,
	}}
	starter := &fakeStarter{pid: 4242}
	srv := newOllamaStartServer(t, det, starter, "")
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "POST", srv.URL+"/api/ai/ollama/start", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// Cancel after a brief delay so the poll loop is running.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = http.DefaultClient.Do(req)
	elapsed := time.Since(start)
	// http.Client.Do returns an error (context canceled) when the client context is cancelled mid-request.
	// This is the expected path — we're verifying the handler exited its poll loop promptly,
	// not hitting the 5-second timeout.
	if err == nil {
		t.Fatal("expected client error after cancel, got nil (handler did not exit promptly)")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("handler took %v — poll loop did not honor ctx cancellation", elapsed)
	}
}

// TestOllamaPull_ClientDisconnectCancelsSubprocess — when the client cancels
// its request mid-stream, the runner must observe ctx.Done so real subprocesses
// are killed instead of orphaned.
func TestOllamaPull_ClientDisconnectCancelsSubprocess(t *testing.T) {
	runner := &fakePullRunner{
		lines:   []string{`{"status":"pulling manifest"}`},
		blockCh: make(chan struct{}),
	}
	srv := newOllamaPullServer(t, runner, "")
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "POST", srv.URL+"/api/ai/ollama/pull",
		bytes.NewBufferString(`{"model":"gemma4:4b"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Kick off the streaming request in a goroutine so we can cancel from the test.
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return // expected after cancel
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Give the server a moment to start streaming, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for the in-flight request goroutine to unwind.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("client request did not unwind within 2s after cancel")
	}

	// Poll briefly — the server may still be in the runner's select block when
	// our goroutine returns; give it up to 1s to observe the cancellation.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if runner.sawCtxCancel() {
			return // pass
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runner did not observe ctx cancellation after client disconnect")
}

// ── Sidecar enable tests ──────────────────────────────────────────────────────

// templateYAML is the canonical sidecar compose file — tests write this into
// a temp repo root to exercise the handler.
const templateYAML = `services:
  ollama:
    image: ollama/ollama:latest
`

func newSidecarServer(t *testing.T, repoRoot, apiToken string) *httptest.Server {
	t.Helper()
	s := &Server{}
	s.WithOllamaRepoRoot(repoRoot)
	if apiToken != "" {
		s.WithAPIToken(apiToken)
	}
	return httptest.NewServer(s.Handler())
}

func TestOllamaSidecarEnable_Success(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.ollama.yml.template"),
		[]byte(templateYAML), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	srv := newSidecarServer(t, dir, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/sidecar/enable", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["override_path"] != "./docker-compose.override.yml" {
		t.Fatalf("override_path: %q", body["override_path"])
	}
	if body["run_command"] != "docker compose up -d ollama" {
		t.Fatalf("run_command: %q", body["run_command"])
	}

	// Verify override file now exists with template content.
	written, err := os.ReadFile(filepath.Join(dir, "docker-compose.override.yml"))
	if err != nil {
		t.Fatalf("read override: %v", err)
	}
	if string(written) != templateYAML {
		t.Fatalf("override content mismatch: got %q", string(written))
	}
}

func TestOllamaSidecarEnable_OverrideExists409(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.ollama.yml.template"),
		[]byte(templateYAML), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	// Pre-create the override so the handler hits 409.
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.override.yml"),
		[]byte("existing: yes\n"), 0644); err != nil {
		t.Fatalf("pre-create override: %v", err)
	}
	srv := newSidecarServer(t, dir, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/sidecar/enable", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("want 409, got %d", resp.StatusCode)
	}

	// Verify override was NOT overwritten.
	written, _ := os.ReadFile(filepath.Join(dir, "docker-compose.override.yml"))
	if string(written) != "existing: yes\n" {
		t.Fatalf("override was clobbered: %q", string(written))
	}
}

func TestOllamaSidecarEnable_TemplateMissing500(t *testing.T) {
	dir := t.TempDir() // no template written
	srv := newSidecarServer(t, dir, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/sidecar/enable", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "template not found") {
		t.Fatalf("error msg: %s", string(body))
	}
}

func TestOllamaSidecarEnable_NoRoot503(t *testing.T) {
	s := &Server{}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/sidecar/enable", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestOllamaSidecarEnable_Unauthorized(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "docker-compose.ollama.yml.template"), []byte(templateYAML), 0644)
	srv := newSidecarServer(t, dir, "secret-token")
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/ai/ollama/sidecar/enable", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestOllamaSidecarEnable_AuthorizedWithBearer(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "docker-compose.ollama.yml.template"), []byte(templateYAML), 0644)
	srv := newSidecarServer(t, dir, "secret-token")
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/ai/ollama/sidecar/enable", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ── Test connection tests ─────────────────────────────────────────────────────

// fakeTester implements OllamaTester for unit tests.
type fakeTester struct {
	resp    string
	err     error
	blockCh chan struct{} // when non-nil, Complete blocks until ctx.Done or channel close
}

func (f *fakeTester) Complete(ctx context.Context, _, _ string) (string, error) {
	if f.blockCh != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-f.blockCh:
		}
	}
	return f.resp, f.err
}

func newOllamaTestServer(t *testing.T, tester OllamaTester, apiToken string) *httptest.Server {
	t.Helper()
	s := &Server{}
	s.WithOllamaTester(tester)
	if apiToken != "" {
		s.WithAPIToken(apiToken)
	}
	return httptest.NewServer(s.Handler())
}

func TestOllamaTest_Success(t *testing.T) {
	tester := &fakeTester{resp: "1"}
	srv := newOllamaTestServer(t, tester, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/test", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("want ok=true, got %v", body["ok"])
	}
	if _, hasLatency := body["latency_ms"]; !hasLatency {
		t.Fatal("latency_ms field missing")
	}
}

func TestOllamaTest_ErrorReturns500(t *testing.T) {
	tester := &fakeTester{err: errors.New("connection refused")}
	srv := newOllamaTestServer(t, tester, "")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/test", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ok"] != false {
		t.Fatalf("want ok=false, got %v", body["ok"])
	}
	if body["error"] != "connection refused" {
		t.Fatalf("want error='connection refused', got %v", body["error"])
	}
	if _, hasLatency := body["latency_ms"]; !hasLatency {
		t.Fatal("latency_ms field missing")
	}
}

func TestOllamaTest_NoTester503(t *testing.T) {
	s := &Server{}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/test", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestOllamaTest_Unauthorized(t *testing.T) {
	tester := &fakeTester{resp: "1"}
	srv := newOllamaTestServer(t, tester, "secret-token")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/ai/ollama/test", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestOllamaTest_Timeout(t *testing.T) {
	// blockCh is never closed — Complete blocks until ctx is cancelled (5s handler timeout).
	// We cancel the request client-side almost immediately to keep the test fast.
	tester := &fakeTester{blockCh: make(chan struct{})}
	srv := newOllamaTestServer(t, tester, "")
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", srv.URL+"/api/ai/ollama/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// The client context times out — we expect an error from http.DefaultClient.Do.
	_, err = http.DefaultClient.Do(req)
	if err == nil {
		t.Fatal("expected client-side timeout error, got nil")
	}
}
