# Ollama Local LLM Integration — Design

**Date:** 2026-04-19
**Author:** Justin Kwon
**Status:** APPROVED (ready for implementation plan)

## Problem

ChartNagari's multi-analyst AI interpretation currently has four cloud providers (Anthropic, OpenAI, Groq, Gemini). Every interpretation sends the raw trading signal — symbol, direction, entry/TP/SL, rule context, occasionally the user's trading journal fragments — to a third-party API. This contradicts the self-hosted positioning of the product:

- **Privacy:** signal data and any enriching context leaves the user's machine.
- **Cost:** every signal incurs per-token API spend, which is the biggest ongoing cost for a heavy user.
- **Availability:** cloud outages or rate limits silently disable the AI interpretation layer.
- **Offline:** the tool cannot run in air-gapped environments.

There is no local inference option.

## Goal

Add Ollama as a fifth provider so users can run `gemma4` (or any other Ollama-supported model) entirely on their own hardware. Keep cloud providers untouched. Make the local path feel like a first-class option — not a workaround — through guided setup in the UI.

## Non-goals

- **Bundling the Ollama binary.** Installing Ollama, managing OS-level sudo/admin prompts, and shipping model weights is out of scope. We defer to Ollama's own installer.
- **Auto-installing Ollama via our server.** Ruled out during design (2026-04-19): the maintenance surface is too large (platform matrices, elevation prompts, broken installer URLs).
- **Cross-provider fallback.** If Ollama is the selected provider and it fails, the signal's AI interpretation is skipped. No automatic hop to a cloud provider (decision Q2=A).
- **Auto-select by Ollama presence.** The user must explicitly pick Ollama in config or Settings UI (decision Q1=C).
- **Streaming responses.** MVP uses blocking `stream: false` against `/api/generate`. Streaming is a polish item for a later phase.
- **Multi-analyst pipeline optimization.** The four analyst personas still run sequentially; we don't batch them into a single Ollama call. Whether local inference warrants a different prompt structure is out of scope.
- **Model download from our UI.** Pull-model support is included (decision Q4=B) but running as a shell-out to `ollama pull`, not via our own download pipeline.

## Architecture — hybrid (Docker sidecar + host detection)

Two deployment paths, one UI surface:

```
                 ┌─────────────────────────────────────────────┐
                 │  web/src/OllamaSettings.tsx (within         │
                 │  SettingsTab → "AI Provider" section)       │
                 └──────────────────────┬──────────────────────┘
                                        ↓
                 ┌─────────────────────────────────────────────┐
                 │  GET /api/ai/ollama/status                  │
                 │  → { installed, running, models[], host,    │
                 │      deployment: "docker" | "native" |      │
                 │      "unknown", suggest: "..." }            │
                 └──────────────────────┬──────────────────────┘
                                        ↓
             ┌──────────────────────────┴───────────────────────┐
             ↓                                                  ↓
   ┌─────────────────────┐                            ┌──────────────────────┐
   │  Docker path        │                            │  Native path          │
   │  (we detect we're   │                            │  (host runs Go binary │
   │  running in docker, │                            │  directly)            │
   │  ollama is another  │                            │                       │
   │  compose service)   │                            │  ping localhost:11434 │
   └─────────────────────┘                            └──────────────────────┘
             ↓                                                  ↓
             └──────────────────────────┬───────────────────────┘
                                        ↓
                 ┌─────────────────────────────────────────────┐
                 │  internal/llm/ollama.go                     │
                 │  Provider interface — POST /api/generate    │
                 └─────────────────────────────────────────────┘
```

### Detection logic (server-side)

The `GET /api/ai/ollama/status` endpoint returns one of five states with a suggested next action:

| State | Detected by | UI surface |
|-------|-------------|------------|
| `READY` | HTTP 200 from `{host}/api/tags` AND configured model present in response | Green badge "Ready — using {model}". Toggle to disable. |
| `READY_NO_MODEL` | HTTP 200 but model not in `/api/tags` list | "Pull {model} ({size})" button → shells out `ollama pull` with progress stream |
| `INSTALLED_NOT_RUNNING` | `ollama --version` succeeds but `/api/tags` times out | "Start Ollama" button → runs `ollama serve &` in background |
| `NOT_INSTALLED` | `ollama --version` fails with ENOENT | Platform-aware install snippet (copy-to-clipboard) + link to ollama.com/download |
| `DOCKER_SIDECAR_AVAILABLE` | Server runs inside a Docker container AND `compose.yml` has an `ollama` service commented out | "Enable Ollama sidecar" button → writes uncommented `docker-compose.override.yml` + triggers restart guidance |

Deployment detection: the Go server inspects `/.dockerenv` and `DOCKER_SERVICE_NAME` env to decide `deployment: "docker" | "native"`. The override file approach keeps users' primary `docker-compose.yml` untouched.

### Provider selection

