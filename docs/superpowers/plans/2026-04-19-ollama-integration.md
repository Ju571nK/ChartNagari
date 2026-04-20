# Ollama Local LLM Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Ollama as a fifth LLM provider, opt-in only, with a guided Settings UI that detects Ollama's installation state and walks the user through activation (native install / Docker sidecar / model pull / start).

**Architecture:** Hybrid. Native path pings `http://localhost:11434`; Docker path writes a `docker-compose.override.yml` that adds an `ollama` service on the same compose network. One React component (`OllamaSettings.tsx`) renders 5 state-specific cards driven by a single status endpoint. No auto-activation, no cross-provider fallback.

**Tech Stack:** Go 1.26, stdlib `net/http` + `exec`, Vite + React 18 + TS, SSE for pull progress. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-04-19-ollama-integration-design.md`

---

## Phase A — Backend provider + config

### Task 1: `internal/llm/ollama.go` — core provider implementation

**Files:**
- Create: `internal/llm/ollama.go`
- Test: `internal/llm/ollama_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/llm/ollama_test.go`:

```go
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaProvider_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("bad path: %s", r.URL.Path)
		}
		var body struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "gemma4:4b" || body.Stream != false {
			t.Fatalf("unexpected body: %+v", body)
		}
		if !strings.Contains(body.Prompt, "SYSTEM") || !strings.Contains(body.Prompt, "USER") {
			t.Fatalf("prompt not composed: %q", body.Prompt)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": "interpretation here"})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 5*time.Second)
	out, err := p.Complete(context.Background(), "SYSTEM", "USER")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "interpretation here" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestOllamaProvider_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"response": "too late"})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 10*time.Millisecond)
	_, err := p.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestOllamaProvider_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model not found"})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "missing:model", 1*time.Second)
	_, err := p.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "model not found") && !strings.Contains(err.Error(), "404") {
		t.Fatalf("error didn't propagate upstream message: %v", err)
	}
}

func TestOllamaProvider_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 1*time.Second)
	_, err := p.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
}

func TestOllamaProvider_HonorsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Complete(ctx, "s", "u")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/llm/ -run TestOllamaProvider -v
```
Expected: FAIL — `NewOllamaProvider` undefined.

- [ ] **Step 3: Implement the provider**

Create `internal/llm/ollama.go`:

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaProvider implements the Provider interface against a local (or
// compose-networked) Ollama server via POST /api/generate. It is opt-in —
// cmd/server/main.go only selects it when LLMProvider == "ollama".
type OllamaProvider struct {
	host   string
	model  string
	client *http.Client
}

// NewOllamaProvider constructs a provider. host should include scheme
// (e.g., "http://localhost:11434"). timeout <= 0 falls back to 120s —
// local inference can be slow on modest hardware.
func NewOllamaProvider(host, model string, timeout time.Duration) *OllamaProvider {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OllamaProvider{
		host:   host,
		model:  model,
		client: &http.Client{Timeout: timeout},
	}
}

// Complete satisfies the Provider interface.
func (p *OllamaProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	prompt := systemPrompt
	if prompt != "" && userPrompt != "" {
		prompt += "\n\n"
	}
	prompt += userPrompt

	body, err := json.Marshal(map[string]any{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"temperature":  0.3,
			"num_predict":  512,
		},
	})
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to surface Ollama's own error message.
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &errBody)
		if errBody.Error != "" {
			return "", fmt.Errorf("ollama: %d %s", resp.StatusCode, errBody.Error)
		}
		return "", fmt.Errorf("ollama: %d %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("ollama: parse response: %w", err)
	}
	return out.Response, nil
}
```

- [ ] **Step 4: Run tests**

```
go test ./internal/llm/ -run TestOllamaProvider -race -v
```
Expected: PASS (5 cases).

- [ ] **Step 5: Commit**

```bash
git add internal/llm/ollama.go internal/llm/ollama_test.go
git commit -m "feat(llm): Ollama provider with timeout + context cancellation"
```

---

### Task 2: `internal/config/config.go` — `OllamaConfig` + env/YAML loading

