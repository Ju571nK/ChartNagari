package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/ollama"
)

// OllamaTester is a minimal interface for testing the Ollama connection.
// *llm.OllamaProvider satisfies this structurally.
type OllamaTester interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// testOllamaConnection handles POST /api/ai/ollama/test.
// It sends a minimal prompt to Ollama and reports round-trip latency.
func (s *Server) testOllamaConnection(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.ollamaTester == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ollama tester not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_, err := s.ollamaTester.Complete(ctx, "", "1")
	latency := time.Since(start)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         false,
			"error":      err.Error(),
			"latency_ms": latency.Milliseconds(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"latency_ms": latency.Milliseconds(),
	})
}

// Readiness poll parameters for the start handler. Overridable in tests.
var (
	startReadinessTimeout  = 10 * time.Second
	startReadinessInterval = 500 * time.Millisecond
)

// OllamaStarter spawns `ollama serve` as a detached background subprocess.
// *ollama.osStarter satisfies this interface via ollama.DefaultStarter().
type OllamaStarter interface {
	// Start spawns `ollama serve` as a detached subprocess and returns its PID.
	// An error means the spawn itself failed (e.g., binary missing).
	Start(ctx context.Context) (pid int, err error)
}

// writeJSONError writes a JSON {"error": msg} body with the given HTTP status code.
// It follows the same pattern as writeUnauthorized in auth.go.
func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// OllamaPullRunner streams progress from `ollama pull <model>`. Each stdout
// line (JSONL from Ollama) is delivered to onLine in arrival order. Returns
// the process exit error (nil on clean exit). Cancelling ctx kills the process.
type OllamaPullRunner interface {
	Pull(ctx context.Context, model string, onLine func(line []byte)) error
}

// OllamaStatusProvider is the minimal interface the api package needs from the
// ollama detector. *ollama.Detector satisfies it. Tests can substitute a fake.
type OllamaStatusProvider interface {
	Detect(ctx context.Context) ollama.Status
}

// getOllamaStatus returns the detector's current view of Ollama state. Protected
// by requireBearer — the response includes host/model/version which are not
// sensitive, but the endpoint also drives UI that controls subprocess lifecycle,
// so we keep it behind auth.
func (s *Server) getOllamaStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.ollamaDetector == nil {
		http.Error(w, "ollama detector not configured", http.StatusServiceUnavailable)
		return
	}
	status := s.ollamaDetector.Detect(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Error().Err(err).Msg("api: ollama status encode")
	}
}

// pullOllamaModel streams `ollama pull <model>` progress to the client via SSE.
func (s *Server) pullOllamaModel(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.ollamaPullRunner == nil {
		http.Error(w, "ollama pull runner not configured", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // hint to reverse proxies (nginx) not to buffer
	w.WriteHeader(http.StatusOK)

	// Stream stdout lines as SSE data events.
	onLine := func(line []byte) {
		// Ensure each line is a single SSE data frame — Ollama emits one JSON per line.
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(line)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}

	err := s.ollamaPullRunner.Pull(r.Context(), body.Model, onLine)
	if err != nil {
		// Emit a final error event, then close.
		payload, _ := json.Marshal(map[string]string{"error": err.Error()})
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
		log.Warn().Err(err).Str("model", body.Model).Msg("api: ollama pull exited with error")
		return
	}

	// Clean exit — signal EOS via a conventional done event.
	_, _ = w.Write([]byte("event: done\ndata: {}\n\n"))
	flusher.Flush()
}

// startOllama handles POST /api/ai/ollama/start.
// It checks if Ollama is already running, rejects docker deployments, spawns
// `ollama serve` as a detached subprocess, and polls until ready (up to 10s).
func (s *Server) startOllama(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.ollamaStarter == nil || s.ollamaDetector == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ollama start not configured")
		return
	}

	// Step 1: already running?
	status := s.ollamaDetector.Detect(r.Context())
	if status.State == ollama.StateReady || status.State == ollama.StateReadyNoModel {
		writeJSONError(w, http.StatusConflict, "already running")
		return
	}

	// Step 2: native only — docker deployments must use enable-sidecar.
	if status.Deployment == ollama.DeploymentDocker {
		writeJSONError(w, http.StatusBadRequest, "docker sidecar detected; use sidecar/enable instead")
		return
	}

	// Step 3: spawn.
	pid, err := s.ollamaStarter.Start(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("api: ollama start spawn failed")
		writeJSONError(w, http.StatusInternalServerError, "spawn failed: "+err.Error())
		return
	}
	startedAt := time.Now().UTC().Format(time.RFC3339)

	// Step 4: poll readiness.
	deadline := time.Now().Add(startReadinessTimeout)
	tick := time.NewTicker(startReadinessInterval)
	defer tick.Stop()
	for {
		st := s.ollamaDetector.Detect(r.Context())
		if st.State == ollama.StateReady || st.State == ollama.StateReadyNoModel {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pid":        pid,
				"started_at": startedAt,
			})
			return
		}
		if time.Now().After(deadline) {
			log.Warn().Int("pid", pid).Dur("timeout", startReadinessTimeout).Msg("api: ollama did not become ready within timeout")
			writeJSONError(w, http.StatusInternalServerError, "ollama did not become ready within 10s")
			return
		}
		select {
		case <-r.Context().Done():
			writeJSONError(w, http.StatusRequestTimeout, "client cancelled")
			return
		case <-tick.C:
		}
	}
}

// enableOllamaSidecar handles POST /api/ai/ollama/sidecar/enable.
// It copies docker-compose.ollama.yml.template to docker-compose.override.yml
// in the repo root so the user can then run `docker compose up -d ollama`.
func (s *Server) enableOllamaSidecar(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.ollamaRepoRoot == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "sidecar enable not configured")
		return
	}

	overridePath := filepath.Join(s.ollamaRepoRoot, "docker-compose.override.yml")
	templatePath := filepath.Join(s.ollamaRepoRoot, "docker-compose.ollama.yml.template")

	// 1. Already enabled?
	if _, err := os.Stat(overridePath); err == nil {
		writeJSONError(w, http.StatusConflict, "override file already exists")
		return
	} else if !os.IsNotExist(err) {
		log.Error().Err(err).Str("path", overridePath).Msg("api: stat override")
		writeJSONError(w, http.StatusInternalServerError, "filesystem error")
		return
	}

	// 2. Template readable?
	content, err := os.ReadFile(templatePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusInternalServerError, "template not found")
			return
		}
		log.Error().Err(err).Str("path", templatePath).Msg("api: read template")
		writeJSONError(w, http.StatusInternalServerError, "cannot read template")
		return
	}

	// 3. Write override.
	if err := os.WriteFile(overridePath, content, 0644); err != nil {
		log.Error().Err(err).Str("path", overridePath).Msg("api: write override")
		writeJSONError(w, http.StatusInternalServerError, "cannot write override")
		return
	}

	// 4. Success.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"override_path": "./docker-compose.override.yml",
		"run_command":   "docker compose up -d ollama",
	})
}