Unchanged from existing contract. `config.LLMProvider = "ollama"` activates it. The current auto-select block:

```go
switch {
case cfg.Anthropic.APIKey != "": selectedProvider = "anthropic"
case cfg.OpenAI.APIKey != "":    selectedProvider = "openai"
case cfg.Groq.APIKey != "":      selectedProvider = "groq"
case cfg.Gemini.APIKey != "":    selectedProvider = "gemini"
}
```

is **not extended**. Ollama requires explicit selection (`LLM_PROVIDER=ollama` or UI toggle) — consistent with Q1=C. Rationale: Ollama being "available on localhost" is not a reliable signal that the user wants to use it for every inference (someone running Ollama for other reasons shouldn't have their trading data quietly routed there).

### Failure behaviour

If the Ollama `/api/generate` call returns non-2xx or times out:

1. `ollama.go` returns the error up through `Provider.Complete`.
2. `analyst.Director` logs at WARN level and marks this analyst persona's output as empty.
3. The signal's `ai_interpretation` column stores whatever partial text was generated or an empty string.
4. No retry, no fallback to another provider (Q2=A).

Rationale: a user who deliberately picked local inference would be surprised if their data suddenly went to a cloud provider because Ollama was slow. Silent skipping with a log trail is the expected behaviour.

## Component contracts

### `internal/llm/ollama.go`

```go
type OllamaProvider struct {
    host    string         // e.g. "http://localhost:11434" or "http://ollama:11434" (docker)
    model   string         // e.g. "gemma4:4b"
    timeout time.Duration  // default 120s (local inference can be slow)
    client  *http.Client
}

func NewOllamaProvider(host, model string, timeout time.Duration) *OllamaProvider
func (p *OllamaProvider) Complete(ctx, systemPrompt, userPrompt string) (string, error)
```

Request body (POST `{host}/api/generate`):

```json
{
  "model": "gemma4:4b",
  "prompt": "<system>\n\n<user>",
  "stream": false,
  "options": { "temperature": 0.3, "num_predict": 512 }
}
```

Response parsing: field `"response"` is the generated text. All other fields (done, context, timings) are logged at DEBUG.

### `internal/config/config.go`

Add to `Config`:

```go
type Config struct {
    // ... existing
    Ollama OllamaConfig
}

type OllamaConfig struct {
    Host    string        // default "http://localhost:11434"
    Model   string        // default "gemma4:4b" (user-overridable)
    Timeout time.Duration // default 120s
}
```

Env:
- `OLLAMA_HOST` — overrides `host`
- `OLLAMA_MODEL` — overrides `model`
- `OLLAMA_TIMEOUT_SEC` — overrides `timeout`

YAML (`config/server.yaml`):
```yaml
ollama:
  host: http://localhost:11434
  model: gemma4:4b
  timeout_sec: 120
```

None of these are required unless `LLM_PROVIDER=ollama` is set. The config fields have sensible defaults so a minimal `.env` with just `LLM_PROVIDER=ollama` works out of the box.

### New API endpoints

**`GET /api/ai/ollama/status`** — detection + state machine.

Response:
```json
{
  "state": "READY" | "READY_NO_MODEL" | "INSTALLED_NOT_RUNNING" | "NOT_INSTALLED" | "DOCKER_SIDECAR_AVAILABLE",
  "host": "http://localhost:11434",
  "model": "gemma4:4b",
  "models_available": ["gemma4:4b", "llama3.1:8b", ...],   // only when state starts with READY
  "deployment": "docker" | "native",
  "version": "0.4.2" | null,                               // from `ollama --version`
  "suggest": {
    "action": "pull_model" | "start_ollama" | "install_ollama" | "enable_sidecar" | "none",
    "command": "ollama pull gemma4:4b",                    // platform-specific for install
    "size_bytes": 2600000000                               // only for pull_model
  }
}
```

Auth: `requireBearer` (same as other mutating/privacy-sensitive GETs).

**`POST /api/ai/ollama/pull`** — streams `ollama pull {model}` output.

Request body: `{"model": "gemma4:4b"}`

Response: `text/event-stream` (SSE) with progress lines parsed from Ollama's streaming pull output. Each event is a JSON line `{"status": "pulling...", "completed": 1200000000, "total": 2600000000}`.

Auth: `requireBearer`.

**`POST /api/ai/ollama/start`** — launches `ollama serve &` in background (native path only).

Response: `{"pid": 12345, "started_at": "..."}` or 409 if already running.

Auth: `requireBearer`.

**`POST /api/ai/ollama/sidecar/enable`** — writes `docker-compose.override.yml` to the repo root with the Ollama service definition. Does NOT run `docker compose up` (requires host-level Docker access which the container doesn't have). Returns instructions for the user to run.

Response: `{"override_path": "./docker-compose.override.yml", "run_command": "docker compose up -d ollama"}`.

Auth: `requireBearer`.

### Frontend component

`web/src/OllamaSettings.tsx` — self-contained section that mounts into the existing Settings tab. Polls `GET /api/ai/ollama/status` every 5 seconds when the Settings tab is visible, renders the state-appropriate card, wires up Pull/Start/Enable buttons to their respective POST endpoints.

Model dropdown: free-text input with a datalist of recommended models:
```
- gemma4:4b      (~2.5 GB — fast, default)
- gemma4:12b     (~7 GB — balanced)
- gemma4:27b     (~15 GB — best quality, needs 24+ GB RAM)
- llama3.1:8b    (~4.7 GB — alternative)
```

"Test connection" button: sends a 1-token prompt to whatever model is currently configured and reports latency.

### Docker sidecar

New file `docker-compose.override.yml` (written by the `enable_sidecar` endpoint; not committed by default):

```yaml
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

Committed to the repo as `docker-compose.ollama.yml.template` so the endpoint can copy it to `docker-compose.override.yml`. This keeps the main `docker-compose.yml` unchanged and gives users a clean uninstall path (delete the override file).

## Data flow

### Initial signal → AI interpretation (when Ollama is active)

```
signal detected (pipeline)
    ↓
analyst.Director.Interpret(signal, llmProvider=OllamaProvider)
    ↓
FOR each analyst persona (ICT / Wyckoff / SMC / TA):
    OllamaProvider.Complete(ctx, systemPrompt, userPrompt)
        ↓
    POST http://localhost:11434/api/generate  (or ollama:11434 in docker)
    { model, prompt, stream: false, options: {...} }
        ↓
    parse response.response → return string
    ↓
director concatenates analyst outputs → signals.ai_interpretation
    ↓
UI (Chart / Analysis tab) renders the interpretation
```

### Settings UI lifecycle

```
Settings tab mounts
    ↓
GET /api/ai/ollama/status  (every 5s while visible)
    ↓
render state-specific card
    ↓
user clicks button
    ├─ Pull model        → POST /pull (SSE stream, show progress bar)
    ├─ Start Ollama      → POST /start
    ├─ Enable sidecar    → POST /sidecar/enable (copy-to-clipboard run command)
    └─ Install Ollama    → copy-to-clipboard the platform-specific install command
    ↓
after action, status endpoint reflects the new state on next poll
```

## Error handling

| Situation | Status code | Body | UX |
|-----------|-------------|------|----|
| Ollama not running at configured host | 200 on status (reports state), 503 on generate calls | `{error:"ollama unreachable"}` | Status card updates; AI interpretation skipped for that signal |
| Model not pulled, inference requested | 200 on status, 404 on generate | `{error:"model not found"}` | Status card surfaces "Pull model" CTA |
| Pull command fails (disk full, network) | SSE terminates with error line | `{error:"disk space"}` | Progress bar turns red, user sees the raw error line |
| Start command fails (port in use) | 500 | `{error:"port 11434 in use"}` | Inline error below Start button |
| Sidecar enable when not running under Docker | 400 | `{error:"not running in docker"}` | Button hidden when `deployment=="native"` |

## Testing strategy

**Backend:**
- `internal/llm/ollama_test.go` — table-driven: success (mock Ollama HTTP server), timeout, 404 model-not-found, 503, malformed JSON response. Use `httptest.NewServer`.
- `internal/api/ollama_handler_test.go` — status endpoint state machine (5 states via stubbed detector), pull endpoint SSE stream parsing, start endpoint happy/already-running, sidecar write + idempotency.
- No integration tests hitting real Ollama — that's a manual smoke item in the release checklist.

**Frontend:**
- `web/src/OllamaSettings.test.tsx` — render each of the 5 states; button clicks fire the correct endpoint; polling pauses when tab hidden; progress SSE parsing renders a bar.

**Coverage target:** 80%+ on new backend code, 70%+ on the new component.

## Deferred

Captured in `TODOS.md` under a new "AI / Ollama" section:
- Streaming `stream: true` responses with UI progressive rendering.
- Automatic model pulling on first use (instead of manual button).
- Context cache (`options.context`) reuse across analyst personas for speed.
- Benchmarking harness comparing cloud vs local output quality on a fixed signal corpus.
- Cross-provider fallback policy if users ask for it (explicit opt-in config).

## Implementation order (hint)

1. `internal/llm/ollama.go` + unit tests (provider only, no wiring).
2. `internal/config/config.go` — add `OllamaConfig`.
3. `cmd/server/main.go` — add `case "ollama":` to the provider switch.
4. `GET /api/ai/ollama/status` + detection logic.
5. `POST /api/ai/ollama/pull` (SSE).
6. `POST /api/ai/ollama/start` + sidecar enable.
7. Template file `docker-compose.ollama.yml.template`.
8. `OllamaSettings.tsx` component + Settings tab integration.
9. Docs: `docs/OLLAMA_SETUP.md` with per-platform install commands.
10. CHANGELOG entry + VERSION bump.