**Files:**
- Modify: `internal/config/config.go` — add `OllamaConfig` struct + load logic
- Test: extend the existing config test file (grep for `TestLoad` to find it)

- [ ] **Step 1: Read existing config.go** to locate the pattern used for other providers (look at the `Anthropic`/`OpenAI`/`Groq`/`Gemini` blocks around lines 126-140 and 217-240). Mirror that style exactly.

- [ ] **Step 2: Write the failing test**

Append to the existing config test file:

```go
func TestLoad_OllamaConfig(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://ollama:11434")
	t.Setenv("OLLAMA_MODEL", "gemma4:12b")
	t.Setenv("OLLAMA_TIMEOUT_SEC", "60")

	cfg, err := Load("testdata/minimal.yaml") // or whatever pattern other tests use
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Ollama.Host != "http://ollama:11434" {
		t.Fatalf("host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "gemma4:12b" {
		t.Fatalf("model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout != 60*time.Second {
		t.Fatalf("timeout: %v", cfg.Ollama.Timeout)
	}
}

func TestLoad_OllamaDefaults(t *testing.T) {
	// With no env set and no YAML entry, defaults should apply.
	cfg, err := Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Ollama.Host != "http://localhost:11434" {
		t.Fatalf("default host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "gemma4:4b" {
		t.Fatalf("default model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout != 120*time.Second {
		t.Fatalf("default timeout: %v", cfg.Ollama.Timeout)
	}
}
```

- [ ] **Step 3: Run to verify failure**

- [ ] **Step 4: Add to `Config` struct and populate in `Load`:**

Near the other provider fields:

```go
type Config struct {
    // ... existing
    Ollama OllamaConfig
}

type OllamaConfig struct {
    Host    string
    Model   string
    Timeout time.Duration
}
```

In the YAML schema struct:

```go
Ollama struct {
    Host       string `yaml:"host"`
    Model      string `yaml:"model"`
    TimeoutSec int    `yaml:"timeout_sec"`
} `yaml:"ollama"`
```

In `Load`, after parsing the YAML, apply env overrides and defaults:

```go
cfg.Ollama.Host = firstNonEmpty(os.Getenv("OLLAMA_HOST"), raw.Ollama.Host, "http://localhost:11434")
cfg.Ollama.Model = firstNonEmpty(os.Getenv("OLLAMA_MODEL"), raw.Ollama.Model, "gemma4:4b")
timeoutSec := parseIntEnv("OLLAMA_TIMEOUT_SEC", raw.Ollama.TimeoutSec, 120)
cfg.Ollama.Timeout = time.Duration(timeoutSec) * time.Second
```

(Reuse whatever `firstNonEmpty` / env-parsing helpers the file already has. Check the existing Anthropic load path — do NOT invent new helpers if equivalents exist.)

- [ ] **Step 5: Run tests** — PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go  # + the test file
git commit -m "feat(config): OllamaConfig — host, model, timeout with env overrides"
```

---

### Task 3: `cmd/server/main.go` — wire `"ollama"` into provider switch

**Files:**
- Modify: `cmd/server/main.go` lines 366-387 (the `switch selectedProvider` block)

- [ ] **Step 1: Add the case**

Inside the switch, after the `case "gemini":` block:

```go
case "ollama":
    if cfg.Ollama.Host != "" && cfg.Ollama.Model != "" {
        llmProvider = llm.NewOllamaProvider(cfg.Ollama.Host, cfg.Ollama.Model, cfg.Ollama.Timeout)
        log.Info().
            Str("host", cfg.Ollama.Host).
            Str("model", cfg.Ollama.Model).
            Msg("Multi-analyst AI: using Ollama (local)")
    }
