package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// fakePullRunner lets tests control the stream + exit error.
type fakePullRunner struct {
	lines []string
	err   error
}

func (f *fakePullRunner) Pull(_ context.Context, _ string, onLine func(line []byte)) error {
	for _, l := range f.lines {
		onLine([]byte(l))
	}
	return f.err
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
