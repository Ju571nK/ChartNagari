package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	stdruntime "runtime"
	"testing"
	"time"

	"errors"
)

// fakeRuntime is a test double for RuntimeInspector — gives tests full control
// over all OS / process / file seams without touching real system calls.
type fakeRuntime struct {
	inDocker   bool
	ollamaVer  string
	ollamaErr  error
	overrideOK bool
	templateOK bool
}

func (f *fakeRuntime) InDocker() bool                 { return f.inDocker }
func (f *fakeRuntime) OllamaVersion() (string, error) { return f.ollamaVer, f.ollamaErr }
func (f *fakeRuntime) OverrideFileExists() bool       { return f.overrideOK }
func (f *fakeRuntime) SidecarTemplateExists() bool    { return f.templateOK }

// tagsResponse mirrors the Ollama /api/tags response shape. Only name is parsed.
type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// newTagsServer builds an httptest server that returns the supplied model names.
func newTagsServer(t *testing.T, modelNames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		resp := tagsResponse{}
		for _, name := range modelNames {
			resp.Models = append(resp.Models, struct {
				Name string `json:"name"`
			}{Name: name})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestDetect_Ready — /api/tags returns 200 and the target model is present.
func TestDetect_Ready(t *testing.T) {
	srv := newTagsServer(t, []string{"gemma4:4b", "llama3.1:8b"})
	defer srv.Close()

	rt := &fakeRuntime{inDocker: false}
	d := NewDetector(srv.URL, "gemma4:4b", rt)

	status := d.Detect(context.Background())

	if status.State != StateReady {
		t.Errorf("State = %q; want %q", status.State, StateReady)
	}
	if status.Suggest.Action != "ready" {
		t.Errorf("Suggest.Action = %q; want %q", status.Suggest.Action, "ready")
	}
	found := false
	for _, m := range status.ModelsAvailable {
		if m == "gemma4:4b" {
			found = true
		}
	}
	if !found {
		t.Errorf("ModelsAvailable %v does not contain %q", status.ModelsAvailable, "gemma4:4b")
	}
	if status.Host != srv.URL {
		t.Errorf("Host = %q; want %q", status.Host, srv.URL)
	}
	if status.Model != "gemma4:4b" {
		t.Errorf("Model = %q; want %q", status.Model, "gemma4:4b")
	}
	if status.Deployment != DeploymentNative {
		t.Errorf("Deployment = %q; want %q", status.Deployment, DeploymentNative)
	}
}

// TestDetect_ReadyNoModel — /api/tags returns 200 but the target model is absent.
func TestDetect_ReadyNoModel(t *testing.T) {
	srv := newTagsServer(t, []string{"llama3.1:8b"})
	defer srv.Close()

	rt := &fakeRuntime{inDocker: false}
	d := NewDetector(srv.URL, "gemma4:4b", rt)

	status := d.Detect(context.Background())

	if status.State != StateReadyNoModel {
		t.Errorf("State = %q; want %q", status.State, StateReadyNoModel)
	}
	if status.Suggest.Action != "pull_model" {
		t.Errorf("Suggest.Action = %q; want %q", status.Suggest.Action, "pull_model")
	}
	if status.Suggest.Command != "ollama pull gemma4:4b" {
		t.Errorf("Suggest.Command = %q; want %q", status.Suggest.Command, "ollama pull gemma4:4b")
	}
	if status.Suggest.SizeBytes != 2_600_000_000 {
		t.Errorf("Suggest.SizeBytes = %d; want %d", status.Suggest.SizeBytes, int64(2_600_000_000))
	}
	if len(status.ModelsAvailable) == 0 {
		t.Error("ModelsAvailable should be non-empty when /api/tags returns 200")
	}
}

// TestDetect_InstalledNotRunning — host is unreachable but OllamaVersion succeeds.
func TestDetect_InstalledNotRunning(t *testing.T) {
	// Port 1 is reserved/unroutable; the 2s client timeout will fire immediately.
	rt := &fakeRuntime{
		inDocker:  false,
		ollamaVer: "ollama version 0.3.14",
		ollamaErr: nil,
	}
	d := NewDetector("http://127.0.0.1:1", "gemma4:4b", rt)

	status := d.Detect(context.Background())

	if status.State != StateInstalledNotRunning {
		t.Errorf("State = %q; want %q", status.State, StateInstalledNotRunning)
	}
	if status.Version != "ollama version 0.3.14" {
		t.Errorf("Version = %q; want %q", status.Version, "ollama version 0.3.14")
	}
	if status.Suggest.Action != "start_ollama" {
		t.Errorf("Suggest.Action = %q; want %q", status.Suggest.Action, "start_ollama")
	}
	if status.Suggest.Command != "ollama serve" {
		t.Errorf("Suggest.Command = %q; want %q", status.Suggest.Command, "ollama serve")
	}
}

// TestDetect_DockerSidecarAvailable — unreachable host, OllamaVersion errors,
// inside Docker, sidecar template present, override absent.
func TestDetect_DockerSidecarAvailable(t *testing.T) {
	rt := &fakeRuntime{
		inDocker:   true,
		ollamaErr:  errors.New("exec: not found"),
		templateOK: true,
		overrideOK: false,
	}
	d := NewDetector("http://127.0.0.1:1", "gemma4:4b", rt)

	status := d.Detect(context.Background())

	if status.State != StateDockerSidecarAvailable {
		t.Errorf("State = %q; want %q", status.State, StateDockerSidecarAvailable)
	}
	if status.Deployment != DeploymentDocker {
		t.Errorf("Deployment = %q; want %q", status.Deployment, DeploymentDocker)
	}
	if status.Suggest.Action != "enable_sidecar" {
		t.Errorf("Suggest.Action = %q; want %q", status.Suggest.Action, "enable_sidecar")
	}
	if status.Suggest.Command != "" {
		t.Errorf("Suggest.Command should be empty for enable_sidecar; got %q", status.Suggest.Command)
	}
}

// TestDetect_NotInstalled — unreachable host, OllamaVersion errors, not in Docker.
// Verifies StateNotInstalled and that Suggest.Action is "install_ollama" with a
// non-empty Command for the current platform.
func TestDetect_NotInstalled(t *testing.T) {
	rt := &fakeRuntime{
		inDocker:  false,
		ollamaErr: errors.New("exec: not found"),
	}
	d := NewDetector("http://127.0.0.1:1", "gemma4:4b", rt)

	status := d.Detect(context.Background())

	if status.State != StateNotInstalled {
		t.Errorf("State = %q; want %q", status.State, StateNotInstalled)
	}
	if status.Suggest.Action != "install_ollama" {
		t.Errorf("Suggest.Action = %q; want %q", status.Suggest.Action, "install_ollama")
	}
	if status.Suggest.Command == "" {
		t.Error("Suggest.Command must be non-empty for install_ollama")
	}
	// Verify the helper itself returns the expected value for the current GOOS.
	want := installCommandForGOOS(stdruntime.GOOS)
	if status.Suggest.Command != want {
		t.Errorf("Suggest.Command = %q; want %q (from installCommandForGOOS)", status.Suggest.Command, want)
	}
}

// TestInstallCommandForGOOS exercises the helper for all known platforms so the
// table is verified independently of whatever machine runs the tests.
func TestInstallCommandForGOOS(t *testing.T) {
	cases := []struct {
		goos string
		want string
	}{
		{"linux", "curl -fsSL https://ollama.com/install.sh | sh"},
		{"darwin", "brew install ollama"},
		{"windows", "winget install Ollama.Ollama"},
		{"plan9", "https://ollama.com/download"},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			got := installCommandForGOOS(tc.goos)
			if got != tc.want {
				t.Errorf("installCommandForGOOS(%q) = %q; want %q", tc.goos, got, tc.want)
			}
		})
	}
}

