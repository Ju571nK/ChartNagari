# Ollama Local LLM Setup

ChartNagari can use [Ollama](https://ollama.com) to run a local LLM for trade-signal interpretation instead of calling cloud APIs like Anthropic or OpenAI.

## Why local?

- **Privacy** — market data, analyst prompts, and interpretations never leave your machine.
- **Cost** — no per-token billing.
- **Offline** — works without internet once the model is pulled.

Downsides: requires disk space (2.6 GB for the default `gemma4:4b` model), meaningfully slower than cloud APIs, and inference quality is lower than flagship models.

## Choose a path

Two setup paths. Both produce the same result. **Docker sidecar is easiest** and doesn't require installing Ollama on your host OS.

### Path A — Docker sidecar (recommended)

Requirements: Docker Desktop or Docker Engine with `docker compose` v2.

1. Open ChartNagari → Settings → AI Provider (Ollama).
2. Click **Enable Docker sidecar**. The button copies the run command to your clipboard and shows:
   ```
   docker compose up -d ollama
   ```
3. Paste the command in your terminal at the ChartNagari repo root. Docker starts the `chart-ollama` container.
4. Back in Settings, the status pill will flip to **Docker sidecar available → Ready, model not pulled** within 30 seconds.
5. Click **Pull model** to download `gemma4:4b` (~2.6 GB). A progress bar tracks the pull.
6. When complete, the pill shows **Ready — model loaded**. Click **Test connection** — you should see "OK (XXX ms)".
7. In Settings → AI / LLM, set **LLM Provider** to `ollama` and save. Restart ChartNagari.

Uninstall: delete `docker-compose.override.yml` and run `docker compose up -d`.

### Path B — Native install

#### macOS
```bash
brew install ollama
ollama serve  # leave this running
```

Open a second terminal:
```bash
ollama pull gemma4:4b
```

#### Linux
```bash
curl -fsSL https://ollama.com/install.sh | sh
# systemd starts ollama automatically
ollama pull gemma4:4b
```

#### Windows
Download the installer at https://ollama.com/download/windows. After install:
```powershell
ollama pull gemma4:4b
```

Back in ChartNagari, Settings → AI Provider (Ollama) should show **Ready — model loaded**. Set LLM Provider to `ollama` and restart.

## Configuration

Settings that affect Ollama (set via Settings UI or environment):

| Setting | Default | Purpose |
|---|---|---|
| `OLLAMA_HOST` | `http://localhost:11434` | Ollama server URL. For Docker sidecar, set to `http://ollama:11434`. |
| `OLLAMA_MODEL` | `gemma4:4b` | Model tag. Other options: `gemma4:12b` (~7.2 GB), `gemma4:27b` (~15 GB), `llama3.1:8b` (~4.7 GB). |
| `OLLAMA_TIMEOUT_SEC` | `120` | HTTP timeout for inference. Raise on slow hardware. |

## Troubleshooting

### Port 11434 already in use
Another Ollama instance is running. Either stop it (`ollama ps` then kill), or change `OLLAMA_HOST` to a different port in Settings.

### Model not found
`ollama pull <model>` must have completed successfully before Test connection. In the Settings UI, click Pull model again.

### Slow inference (>30s per signal)
Small hardware often can't run larger models. Try `gemma4:4b` if you're on `gemma4:12b`. For Apple Silicon, ensure Ollama is using Metal (enabled by default on macOS).

### Docker compose doesn't see the override
Run `docker compose config` to verify the override is loaded. The `docker-compose.override.yml` must exist at the repo root.

### Settings panel says "Ollama detector not configured"
The server isn't wired for Ollama status. Restart ChartNagari — the endpoints are registered on every boot when `OLLAMA_HOST` is set.

### Test connection returns 500
Check Ollama logs: `docker logs chart-ollama` (Docker path) or `journalctl -u ollama -f` (Linux) or `ollama serve` stdout (macOS). Usual causes: out-of-memory, model download incomplete, or binding conflict.

## Uninstall

### Docker
```bash
rm docker-compose.override.yml
docker compose down ollama
docker volume rm chartter_data_ollama  # deletes the model
```

### Native
```bash
ollama rm gemma4:4b  # or your chosen model
# macOS: brew uninstall ollama
# Linux: sudo systemctl stop ollama && sudo systemctl disable ollama
```

Then in ChartNagari Settings, change LLM Provider away from `ollama` or set it to empty.