```

Also update the `else` branch of the `if llmProvider != nil` check (line 395) to include `OLLAMA` in the hint message:

```go
log.Info().Msg("Multi-analyst AI disabled (no API key — set ANTHROPIC/OPENAI/GROQ/GEMINI_API_KEY or LLM_PROVIDER=ollama)")
```

Do NOT add Ollama to the auto-select block (lines 353-364). Explicit selection only, per spec Q1=C.

- [ ] **Step 2: Verify build**

```
go build ./...
```
Expected: clean.

- [ ] **Step 3: Manual smoke (optional)**

```
LLM_PROVIDER=ollama OLLAMA_HOST=http://localhost:11434 OLLAMA_MODEL=gemma4:4b go run ./cmd/server -config config/server.yaml
```
Expected log line: `Multi-analyst AI: using Ollama (local)` (only works if you have Ollama running locally; otherwise the line still appears, but subsequent inferences will fail).

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(exec): wire Ollama provider into server selection switch"
```

---

## Phase B — Detection and control endpoints

### Task 4: `internal/ollama/detector.go` — platform + Ollama state detection

**Files:**
- Create: `internal/ollama/detector.go`
- Test: `internal/ollama/detector_test.go`

This is a new package under `internal/` (not under `internal/llm/` because it handles process/file detection, not inference).

Contract:

```go
package ollama

type Deployment string

const (
    DeploymentDocker Deployment = "docker"
    DeploymentNative Deployment = "native"
)

type State string

const (
    StateReady               State = "READY"
    StateReadyNoModel        State = "READY_NO_MODEL"
    StateInstalledNotRunning State = "INSTALLED_NOT_RUNNING"
    StateNotInstalled        State = "NOT_INSTALLED"
    StateDockerSidecarAvailable State = "DOCKER_SIDECAR_AVAILABLE"
)

type Status struct {
    State           State    `json:"state"`
    Host            string   `json:"host"`
    Model           string   `json:"model"`
    ModelsAvailable []string `json:"models_available,omitempty"`
    Deployment      Deployment `json:"deployment"`
    Version         string   `json:"version,omitempty"`
    Suggest         Suggest  `json:"suggest"`
}

type Suggest struct {
    Action    string `json:"action"`
    Command   string `json:"command,omitempty"`
    SizeBytes int64  `json:"size_bytes,omitempty"`
}

type Detector struct {
    host    string
    model   string
    client  *http.Client  // short-timeout client for /api/tags
    runtime RuntimeInspector  // interface for testability (see below)
}

type RuntimeInspector interface {
    InDocker() bool
    OllamaVersion() (string, error)      // runs "ollama --version"
    OverrideFileExists() bool            // checks for docker-compose.override.yml
    SidecarTemplateExists() bool         // checks for docker-compose.ollama.yml.template
}

func NewDetector(host, model string, runtime RuntimeInspector) *Detector
func (d *Detector) Detect(ctx context.Context) Status
```

Detection priority (in order, first match wins):

1. `/api/tags` returns 200 with the configured model present → `READY`
2. `/api/tags` returns 200 without the configured model → `READY_NO_MODEL`, suggest `pull_model`
3. `/api/tags` unreachable AND `ollama --version` succeeds → `INSTALLED_NOT_RUNNING`, suggest `start_ollama`
4. `/api/tags` unreachable AND `ollama --version` fails AND `InDocker()` returns true AND sidecar template exists AND override doesn't exist → `DOCKER_SIDECAR_AVAILABLE`, suggest `enable_sidecar`
5. Otherwise → `NOT_INSTALLED`, suggest platform-specific install command.

For model size lookup (the `size_bytes` field in `pull_model`), hardcode a map for the known recommended models:

```go
var modelSizes = map[string]int64{
    "gemma4:4b":    2_600_000_000,
    "gemma4:12b":   7_200_000_000,
    "gemma4:27b":  15_000_000_000,
    "llama3.1:8b":  4_700_000_000,
}
```

Tests use a fake `RuntimeInspector` + `httptest.NewServer` to exercise all five state transitions. No real `exec.Command` calls in tests.

- [ ] **Step 1: Write the failing tests** (5 test functions, one per state).

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5: Commit:**

```
feat(ollama): detector package with 5-state machine (ready / no-model / not-running / sidecar / not-installed)
```

---

### Task 5: `GET /api/ai/ollama/status` handler