// TestDetect_HTTPTimeout — the httptest server sleeps longer than the 2s client
// timeout.  Detect must return without hanging.
func TestDetect_HTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := &fakeRuntime{
		inDocker:  false,
		ollamaVer: "ollama version 0.3.14",
		ollamaErr: nil,
	}
	d := NewDetector(srv.URL, "gemma4:4b", rt)

	start := time.Now()
	status := d.Detect(context.Background())
	elapsed := time.Since(start)

	// Must return before the server's 5-second sleep completes.
	if elapsed >= 4*time.Second {
		t.Errorf("Detect took %v; expected < 4s (2s client timeout should fire)", elapsed)
	}
	// Because OllamaVersion succeeds, the fallthrough state is InstalledNotRunning.
	if status.State != StateInstalledNotRunning {
		t.Errorf("State = %q; want %q", status.State, StateInstalledNotRunning)
	}
}

// TestDetect_Http500FallsThrough — /api/tags returns 500; with no installed binary
// the state must not be READY or READY_NO_MODEL.
func TestDetect_Http500FallsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	// Ollama is "running" but /api/tags is 500ing. fakeRuntime reports NO installed binary
	// so we fall through to NOT_INSTALLED (vs. INSTALLED_NOT_RUNNING).
	det := NewDetector(srv.URL, "gemma4:4b", &fakeRuntime{
		ollamaErr: errors.New("ollama: command not found"),
	})
	st := det.Detect(context.Background())
	if st.State == StateReady || st.State == StateReadyNoModel {
		t.Fatalf("HTTP 500 should NOT produce a READY state, got %s", st.State)
	}
}

// TestDetect_MalformedJSONFallsThrough — /api/tags returns 200 with invalid JSON;
// treated as unreachable → INSTALLED_NOT_RUNNING because ollama --version succeeds.
func TestDetect_MalformedJSONFallsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	det := NewDetector(srv.URL, "gemma4:4b", &fakeRuntime{
		ollamaVer: "ollama version 0.3.14",
	})
	st := det.Detect(context.Background())
	// Malformed 200 response is treated as unreachable → INSTALLED_NOT_RUNNING because ollama --version succeeds.
	if st.State != StateInstalledNotRunning {
		t.Fatalf("malformed JSON should fall through to INSTALLED_NOT_RUNNING, got %s", st.State)
	}
}

// TestDetect_EmptyModelsList — /api/tags returns 200 with an empty models array;
// server is reachable but no model is present → READY_NO_MODEL.
func TestDetect_EmptyModelsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	det := NewDetector(srv.URL, "gemma4:4b", &fakeRuntime{})
	st := det.Detect(context.Background())
	if st.State != StateReadyNoModel {
		t.Fatalf("empty models list with 200 should be READY_NO_MODEL, got %s", st.State)
	}
	if st.Suggest.Action != "pull_model" {
		t.Fatalf("suggest action: want pull_model, got %q", st.Suggest.Action)
	}
	if st.Suggest.Command != "ollama pull gemma4:4b" {
		t.Fatalf("suggest command: %q", st.Suggest.Command)
	}
	if st.Suggest.SizeBytes != 2_600_000_000 {
		t.Fatalf("size bytes: %d", st.Suggest.SizeBytes)
	}
}
