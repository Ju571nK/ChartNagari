# Web-managed YAML settings

Application configuration is stored in `config/settings.yaml`, not continuously
read from `.env`. Open **Settings** in the web UI to edit server/database, market
data providers, notification credentials, AI/Ollama, MCP bridge and Alpaca paper
adapter settings. Alert timing/risk settings remain in `config/alert.yaml` and
are editable in **Settings → Alerts** or the existing Alerts page.

For local/remote server profiles, remote API authentication and allowed app
origins, see [Local and remote connections](docs/remote-connections.md). Remote
access is opt-in and requires an administrator token and HTTPS deployment.

## AI connection setup

The AI setup wizard is available during onboarding and in the AI settings tab.
Choose the **ChartNagari installation device** or a remote model server, select a
model preset or enter a compatible model identifier, and save a connection
profile. The installation device is the machine running the Go server, which
can differ from the browser device. A remote URL must be reachable from that
server; a localhost URL refers to the server or its container.

Ollama profiles support listing installed models and downloading a model on the
selected server. Compatible Hugging Face GGUF identifiers use
`hf.co/owner/repository[:quantization]`; this is not support for every Hub model
architecture or format. OpenAI-compatible profiles connect to an existing
inference server. Use the API base URL including `/v1` when the server requires
it (for example `http://model-host:8000/v1`). They do not install or provision
that remote server.

After preparation, run the sample response test and review its output and
latency, then explicitly activate the profile. Saving a profile alone does not
switch the active AI. Changing its connection/model requires testing again.
Activation applies to analysis without restarting and is restored on restart.
The existing provider configuration remains the fallback until a profile is
activated. A local inference failure must not silently switch to a paid service.

Presets are convenient starting points, not measured performance guarantees.
Memory usage depends on the model, quantization and context size. Public model
weights do not imply free hosted inference or unrestricted licensing; consult
the linked model card. Laya is shown separately as a typed-decision model and
is not currently activatable as the text-generation provider. Its dedicated
classification integration remains future work.

Profile credentials are stored in `config/ai_profiles.json` (mode `0600`) on the
server and omitted from API responses (only a saved-key indicator is returned).
An empty key field preserves the saved key; use the explicit clear action to
remove it. Changing a keyed endpoint requires a new key or clearing the old one.
Protect the profile file and backups as credentials; never commit them. Model
installation can require large downloads. Select the target and model before
starting a download; cancel or retry from the wizard when needed.

## Existing installations

On the first start of the updated server, settings without `version: 1` import
the previous effective values once: process environment, then `.env`, then the
existing YAML. Only recognized application keys are imported. This does not
modify the process environment or delete `.env`. All later starts use YAML
plus built-in defaults, so clearing a key in the web UI stays cleared even if
an old `.env` value exists. Do not remove the version marker.

To migrate without starting collectors, notifications or order execution:

```sh
go run ./cmd/server --migrate-settings-only
```

Run this from the project directory with the old deployment environment. If a
sidecar had a separate environment, enter its credentials in the web UI before
restarting it. Do not overwrite an existing settings file with the example.

## New installations

Copy `config/settings.example.yaml` to `config/settings.yaml`, restrict it to
owner read/write, and start the server. The example already has `version: 1`;
it intentionally ignores environment overrides. Default native binding is
`127.0.0.1:8080`. Initial deployment paths/binding must be reachable before the
browser can configure the application.

Docker no longer requires `.env`. Before Docker startup, set `server.host` to
`0.0.0.0`, `server.port` to `"8080"`, and `database.path` to
`./data/chart_analyzer.db` in YAML. Keep the host port mapping loopback-only.
For Ollama in another container, set its reachable URL in YAML too.

## Saving and applying

- General settings are persisted, not hot-reloaded. Restart the server to apply.
- MCP bridge and Alpaca adapter are independent processes; restart each separately.
- Saving Alpaca credentials does **not** start the adapter, enable execution,
  disable the kill switch, register an execution plugin or submit an order.
- Plugin IDs/secrets must match the separate `execution.yaml` registration.
- Database path changes select another database; they do not copy existing data.
- Changing server host/port or API token changes how to reconnect after restart.
- A blank password display means “configured” when accompanied by its placeholder.
  Untouched keys remain unchanged; **Clear** schedules explicit removal on Save.
- Settings writes are atomic and mode `0600`. The YAML still contains plaintext
  secrets on disk: restrict access, protect backups and do not commit it.
- Existing API bearer authentication still applies to saves. Do not expose an
  unauthenticated server to the internet.
- If authentication is enabled, enter the **current** API token at the top of
  Settings and select **Authorize changes**. It is kept in tab memory only,
  attached only to same-origin API calls and forgotten on reload or **Forget token**.
  Changing the saved server token takes effect after restart; authorize again
  with the new token then. This field does not change the server's token.

Standalone clients accept an explicit absolute path:

```sh
chartnagari-mcp --settings /absolute/path/to/Chartter/config/settings.yaml
plugin-alpaca --settings /absolute/path/to/Chartter/config/settings.yaml
```

The web API retains flat legacy key names for compatibility, while persistence
uses YAML sections. `clients` holds standalone client keys. Test-only variables
(`CHARTTER_TEST_YAHOO_LIVE`, `CHARTTER_USABILITY_PREVIEW`, `CAPTURE_DEMO`) are not
user configuration and remain environment-controlled.

The Codex snippet uses a named MCP table and `args` for the YAML path, following
the [official MCP configuration documentation](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

## Verification (2026-09-12)

- Full Go suite, config/API/Alpaca race tests, frontend 156 tests and production build passed.
- Chrome isolated fixture: database-path save/reload, polling interval save,
  disposable provider-key masking and explicit deletion verified. No provider,
  notifier or execution adapter was attached to the fixture.
- Native local server updated on loopback port 8080. `/health`, settings API and
  actual Chrome settings page verified. Watchlist, alert and execution YAML
  hashes were unchanged; execution remained disabled with no plugins.
- Original local settings were backed up with mode `0600` to
  `bin/settings-before-yaml-20260912.yaml` (ignored by Git); `.env` was retained.
  Active process uses `bin/chartter-server-yaml-auth`; log is
  `logs/server-yaml-auth-20260912.log`.
- No real orders, test notifications, or Alpaca account calls were performed.
  Docker configuration was updated but Docker deployment was not exercised.
- The manual fixture reached its old 10-minute timeout after browser checks;
  its lifetime was shortened to 9 minutes to leave cleanup headroom. This fixture
  is opt-in and skipped in the passing normal test suite.