**Files:**
- Modify: `internal/api/server.go` — add field `ollamaDetector *ollama.Detector` + `WithOllamaDetector` setter, register route
- Create: `internal/api/ollama_handler.go` — handler body
- Test: `internal/api/ollama_handler_test.go`

- [ ] **Step 1: Failing test**

```go
func TestOllamaStatus_ReadyState(t *testing.T) {
    srv, cleanup := newOllamaTestServer(t, &fakeDetector{status: ollama.Status{
        State: ollama.StateReady, Host: "http://localhost:11434", Model: "gemma4:4b",
        ModelsAvailable: []string{"gemma4:4b"}, Deployment: ollama.DeploymentNative,
    }})
    defer cleanup()
    resp := srv.get(t, "/api/ai/ollama/status")
    if resp.StatusCode != http.StatusOK { t.Fatalf("status: %d", resp.StatusCode) }
    var body ollama.Status
    _ = json.NewDecoder(resp.Body).Decode(&body)
    if body.State != ollama.StateReady { t.Fatalf("state: %s", body.State) }
}

func TestOllamaStatus_Unauthorized(t *testing.T) { /* ... */ }
```

Plus one test per other state (4 more).

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement handler** (auth via `requireBearer`, then delegate to `s.ollamaDetector.Detect(ctx)`).

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5: Wire in `cmd/server/main.go`** — construct real detector and call `apiSrv.WithOllamaDetector(...)`.

- [ ] **Step 6: Commit:**

```
feat(api): GET /api/ai/ollama/status — detection state machine endpoint
```

---

### Task 6: `POST /api/ai/ollama/pull` — SSE progress stream

**Files:**
- Modify: `internal/api/ollama_handler.go` — add `pullOllamaModel` handler
- Test: extend `ollama_handler_test.go`

Contract: accepts `{"model": "gemma4:4b"}`. Runs `ollama pull <model>` via `os/exec`. Parses stdout JSON-lines output (Ollama's pull command streams progress as JSONL) and relays each line to the client as SSE `data:` events. Closes stream when the subprocess exits.

Important:
- Do NOT block the handler on the subprocess — use goroutine + channel to pipe.
- Use `w.(http.Flusher).Flush()` after each SSE event.
- On client disconnect (`r.Context().Done()`), kill the subprocess.
- On subprocess exit non-zero, send a final `data: {"error": "..."}` event.

Tests use a fake subprocess: write a shell script that emits 3 progress lines + exits 0. Verify the SSE stream parses correctly.

- [ ] Steps 1-5 as standard. Commit: `feat(api): POST /api/ai/ollama/pull with SSE progress stream`.

---

### Task 7: `POST /api/ai/ollama/start` — background subprocess launch

**Files:**
- Modify: `internal/api/ollama_handler.go`
- Test: `ollama_handler_test.go`

Contract: checks `/api/tags` reachability first. If reachable, returns 409 `{"error":"already running"}`. Else spawns `ollama serve &` (detached), waits up to 10s polling `/api/tags`, returns `{"pid": ..., "started_at": "..."}` on success or 500 on timeout.

Native path only — returns 400 if `detector.deployment == docker`.

- [ ] Steps. Commit: `feat(api): POST /api/ai/ollama/start — background ollama serve launch`.

---

### Task 8: `POST /api/ai/ollama/sidecar/enable` + template file

**Files:**
- Create: `docker-compose.ollama.yml.template` (at repo root)
- Modify: `internal/api/ollama_handler.go` — add `enableOllamaSidecar` handler
- Test: `ollama_handler_test.go`

**Template file** (commit to repo):

```yaml
# Enable: POST /api/ai/ollama/sidecar/enable copies this file to
# docker-compose.override.yml, then run:
#   docker compose up -d ollama
# Uninstall: delete docker-compose.override.yml and run docker compose up -d.

services:
  ollama:
    image: ollama/ollama:latest
    container_name: chart-ollama
    restart: unless-stopped
    volumes:
      - ./data/ollama:/root/.ollama
    ports:
      - "127.0.0.1:11434:11434"
    healthcheck:
      test: ["CMD", "ollama", "list"]
      interval: 30s
      timeout: 5s
      retries: 3

  server:
    environment:
      - OLLAMA_HOST=http://ollama:11434
    depends_on:
      ollama:
        condition: service_healthy
```

**Handler:** reads the template, writes `docker-compose.override.yml` at repo root. If override already exists, returns 409. Returns `{"override_path": "./docker-compose.override.yml", "run_command": "docker compose up -d ollama"}`.

Tests use `t.TempDir()` + constructor injection of the repo root so we don't actually write at `./`.

- [ ] Steps. Commit: `feat(api): POST /api/ai/ollama/sidecar/enable + compose template`.

---

## Phase C — Frontend Settings UI

### Task 9: `OllamaSettings.tsx` component skeleton + 5-state render

**Files:**
- Create: `web/src/OllamaSettings.tsx`
- Test: `web/src/OllamaSettings.test.tsx`
- Modify: `web/src/App.tsx` — SettingsTab component adds `<OllamaSettings />` in an "AI Provider" section

**Component shape:**

```tsx
type Status = {
    state: 'READY' | 'READY_NO_MODEL' | 'INSTALLED_NOT_RUNNING' | 'NOT_INSTALLED' | 'DOCKER_SIDECAR_AVAILABLE'
    host: string
    model: string
    models_available?: string[]
    deployment: 'docker' | 'native'
    version?: string
    suggest: { action: string; command?: string; size_bytes?: number }
}

export default function OllamaSettings() {
    const [status, setStatus] = useState<Status | null>(null)
    // 5s poll only while Settings tab visible (use document.visibilityState + tab-level guard)
    // Render a card per state with the appropriate CTA button
}
```

- [ ] **Step 1: Failing tests** — 5 tests, one per state, asserting the right button/label renders. Mock fetch for `/api/ai/ollama/status`.

- [ ] **Step 2: Implement** — conditional rendering per `status.state`.

- [ ] **Step 3: Wire into SettingsTab** — find where SettingsTab renders its sections and add an "AI Provider" heading + `<OllamaSettings />`.

- [ ] **Step 4: Add i18n keys** — `settings.ai_provider`, `ollama.state_ready`, `ollama.state_no_model`, `ollama.state_not_running`, `ollama.state_not_installed`, `ollama.state_sidecar_available`, `ollama.pull_model`, `ollama.start_ollama`, `ollama.install_ollama`, `ollama.enable_sidecar`, `ollama.test_connection`. All three locales.

- [ ] **Step 5: Commit:**

```
feat(ui): OllamaSettings component with 5-state renderer
```

---

### Task 10: Pull button + SSE progress rendering

**Files:**
- Modify: `web/src/OllamaSettings.tsx`
- Test: `web/src/OllamaSettings.test.tsx`

Add `handlePull()` that calls `POST /api/ai/ollama/pull` and reads the SSE stream using `EventSource` or manual `fetch` + `getReader()`. Renders a progress bar with `completed / total * 100%`.

Test: mock a ReadableStream that emits 3 SSE events + closes. Verify the progress bar reaches 100% and the success state follows.

- [ ] Commit: `feat(ui): Ollama pull-model progress bar (SSE)`.

---

### Task 11: Start button + sidecar enable button

**Files:**
- Modify: `web/src/OllamaSettings.tsx`

Straightforward POSTs with loading + error handling. Sidecar button copies the run command (`docker compose up -d ollama`) to the clipboard after enable succeeds and shows a toast.

- [ ] Commit: `feat(ui): Ollama start + sidecar-enable buttons`.

---

### Task 12: Test connection button

**Files:**
- Modify: `web/src/OllamaSettings.tsx`

Sends a POST to `/api/ai/ollama/test` (new endpoint on backend — 1-token inference against configured model, reports latency). Renders `OK (1.3s)` or `FAILED: <reason>` inline.

Backend handler for `POST /api/ai/ollama/test`: runs `OllamaProvider.Complete(ctx, "", "1")` with a 5s timeout; returns `{"ok": true, "latency_ms": 1342}` or error.

- [ ] Commit: `feat(ui): Ollama test-connection button + /api/ai/ollama/test endpoint`.

---

## Phase D — Docs and release

### Task 13: User-facing setup guide

**Files:**
- Create: `docs/OLLAMA_SETUP.md`

Contents:
1. Why local LLM (privacy, cost, offline).
2. Two paths: Docker sidecar (easiest) vs native install.
3. Docker path: "Click Enable Ollama sidecar in Settings → run `docker compose up -d ollama` → click Pull gemma4:4b → done."
4. Native path: per-OS install commands (mac: `brew install ollama`, linux: `curl -fsSL https://ollama.com/install.sh | sh`, windows: direct download). Then `ollama serve` and `ollama pull gemma4:4b`.
5. Troubleshooting: port 11434 conflict, model not found, slow inference (hardware tips), docker compose doesn't see override.
6. Uninstall: delete `docker-compose.override.yml`; `docker compose down ollama`; `ollama rm gemma4:4b`.

- [ ] Commit: `docs(ollama): end-user setup guide with Docker and native paths`.

---

### Task 14: CHANGELOG + VERSION

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

Entry format:

```markdown
## [2.6.0.0] - YYYY-MM-DD

### Added
- **Ollama local LLM provider** (`internal/llm/ollama.go`) — fifth AI interpretation backend. Opt-in via `LLM_PROVIDER=ollama` or the Settings UI. Supports any Ollama-compatible model; default `gemma4:4b`. Local inference means trading data never leaves the machine.
- **AI Provider section in Settings** (`web/src/OllamaSettings.tsx`) — state-aware card that walks the user through installing Ollama, pulling a model, and enabling a Docker sidecar. Five states (READY / READY_NO_MODEL / INSTALLED_NOT_RUNNING / NOT_INSTALLED / DOCKER_SIDECAR_AVAILABLE), each with a one-click next action.
- **`/api/ai/ollama/*` endpoints** — status (state machine), pull (SSE progress), start (background `ollama serve`), sidecar/enable (compose override), test (1-token latency probe).
- **`docker-compose.ollama.yml.template`** — sidecar service definition. Enabled by the `sidecar/enable` endpoint; writes `docker-compose.override.yml`. No main compose changes required.
- **`docs/OLLAMA_SETUP.md`** — per-platform install guide + troubleshooting.

### Changed
- `cmd/server/main.go` provider switch accepts `"ollama"`. Auto-select block is unchanged — Ollama requires explicit selection.

### Deferred (see TODOS.md)
- Streaming `stream:true` with progressive UI rendering.
- Automatic first-use model pull.
- Context reuse across analyst personas.
- Cloud/local fallback policy.

### Manual verification checklist
- [ ] Settings tab → AI Provider section shows current Ollama state
- [ ] NOT_INSTALLED → install command copy button works
- [ ] Pull button streams progress and updates to READY when done
- [ ] Start button launches `ollama serve` and status transitions
- [ ] Sidecar enable writes override file and prints run command
- [ ] Setting `LLM_PROVIDER=ollama` routes signal interpretations to local model
- [ ] Ollama offline → signal still persists, ai_interpretation is empty, server logs WARN
```

Bump `VERSION` to `2.6.0.0`.

- [ ] Commit: `chore: bump version to 2.6.0.0 — Ollama integration`.

---

## Review Checklist

- [ ] `go test ./... -race` — green
- [ ] `cd web && npx vitest run` — green
- [ ] `cd web && npm run build` — tsc clean, Vite bundle succeeds
- [ ] Manual smoke (Task 14 checklist) — all items verified
- [ ] CHANGELOG entry committed
- [ ] VERSION bumped to 2.6.0.0

## Out-of-Scope Reminder

These are NOT part of this plan:
- Bundling the Ollama binary with our release artifacts.
- Automatic Ollama installation without user action.
- Cross-provider fallback on Ollama failure.
- Auto-selecting Ollama based on localhost availability.
- Streaming token-by-token inference.
- Benchmark harness comparing cloud vs local output quality.
