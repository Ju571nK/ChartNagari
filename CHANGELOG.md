# Changelog

All notable changes to this project are documented in this file.

Format:
```
## [version or date] - YYYY-MM-DD
### Added    → new features
### Changed  → changes to existing features
### Fixed    → bug fixes
### Removed  → removed features
### Docs     → documentation changes
```

---

## [2.11.0.0] - 2026-07-06

### Added
- **Zero-install live demo on GitHub Pages.** Try the full dashboard in the browser — no clone, no Docker, no API keys: https://ju571nk.github.io/ChartNagari/. The frontend now ships a static demo mode (`VITE_DEMO_STATIC=true`) that serves every `/api` call from fixtures captured verbatim from the real rule engine (`web/public/demo/scan-{1W,1D,4H,1H}.json`), so the candles and signals shown are authentic detector output. All three READMEs link the demo with a "▶ Live Demo" badge.
- **Demo deploy pipeline.** New `deploy-demo.yml` workflow tests and publishes the static demo to GitHub Pages on every `main` push touching `web/` — actions pinned to commit SHAs, vitest gate before build.
- **Demo endpoint regression tests.** Ungated Go test asserts `/api/demo/scan` returns a valid payload (bars, signals, no future candles) for all four timeframes; 23 new vitest cases cover the demo fetch shim end-to-end plus a fixture contract check.

### Changed
- **Faster first load.** Initial JS dropped 660 kB → 546 kB (gzip 201 → 168 kB): vendor chunks (`react`, `lightweight-charts`, `i18n`) split for parallel download and long-term caching, and heavy tab panels (Analysis, Execution, My Trades, MCP/Ollama settings) now lazy-load on demand with hover prefetch. A chunk-error boundary reloads gracefully when an open tab spans a redeploy.

### Fixed
- **Weekly demo bars no longer live in the future.** `generateDemoBars` walked back a fixed count of days regardless of timeframe, so 50 weekly bars ended ~10 months from now; the last bar of every timeframe now lands at the present.

---

## [2.10.0.2] - 2026-06-07

### Docs
- **Animated README hero.** Replaced the static `docs/demo.svg` placeholder with a real looping `docs/demo.gif` (820px, 2.6 MB) across all three READMEs (en/ko/ja). The loop crossfades the zero-key demo scan ("5 signals detected on sample data") with the full multi-timeframe chart view — overlay markers (Order Block / Spring / Upthrust) plus a live `LONG ... HIGH` signal toast. GitHub strips SVG animation, so a GIF is required for the hero to move.

---

## [2.10.0.1] - 2026-05-25

### Fixed
- **Onboarding footer actions now stack vertically.** "Try demo" and "Set up later" rendered side-by-side and touching (no space between them); they now stack left-aligned with proper spacing, matching the modal's intended layout.

---

## [2.10.0.0] - 2026-05-23

### Added
- **Signal-aware macro annotations.** When a high-impact economic event falls within the alert window of a dispatched signal, the alert now carries a `⚠️ High-impact macro event in Nm: <event> (<country>)` line. Renders on both Telegram and Discord. Fail-open: a calendar DB error never blocks the underlying signal alert. (Completes the final accepted scope item from the economic-calendar CEO review.)
- **`finnhub.alert_window_minutes` setting** (env `CALENDAR_ALERT_WINDOW`, default 30). Drives both the pre-event calendar watcher and the new signal annotations. Validated in `config.Load()` — values outside `[5, 1440]` are clamped to the nearest bound so a typo can't silently disable alerts or schedule them days out.
- **New read-only storage query `GetImminentHighImpact(window)`** — returns high-impact events inside the window, soonest-first, ignoring the `alerted` flag (orthogonal to the watcher's one-shot pre-event alert).
- **Tests**: macro-annotation dispatch + fail-open + formatter coverage (notifier), `GetImminentHighImpact` (storage), alert-window clamp (config), and `GET /api/calendar` handler coverage (200 / empty-array / 500).

### Changed
- `models.Signal` gains a transient `MacroNote` field (set by the Notifier at dispatch, not persisted).
- `notifier.Notifier` gains `WithMacroStore(store, window)`; `*storage.DB` is adapted to the lookup in `cmd/server`.

---

## [2.9.0.0] - 2026-05-09

### Added
- **Signal Performance Tracking.** Mark each fired alert as Took or Skipped, then for Took alerts mark the outcome as Win / Loss / BE. Personal hit rate aggregates per rule, symbol, methodology, and timeframe.
- **Telegram inline keyboard marking.** Alert messages now ship with `[✅ Took / ❌ Skipped]` buttons; tapping Took swaps in `[💰 Win / 💸 Loss / ⚖ BE / ↺ Undo]`. Outcomes append to the message text as a journal: `✓ Took at HH:MM → 💰 Win at HH:MM`.
- **My Trades tab** with three subtabs:
  - *Rollup* — aggregated stats by rule / symbol / methodology / timeframe with selectable period (7d / 30d / 90d / 365d / all).
  - *Pending* — alerts not yet marked, with one-click Took/Skipped buttons.
  - *History* — marked alerts with edit / undo support.
- **Performance tab extension.** New columns: My Took, My Hit Rate, My Skip Rate, Δ vs BT — your personal hit rate side-by-side with the rule's backtest WinRate.
- **New MCP tool `get_my_performance`** (6 tools total). Claude Desktop / Claude Code / Codex CLI can now answer questions like "내 ICT 통계 어때?" with a markdown table of personal stats.
- **New SQLite table `signal_marks`** with FSM-validated transitions and lazy-create semantic (no row = implicit PENDING).
- **New API endpoints**: `POST /api/marks/{signal_id}` (bearer), `GET /api/marks/pending`, `GET /api/marks/recent`, `GET /api/marks/rollup`.

### Changed
- `Signal.ID` field added to `pkg/models/signal.go`. `storage.DB.SaveSignal` now returns `(int64, error)` so the pipeline can capture and propagate the row id to the notifier (required for Telegram callback_data).
- `notifier.NewTelegramBot` now takes a `MarkStore` parameter (use nil to disable callback marking).

### Notes
- Discord receives the same alert text without inline buttons in v1. Discord interaction-based marking is deferred to v2.10.
- Outcome is 3-state (Win/Loss/BE) in v1. Per-trade ±R precision input is reserved for v2.10 (the `outcome_pnl_r` column already exists).
- The user can mark an alert at any time — there is no PENDING timeout. Edits are reversible (`undo` action).

---

## [2.8.1.0] - 2026-05-08

### Fixed
- Per-symbol `cooldown_hours` and `alert_limit_per_day` overrides are now actually consumed by the Notifier — previously the UI stored them but the backend silently ignored them, so users tuning the cooldown slider saw no behavior change. (Resolves the v2.8.0.0 TODO in `internal/pipeline/pipeline.go`.)
- MCP `get_analysis` and `get_signal_history` tools now pass `"ALL"` (not `""`) to `GetSignalsFiltered`'s SQL wildcard, fixing a silent regression where both tools always returned zero signals from the real DB. The fakeSignalSource used in unit tests bypassed SQL, so the bug only manifested in production. New regression test exercises the real `storage.DB` path.

### Added
- New `DailyLimit` tracker in `internal/notifier/daily_limit.go` — UTC-day-bucketed per-symbol counter with idempotent reset at midnight.
- `Cooldown.AllowWithDuration(symbol, rule, dur)` for per-call cooldown durations; the existing `Allow` method is preserved as a global-default wrapper.

---

## [2.8.0.0] - 2026-05-06

### Added
- Per-symbol alert overrides: tune `score_threshold`, `cooldown_hours`, `alert_limit_per_day`, and `timeframes` per symbol from the UI without touching YAML or restarting the server.
- New SQLite table `symbol_alert_overrides` with nullable fields (NULL = inherit from profile).
- New API endpoints: `GET/PUT/DELETE /api/symbol-overrides/{symbol}`.
- New React component `SymbolOverrideEditor` mounted in Symbols tab (row expand chevron) and Chart tab (⚙ modal).
- New optional `Profile.Timeframes` field in `config/symbol_profiles.yaml` (existing YAMLs unaffected).

### Changed
- Pipeline filter chain now consults `EffectiveAlertConfig(symbol)` for each tick — score threshold, timeframe, and override-driven `allowed_rules` overrides take effect on the next signal evaluation, no restart required.

### Notes
- Empty arrays for `timeframes`/`allowed_rules` in PUT bodies are normalized to NULL on disk to avoid silently muting all alerts.
- Per-symbol `cooldown_hours` and `alert_limit_per_day` overrides are computed but NOT yet consumed by the global notifier cooldown tracker — flagged as a follow-up (TODO inline in `internal/pipeline/pipeline.go`).
- `allowed_rules` UI editor is deferred to v1.1 per spec §11.

## [2.7.0.0] - 2026-04-22

### Added
- **MCP (Model Context Protocol) server integration.** Claude Desktop / Claude Code / Codex CLI can query ChartNagari analysis directly, saving ~85% of tokens vs external OHLCV fetches on typical "analyze my watchlist" workflows.
- 5 read-only MCP tools:
  - `list_watchlist` — all tracked symbols with enable flags
  - `get_analysis` — multi-timeframe analysis (fired rules, MTF score, key levels) as markdown
  - `get_signal_history` — recent alerts for a symbol
  - `get_ohlcv` — raw candles (JSON, fallback — prefer `get_analysis` for pattern questions)
  - `get_economic_calendar` — economic events in a date range
- New HTTP streamable endpoint `POST /api/mcp` (bearer-protected, reuses `API_TOKEN`)
- New `chartnagari-mcp` stdio bridge binary for stdio-only clients (Codex CLI)
- `MCPSettings` Settings UI section with ready-to-copy config snippets for all three clients
- User setup guide at `docs/MCP_SETUP.md`

### Technical
- New Go package `internal/mcp` (registry + 5 tool handlers + JSON-RPC envelope types + markdown formatter)
- `internal/api/mcp_handler.go` — streamable HTTP + in-memory session store with 30 min idle TTL
- `cmd/chartnagari-mcp/` — stdio bridge (~130 lines, stdlib only)
- Stdlib-only hand-roll of JSON-RPC 2.0 (SDK deferred — v1 scope is initialize + tools/list + tools/call)
- All endpoints `127.0.0.1`-bound, 1 MiB request cap

### Notes
- `get_analysis` returns markdown tables; `get_ohlcv` returns JSON. Token savings were measured at ~85% vs external OHLCV fetches for typical 10-symbol watchlist workflows.
- MCP session TTL is 30 min idle. Clients must re-initialize after expiry (automatic in Claude Desktop/Code).
- No auto-activation: MCP is served whenever the server runs, but clients must be explicitly configured.

## [2.6.0.0] - 2026-04-20

### Added
- **Ollama local LLM integration** (opt-in). Run Gemma 4 or Llama 3.1 locally via Ollama for privacy and cost reduction. Two setup paths:
  - Docker sidecar (one-click Enable from Settings → copies `docker compose up -d ollama` to clipboard)
  - Native install (macOS / Linux / Windows instructions)
- New `OllamaSettings` component in the Settings tab with a 5-state detection machine (READY, READY_NO_MODEL, INSTALLED_NOT_RUNNING, DOCKER_SIDECAR_AVAILABLE, NOT_INSTALLED), each with a specific CTA
- Streaming progress bar for `ollama pull` via Server-Sent Events (cancel supported)
- Test connection button that performs a 1-token inference and reports latency
- New backend endpoints (all behind `requireBearer`):
  - `GET /api/ai/ollama/status` — detection state machine
  - `POST /api/ai/ollama/pull` — SSE model download
  - `POST /api/ai/ollama/start` — background `ollama serve` launch
  - `POST /api/ai/ollama/sidecar/enable` — write `docker-compose.override.yml`
  - `POST /api/ai/ollama/test` — connection test with latency
- New config keys: `OLLAMA_HOST` (default `http://localhost:11434`), `OLLAMA_MODEL` (default `gemma4:4b`), `OLLAMA_TIMEOUT_SEC` (default 120)
- `ollama` added to `LLM_PROVIDER` options — select to route all trade-signal interpretation through the local model
- i18n support (en/ko/ja) for all Ollama Settings strings
- User-facing setup guide at `docs/OLLAMA_SETUP.md`

### Technical
- New Go package `internal/ollama` (detector + pull runner + starter with clean interface seams for testing)
- New `internal/llm/ollama.go` `OllamaProvider` implements the existing `Provider` interface (stdlib only, no new deps)
- 71 new tests across backend (34 Ollama-specific + 5 test-connection) and frontend (23 OllamaSettings)
- `docker-compose.ollama.yml.template` committed — copied to `docker-compose.override.yml` on sidecar enable

### Notes
- The default model tag `gemma4:4b` assumes Google publishes Gemma 4 on Ollama. If the tag is not yet live, set `OLLAMA_MODEL=gemma3:4b` or similar.
- Ollama is NOT auto-activated when `LLM_PROVIDER` is empty — it must be explicitly selected. Ollama is bypassed in the API-key auto-select fallback.
- When Ollama is configured, the `Test connection` endpoint is available regardless of the active `LLM_PROVIDER`, so users can validate connectivity before switching providers.

---

## [2.5.0.0] - 2026-04-18

### Added
- **Execution tab (React UI)** (`web/src/ExecutionTab.tsx` + 5 component files) — first UI surface for the execution plugin system shipped in v2.3–v2.4. Top-level tab with four regions: kill switch (confirmation modal + red banner with `killed_at` timestamp), plugin cards (24h SUBMITTED/FILLED/REJECTED counts with last-failure tooltip), global config form (max_dispatched, dedup_window, symbol_map editor), and filtered feedback table (plugin/status/symbol filters, 100-row default).
- **`GET /api/execution/feedback`** (`internal/api/execution_handler.go`) — lists recent feedback with plugin/status/symbol filters and a `limit` bound (0..500, default 100). Queries the new `symbol`/`message` columns on `feedback_idempotency`.
- **`GET /api/execution/plugins/stats?window=24h`** — per-plugin 24h aggregation (SUBMITTED/FILLED/REJECTED counts + most recent failure message). Emits `Cache-Control: max-age=60`.
- **Config version field** on `PUT /api/execution/config` — request must include the last-read `version`; mismatch returns 409 with `current_version` so a concurrent edit surfaces as a banner instead of silently overwriting. Mutex-serialized: 10 concurrent PUTs produce exactly 1 OK + 9 CONFLICT, eliminating the TOCTOU race.
- **`execution_state` SQLite table** — generic key-value runtime state. Houses `killed_at` (moved out of YAML — kill switch state now survives restarts via DB, not git-tracked config) and `config_version`.
- **Secret `Generate` UX** — in-modal button produces a 32-byte hex secret via `crypto.getRandomValues` and auto-copies to clipboard; falls back to a `Copy manually` toast when clipboard is unavailable (HTTP-served remote sessions).
- **Stale-rows regression guard** — FeedbackTable applies the v2.4.0.2 `setFeedback([])` pattern on filter change so no previous-filter rows linger during the new fetch. Same pattern locked in with a filtersRef pattern so the 30s polling timer does not tear down and restart on each filter change.
- **`requireBearer` helper** (`internal/api/auth.go`) — constant-time bearer-token comparison for GET endpoints that the global method-gated middleware bypasses.
- **`TestNoSecretLeakInExecutionEndpoints`** — blanket regression that asserts raw plugin secrets never appear in any `/api/execution/*` response body or header, seeded with a sentinel and verified across GET config / feedback / plugins/stats.

### Changed
- `feedback_idempotency` schema — added `symbol` and `message` columns (NOT NULL DEFAULT '' so pre-existing rows remain valid after migration). New index `idx_feedback_received_at` supports the 24h aggregation query.
- `OrderFeedback` wire format (`pkg/models/trade_signal.go`) — added optional `Symbol` field. Alpaca adapter now echoes `TradeSignal.Symbol` in feedback callbacks so the feedback table can show which symbol each order corresponded to.
- `FeedbackIdempotency.RecordOnce` signature extended with `symbol` and `message` arguments.
- Kill switch state — `killed_at` moved from `config/execution.yaml` to the new `execution_state` SQLite key-value table.

### Database
- New table `execution_state(key PRIMARY KEY, value NOT NULL, updated_at NOT NULL)`.
- New columns on `feedback_idempotency`: `symbol TEXT NOT NULL DEFAULT ''`, `message TEXT NOT NULL DEFAULT ''`.
- New index `idx_feedback_received_at ON feedback_idempotency(received_at)` for the 24h plugin-stats aggregation.

### Tests
- Backend: new tests in `internal/execution/state_test.go`, `internal/api/execution_handler_test.go` (15+ new cases covering feedback listing, plugin stats, config version gate + concurrent-writes race-free contract, kill state persistence, secret-leak blanket test). Backend coverage on new handlers ~80%. Full suite `-race` clean.
- Frontend: new Vitest files `ExecutionTab.test.tsx`, `KillSwitch.test.tsx`, `PluginCard.test.tsx`, `FeedbackTable.test.tsx`, `PluginEditModal.test.tsx`, `GlobalConfigForm.test.tsx` — 43 tests total.

### i18n
- Added ~40 new keys across en/ko/ja for the Execution tab (field labels, validation errors, status filters, column headers, kill banner, secret toasts, duplicate-name error, config-conflict banner).

### Deferred (tracked in TODOS.md)
- WebSocket feedback push (Phase 5).
- Playwright E2E infrastructure.
- Detailed dispatcher metrics (active positions, avg latency, dedup skip count).

### Manual verification checklist (post-merge smoke)
- [ ] Execution tab renders with at least one plugin card after server start.
- [ ] Kill switch → confirm modal → red banner with last-killed timestamp.
- [ ] Re-enable → banner clears.
- [ ] Edit plugin → change `min_score`, save, reopen modal → value persists.
- [ ] Generate secret → toast + clipboard receives hex.
- [ ] Two-tab 409: edit in tab A, save, then save in tab B → banner appears, modal closes, tab B can reload and retry.
- [ ] FeedbackTable filter change → rows clear synchronously, then repopulate.

---

## [2.4.0.2] - 2026-04-14

### Fixed
- **History tab shows stale rows after filter change** (`web/src/App.tsx` HistoryTab) — same stale-state pattern as the chart fix in `2.4.0.1`: when the user changed the symbol/direction/limit filter, the table kept the previous filter's rows visible until `/history` resolved, so "SHORT selected" could briefly show LONG rows. Fix: `setSignals([])` synchronously at the start of the `load` callback before the fetch. Reproduced and verified with `/browse`: after the patch, switching direction shows an empty table + "Loading..." during the in-flight request.

---

## [2.4.0.1] - 2026-04-13

### Fixed
- **Chart shows stale candles after symbol/timeframe switch** (`web/src/App.tsx`) — when switching symbols (e.g. BTCUSDT → TSLA) or timeframes the chart kept rendering the previous symbol's candlesticks, volume bars, and price axis until the new OHLCV fetch resolved (~30ms locally, longer on remote networks). The selector label updated immediately, producing a confusing "TSLA selected with BTC 70000+ price axis" state. Fix: clear `seriesRef.setData([])`, `volRef.setData([])`, and `setSignals([])` synchronously at the start of the load effect, before issuing the fetch. The Wyckoff effect was patched the same way (`setWyckoffData(null)`) so the phase badge no longer carries the previous symbol's label during the in-flight `/wyckoff/{symbol}/{tf}` request.

---

## [2.4.0.0] - 2026-04-13

### Added
- **Alpaca paper trading plugin adapter** (`cmd/plugin-alpaca/`, `internal/plugins/alpaca/`) — first reference execution plugin. Standalone Go binary that receives ChartNagari `TradeSignal` webhooks, translates them into Alpaca paper-trading market orders, and POSTs `OrderFeedback` callbacks back to `/api/execution/feedback`.
- **Webhook endpoint** (`/webhook`) — verifies inbound HMAC-SHA256 signatures using `internal/execution.Verify` directly (single source of truth — no canonical-string drift between dispatcher and adapter); enforces the same `±300s` skew window as the dispatcher (configurable via `ALPACA_TIMESTAMP_SKEW_SEC`).
- **Paper-only hard guard** (`Config.Validate`) — refuses to start unless `ALPACA_API_URL` resolves to `paper-api.alpaca.markets` (loopback hosts allowed only for tests). The live Alpaca endpoint is rejected at startup; this is a deliberate operational safety rail.
- **Direction mapping** (`internal/plugins/alpaca/mapper.go`) — `LONG → buy`, `SHORT → sell` market orders with `time_in_force=day`; quantity sized as `floor(notional / entry_price)` from `ALPACA_NOTIONAL_PER_TRADE` (default $1000). Crypto signals are rejected (Alpaca's crypto API requires a different endpoint contract).
- **SQLite idempotency store** (`internal/plugins/alpaca/idempotency.go`) — `INSERT OR IGNORE` against `UNIQUE(signal_id)` eliminates double-submits across restarts; `Reserve` returns `ErrDuplicate` (HTTP 409) for repeat signal_ids; `MarkSubmitted` records the resulting Alpaca order_id for diagnostics.
- **Outbound feedback** (`internal/plugins/alpaca/feedback.go`) — async POSTs `OrderFeedback{SUBMITTED|REJECTED|ERROR}` signed with `internal/execution.Sign` so the dispatcher's `Verify` accepts the response without any signing-format duplication.
- **Error response taxonomy** — `400` bad JSON, `401` HMAC/skew/unknown plugin_id, `405` wrong method, `409` duplicate signal_id, `422` mapping reject or Alpaca 4xx, `502` Alpaca 5xx, `500` persistence failure.
- **Env-var configuration** (`config.go`) — `ALPACA_API_URL`, `ALPACA_API_KEY`, `ALPACA_API_SECRET`, `CHARTNAGARI_FEEDBACK_URL`, `CHARTNAGARI_PLUGIN_SECRET`, `CHARTNAGARI_PLUGIN_ID`, `LISTEN_ADDR` (default `:9100`), `ALPACA_DB_PATH`, `ALPACA_NOTIONAL_PER_TRADE`, `ALPACA_TIMESTAMP_SKEW_SEC`. Sidecar deployment surface — no YAML, no hot reload.
- **Runner lifecycle** (`internal/plugins/alpaca/runner.go`) — `NewRunner` validates config + opens store; `Start(ctx)` blocks on `http.Server.Serve` with `ReadHeaderTimeout=5s` and graceful `Shutdown` on `SIGINT`/`SIGTERM` (5s drain).
- **Example plugin entry** in `config/execution.yaml` (commented) showing how to register the Alpaca sidecar at `http://127.0.0.1:9100/webhook` for local smoke testing.

### Tests
- `internal/plugins/alpaca/` at **87.5% coverage**, all `-race` clean. Tables cover: config validation (paper-only guard, loopback allow-list, IPv6 host parsing, env fallbacks), mapper direction/quantity/asset-class rejection, idempotency reserve/release/duplicate, Alpaca client 2xx/4xx/5xx + transport error paths, feedback HMAC headers + non-2xx propagation, server end-to-end (HMAC fail, skew, duplicate, mapping reject, Alpaca 4xx/5xx, success path), runner Start→Shutdown lifecycle and listen-error.

---

## [2.3.1.0] - 2026-04-12

### Added
- **Execution dispatcher** (`internal/execution/dispatcher.go`) — fan-out goroutine per eligible plugin with per-request HMAC signing, bounded 500ms timeout (default 10s), one retry on non-2xx, atomic `ActiveCount` gated by `max_dispatched` cap. Kill switch and `enabled=false` short-circuit before any HTTP I/O.
- **HMAC-SHA256 signing** (`internal/execution/hmac.go`) — canonical string `plugin_id\ntimestamp\nmethod\npath\nhex(sha256(body))`; `Sign`/`Verify` use constant-time compare; `WithinSkew` enforces inclusive ±300s (configurable) on inbound feedback timestamps.
- **SQLite-backed dedup** (`internal/execution/dedup.go`) — single-statement `INSERT OR IGNORE` against `UNIQUE(key, bucket)` eliminates TOCTOU; bucket key normalizes symbol+direction to upper/trim; `SQLITE_BUSY` fails closed.
- **Feedback idempotency store** — `UNIQUE(plugin_id, signal_id, order_id, status)` prevents replay; distinct statuses for the same signal (ACK → FILLED) remain independent rows.
- **Dedup cleaner** (`internal/execution/cleanup.go`) — background goroutine deletes rows older than `max(2*window, 2m)` on a 1-minute tick.
- **API endpoints** (`internal/api/execution_handler.go`) — `GET /api/execution/config` (secrets redacted), `PUT /api/execution/config` (merge preserves unchanged secrets on empty/masked input), `POST /api/execution/kill` (disk-first persist then memory flip), `POST /api/execution/feedback` (HMAC-verified, terminal statuses free capacity via `Release()`, duplicates return 409).
- **Pipeline integration** (`internal/pipeline/pipeline.go`) — `SetExecutionDispatcher` hook dispatches enriched signals post-notify via `models.ToTradeSignal` conversion.
- **Server wiring** (`cmd/server/main.go`) — loads `config/execution.yaml` (zero-value fallback if missing), constructs dedup store + dispatcher + feedback idempotency + cleaner, binds to pipeline and API server.
- **Default execution config** (`config/execution.yaml`) — fully disabled out of the box with commentary on every field.
- **WebSocket hub subprotocol auth** — client type filtering with HMAC-authenticated subprotocol on `/ws` connections.

### Database
- New tables `execution_dedup` (UNIQUE(key, bucket), cleanup index on `dispatched_at`) and `feedback_idempotency` (UNIQUE(plugin_id, signal_id, order_id, status), lookup index on `(plugin_id, signal_id)`).

### Tests
- Coverage on `internal/execution/` at 82.2% across dispatcher (T1–T14), HMAC canonical format, dedup window/normalization, cleanup TTL flooring, and API handlers (redaction, secret preservation, kill persistence, replay 409, skew, terminal/non-terminal release semantics). Full suite runs `-race` clean.

---

## [2.3.0.0] - 2026-04-12

### Added
- **TradeSignal envelope** (`pkg/models/trade_signal.go`) — outbound webhook payload wrapping internal `Signal`; injects uuid/timestamp/version/asset_class/exchange at dispatch time. JSON wire format frozen against `pkg/models/testdata/trade_signal_v1.json`.
- **OrderFeedback model + status enum** (RECEIVED/SUBMITTED/FILLED/PARTIAL_FILL/REJECTED/CANCELLED/ERROR) for plugin callbacks.
- **Execution plugin config** (`internal/config/execution.go`) — `ExecutionConfig`/`PluginConfig`, YAML load with zero-value fallback on missing file, atomic save (temp → fsync → rename → fsync parent dir) so kill-switch toggles are durable before the in-memory flag flips.
- `ExecutionHolder.SetKillSwitch` persists disk before updating memory, `RedactSecrets`/`MergeIncomingSecrets` helpers keep plugin secrets out of API GETs while allowing rotation on PUT.
- Validation rejects empty plugin id/url, non-http(s) url, duplicate ids, negative counters, `direction_filter` outside `{"", "LONG", "SHORT"}`, and empty `symbol_map` keys; normalizes plugin `symbols` and `symbol_map` keys/values to uppercase.

### Changed
- `go.mod`/`go.sum` — `github.com/google/uuid` promoted from indirect to direct dependency (used by `ToTradeSignal`); `go mod tidy` also promoted other already-direct deps misfiled as indirect.

---

## [2.2.3.0] - 2026-04-09

### Security
- **CORS restriction** — replace wildcard `*` with localhost-only allowlist (ports 5173/8080), add `Vary: Origin`
- **Localhost binding** — server defaults to `127.0.0.1` instead of `0.0.0.0`; opt-in via `SERVER_HOST=0.0.0.0`
- **Bearer token auth** — optional `API_TOKEN` protects all mutating endpoints (POST/PUT/PATCH/DELETE); backward-compatible when unset
- **SQLite restore validation** — verify magic bytes + `PRAGMA integrity_check` before replacing live database

---

## [2.2.2.0] - 2026-04-07

### Added
- **VIX data collection** — ^VIX index collected via Yahoo Finance (1D, indices section in watchlist)
- **Coiled market detection** — realized vol vs VIX comparison detects compressed markets (breakout imminent)
- **Realized volatility indicator** — 20-period annualized log-return volatility
- **VIX Status widget** — Status tab shows current VIX, 20d average, realized vol, ratio, coiled status
- **Chart VIX overlay** — toggle VIX as LineSeries in bottom 20% of chart pane
- **Coiled Market settings** — enable/disable, ratio threshold, bonus % in Signal Tuning
- **Signal DB infra** — htf_trend and atr_percentile recorded on every signal (foundation for auto-calibration)

## [2.2.1.0] - 2026-04-07

### Added
- **Forward return tracking** — auto-compute 5/10/20/40 day returns for historical signals
- **Beginner/Expert mode** — Settings toggle simplifies UI for new users (hides overlays, tuning, scores)
- **Wyckoff relative strength** — accumulation scoring compares symbol vs SPY benchmark
- **Backtest equity curve** — cumulative return chart (LineSeries, starting capital 10000)
- **Filter impact report** — shows signal count reduction at each pipeline filter stage
- **Continuous gradient HTF penalty** — smooth formula `base * (1 - scaling * atr_pct)` replaces 3-bucket
- **Regime-dependent HTF penalty** — LOW_VOL 70%, NORMAL 50%, HIGH_VOL 30% (configurable)
- **Signal CSV export** — `GET /api/signals/export` with 12 columns
- **DB backup/restore** — download/upload SQLite with .bak safety backup
- **Settings export/import** — portable config bundle as JSON
- **Data Management section** in Settings tab
- **Market research doc** — Koventium competitive analysis

### Changed
- HTF penalty supports both gradient mode (default) and legacy 3-bucket mode via Settings toggle

## [2.2.0.0] - 2026-04-07

### Added
- **3 new trading rules:** ICT Optimal Trade Entry (0.618-0.786 Fib zone), VSA Effort Candle (Stopping Volume, No Demand, No Supply), ICT AMD Session Structure (Asia-London-NY manipulation detection). Total rules: 33
- **ADX trend strength indicator** integrated into HTF context filter for continuous trending/ranging classification
- **ATR percentile regime classification** — signals tagged as LOW_VOL/NORMAL/HIGH_VOL based on 90-bar ATR history
- **ATR slope transition detector** — rising ATR EMA triggers volatility expansion bonus
- **Volume Profile signal integration** — HVN/LVN/POC proximity adjusts scores for sweep, FVG, and OB signals
- **Per-symbol signal profiles** — crypto/large_cap/small_cap presets with different rule sets per symbol
- **Signal Tuning UI** in Settings — configurable HTF penalty, vol regime thresholds, ATR slope parameters
- **Chart overlay toggles** — FVG zones, Order Block zones, Wyckoff phase zones as dotted price line pairs
- **Backtest regime performance breakdown** — per-rule stats split by LOW_VOL/NORMAL/HIGH_VOL at entry time
- **BacktestTab market filter** — crypto/stock/exchange toggle buttons
- **Demo mode improvements** — multi-TF sample data with Wyckoff cycle, 5+ signals
- **Signal CSV export** — `GET /api/signals/export?format=csv` with 12 columns
- **DB backup/restore** — download SQLite, upload replacement with .bak safety backup
- **Settings export/import** — bundle all config YAMLs as JSON
- **Data Management section** in Settings tab with backup/restore/export/import UI
- **DESIGN.md** — full design system documentation
- **Market research** — Koventium competitive analysis

### Changed
- **HTF filter: score reduction instead of binary suppress** — configurable 0-100% penalty (default 50%) instead of removing counter-trend signals entirely
- **Wyckoff phase overrides HTF filter** — accumulation in bearish EMA allows LONG through at reduced score
- **Wyckoff phase detection moved earlier in pipeline** — reused by both HTF filter and phase boost (no duplicate computation)
- **restart.sh** — quick/frontend/backend/full modes for faster development workflow

### Fixed
- Chart marker plugin reuse prevents stale markers on filter toggle
- Unified marker effect prevents Wyckoff overlay from overriding category filter
- Wyckoff overlay button renamed to "W.Phase" to avoid duplicate labels

## [2.1.4.0] - 2026-03-31

### Added
- Signal quality scoring for liquidity sweeps: volume ratio, wick ratio, and reversal strength produce a 0.1-1.0 score instead of fixed 1.0
- Sweep-vs-breakout classifier: 3+ confirmation bars after a sweep suppress false signals where price continues through the level
- FVG relevance filter: gap size vs ATR, impulse candle strength, and unfilled duration score each Fair Value Gap
- Volume profile indicator: 20-bin price distribution computes POC, HVN, and LVN levels per timeframe
- Order block mitigation tracking: OB zones revisited and closed through are excluded from future signals
- Order block impulse strength filter: requires 1.5x ATR combined body to qualify
- Chart signal category filter: ICT / Wyckoff / SMC / TA toggle buttons with localStorage persistence
- Community feedback improvements doc (`docs/community-feedback-improvements.md`)

### Changed
- Chart markers deduplicated per candle: same time + direction keeps only the highest-scoring signal
- Chart marker opacity reflects quality score (HIGH >= 0.7, MED 0.4-0.7, LOW < 0.4)

### Fixed
- Wyckoff overlay useEffect overriding category filter by rebuilding markers from unfiltered signals
- BacktestTab TypeScript error: `SymbolItem[]` type mismatch and unused `marketFilter` state

## [2.1.3.0] - 2026-03-28

### Added
- **OnboardingModal** (`web/src/OnboardingModal.tsx`) — 2-step guided setup modal shown to
  first-time users; step 1 adds a symbol via `POST /api/symbols`, step 2 runs a full
  analysis scan via `POST /api/analysis/full` with 60-second abort timeout; completion
  sets `chartnagari_onboarding_done` in localStorage so it never shows again
- Alert channel status banner in step 2 — checks `GET /api/settings/config` on mount and
  shows ok/warning/unknown badge depending on whether Telegram or Discord is configured
- AI scenario card on completion — bull/bear/sideways probability bars from scan result;
  falls back to "technical signals only" label when LLM service returns 503
- Twitter/X share button on completion with confirm dialog before opening external link
- Vitest test suite for the frontend (`web/src/OnboardingModal.test.tsx`) — 15 tests
  covering render, step transitions, alert banners, scan results, timeout, localStorage,
  skip, ESC close, and share dialog
- `web/src/test-setup.ts` — in-memory localStorage mock for vitest/jsdom environments
- i18n keys added to `en.json`, `ko.json`, `ja.json` for all onboarding strings

### Changed
- `web/vite.config.ts` — added vitest configuration (jsdom environment, globals, setup file)
- `web/package.json` — added vitest, @testing-library/react, @testing-library/user-event,
  @testing-library/jest-dom, and jsdom as devDependencies

---

## [2.1.2.0] - 2026-03-23

### Added
- **Wyckoff phase analyzer** (`internal/wyckoff/`) — you can now detect Markup,
  Accumulation, Distribution, Markdown, and Ranging phases from OHLCV bars using
  EMA-50 and swing high/low breakout logic; Spring and Upthrust events are identified
  and returned alongside phase zones and current swing levels
- **Wyckoff API endpoint** (`GET /api/wyckoff/{symbol}/{timeframe}`) — fetches up to
  1000 bars and returns the full overlay payload for the frontend chart
- **ChartTab Wyckoff overlay** — toggle button to show/hide Wyckoff phase zones as
  semi-transparent price bands with phase labels, plus Spring (Sp) and Upthrust (Ut)
  event markers merged with existing signal markers
- **BacktestTab trade chart** — Lightweight Charts candlestick overlay renders entry
  (arrow) and exit (circle) markers for each backtest trade; clicking a row in the trade
  list scrolls the chart to that trade's entry bar

### Fixed
- `TradeOutcome.entry_time`: corrected frontend type from `number` to `string` and parse
  path from `/ 1000` to `new Date().getTime() / 1000` — eliminates NaN timestamp bug
  that produced invisible/misplaced trade markers
- Wyckoff `useEffect` dependency array now includes `signals` — prevents stale closure
  showing previous symbol's markers when switching symbols with overlay enabled
- `internal/wyckoff/analyzer.go`: removed local `max()` helper that shadowed Go 1.21+
  built-in
- `data/tiingo_state.json` removed from git tracking (was committed before gitignore rule)

---

## [2.1.1.0] - 2026-03-22

### Added
- `.github/workflows/validate-new-rules.yml`: CI workflow that auto-runs on PRs touching
  `internal/methodology/**` — builds the package and runs tests with the race detector,
  giving contributors instant feedback without requiring a label
- `.github/ISSUE_TEMPLATE/new-rule.yml`: Structured GitHub issue form for proposing new
  ICT/Wyckoff/TA rules, with dropdowns for category, acceptance-criteria checklist, and
  auto-applied `new-rule` / `good first issue` labels
- `.github/PULL_REQUEST_TEMPLATE.md`: Universal PR template with new-rule checklist and
  general PR checklist (items not applicable can be marked N/A)
- Compile-time interface assertion in `internal/methodology/ict/order_block.go`:
  `var _ rule.AnalysisRule = (*ICTOrderBlockRule)(nil)` — turns a missing-method silent
  compile into an instant build error

### Changed
- `CONTRIBUTING.md`: Complete rewrite — fixed interface name (`rule.AnalysisRule`),
  added "Ways to Contribute (No Code Required)" section, step-by-step new-rule guide
  with code examples, compile-time-assertion rationale, and updated PR checklist
- `README.md`: Added "What You Get in 5 Minutes" quick-start summary, "Why ChartNagari"
  competitive landscape table, and `LLM_LANGUAGE` i18n entry; fixed interface reference
  from `rule.Rule` to `rule.AnalysisRule`

### Fixed
- `data/tiingo_state.json` added to `.gitignore` — runtime state file was being accidentally
  tracked; it now stays out of version control

---

## [0.1.0] - 2026-03-07

### Docs
- 프로젝트 초기 문서 세트 생성
  - `CLAUDE.md` : 프로젝트 진입점
  - `CHANGELOG.md` : 이 파일

### Research
- ICT (Order Block, FVG, Liquidity Sweep, Breaker Block, Kill Zone) — 사전 검증 완료
- Wyckoff (Accumulation, Distribution, Spring, Upthrust, Volume Anomaly) — 사전 검증 완료
- 일반 기술적분석 (RSI, S/R, EMA Cross, Fibonacci, Volume Spike) — 사전 검증 완료

---

## [2.1.0] - 2026-03-22

### Added
- FMP (Financial Modeling Prep) as alternative economic calendar provider — dual-provider support; FMP preferred when `FMP_API_KEY` is set, Finnhub used as fallback
- `CALENDAR_ALERT_WINDOW` config setting — configurable pre-event alert window (default 30 min); settable via Settings UI
- 19 new tests: `internal/calendar/fetcher_test.go` (10), `internal/calendar/watcher_test.go` (5), `internal/storage/calendar_test.go` (4)
- LinkedIn badge added to README (`## Builder` section)

### Changed
- `internal/calendar/fetcher.go`: refactored to dual-provider architecture; added fixed backoff retry (1 min → 5 min, 2 retries per 6h cycle); `finnhubBaseURL`/`fmpBaseURL` fields for test injection
- `internal/calendar/watcher.go`: in-memory `alerted map[int64]struct{}` prevents duplicate Telegram alerts when `MarkEventAlerted` DB write fails; `alertWindow` now configurable via `NewWatcher`
- `internal/config/config.go`: added `FMPConfig`, `FMP FMPConfig` to `Config`; added `AlertWindowMinutes int` to `FinnhubConfig`; `ToMap`/`ApplyMap` updated for `FMP_API_KEY` and `CALENDAR_ALERT_WINDOW`
- `internal/api/server.go`: `FMP_API_KEY` added to `envSensitiveKeys`; `FINNHUB_API_KEY`, `FMP_API_KEY`, `CALENDAR_ALERT_WINDOW` added to `envExposedKeys`
- `cmd/server/main.go`: calendar init updated for dual-provider; `alertWindow` passed to `NewWatcher`
- `web/src/App.tsx`: CalendarTab empty state now explains paid API requirement with links; event times shown in browser local timezone (`toLocaleTimeString`); calendar tab label uses `t('calendar')` i18n key; Settings "Economic Calendar" group with FMP_API_KEY, FINNHUB_API_KEY, CALENDAR_ALERT_WINDOW fields
- `web/src/i18n/locales/{en,ko,ja}.json`: added `"calendar"` translation key

### Fixed
- `internal/storage/db.go`: added `PRAGMA busy_timeout = 5000` after WAL mode — prevents `SQLITE_BUSY` errors when Fetcher, Watcher, and Pipeline goroutines write concurrently
- `internal/calendar/fetcher.go`: Finnhub API key moved from `?token=` URL query param to `X-Finnhub-Token` HTTP header — prevents key exposure in access logs and request URLs

---

## [2.0.0] - 2026-03-21

### Added
- `internal/calendar/fetcher.go`: Finnhub 경제 캘린더 Fetcher — 6시간마다 향후 14일 미국 경제지표 데이터 자동 수집, SQLite 캐시
- `internal/calendar/watcher.go`: 경제 캘린더 Watcher — 1분 주기 체크, 고영향 이벤트 35분 전 Telegram/Discord 사전 알림
- `internal/storage/db.go`: `economic_events` 테이블 + `UpsertEconomicEvents`, `GetEconomicEvents`, `GetUpcomingAlerts`, `MarkEventAlerted`, `scanEconomicEvents` 메서드
- `GET /api/calendar` 엔드포인트: `from`/`to` 쿼리 파라미터 지원, 기본 ±14일
- React `CalendarTab` 컴포넌트: 날짜별 그룹, 고/중/저 영향도 뱃지, 실제값 vs 예측값 표시, 5분 자동 새로고침
- `config/settings.example.yaml`: `finnhub.api_key` 항목 추가 (무료 계정으로 사용 가능)

### Changed
- `internal/config/config.go`: `FinnhubConfig` struct + `SettingsYAML.Finnhub` 필드 + `ToMap`/`ApplyMap` 매핑
- `internal/api/server.go`: `CalendarStore` 인터페이스 + `WithCalendarStore` setter + `/api/calendar` 라우트
- `cmd/server/main.go`: `calendar.New()` + `calendar.NewWatcher()` 초기화, Finnhub API 키 설정 시 goroutine 실행, `apiSrv.WithCalendarStore(db)` 연결
- `web/src/App.css`: `.calendar-date-header`, `.calendar-event`, `.calendar-impact-*`, `.calendar-actual`, `.calendar-forecast` CSS 추가

---

## [1.9.0] - 2026-03-21

### Added
- `internal/hub/hub.go`: WebSocket 허브 패키지 — 다중 클라이언트 연결 관리, ping/pong keepalive, `Broadcast(type, payload)` 메서드
- `GET /ws` 엔드포인트: 브라우저 WebSocket 업그레이드 처리
- React LIVE 인디케이터: 헤더에 `● LIVE` / `○ --` 연결 상태 표시
- React 시그널 토스트: 새 신호 발생 시 우하단 슬라이드인 알림 (6초 후 자동 소멸, 클릭 닫기)
- `/api/status` 응답에 `ws_clients` (현재 연결 수) 필드 추가

### Changed
- `internal/pipeline/pipeline.go`: `SignalBroadcaster` 인터페이스 + `SetBroadcaster` + AI 해석 완료 후 즉시 브로드캐스트
- `internal/api/server.go`: `WSHub` 인터페이스 + `WithHub` setter + `/ws` 라우트 등록
- `cmd/server/main.go`: `hub.New()` 초기화, `go wsHub.Run()`, 파이프라인·API서버 연결
- `web/src/App.css`: `.ws-indicator`, `.ws-toast`, `@keyframes slideIn` CSS 추가

---

## [1.8.0] - 2026-03-21

### Added
- `internal/methodology/candlestick/`: 캔들스틱 패턴 인식 패키지 (14개 룰, 44개 테스트)
  - 단일봉: `DojiRule`, `HammerRule`, `HangingManRule`, `ShootingStarRule`, `InvertedHammerRule`, `MarubozuRule`
  - 2봉: `BullishEngulfingRule`, `BearishEngulfingRule`, `BullishHaramiRule`, `BearishHaramiRule`
  - 3봉: `MorningStarRule`, `EveningStarRule`, `ThreeWhiteSoldiersRule`, `ThreeBlackCrowsRule`
- `internal/pricealert/watcher.go`: 가격 목표 알림 Watcher (파이프라인 tick마다 조건 체크 → Telegram/Discord 발송)
- `internal/storage/db.go`: `price_alerts` 테이블 + CRUD 5개 메서드 (`AddPriceAlert`, `ListPriceAlerts`, `GetActivePriceAlerts`, `MarkAlertTriggered`, `DeletePriceAlert`)
- `config/settings.example.yaml`: `.env` 대체 YAML 설정 파일 템플릿

### Changed
- `internal/config/config.go`: `SettingsYAML` 구조체 추가, `Load()` 우선순위 OS환경변수 > settings.yaml > .env > 기본값
- `internal/api/server.go`: `GET/POST/DELETE /api/price-alerts` 엔드포인트, `.env` → `settings.yaml` 기반 설정 API, 하위호환 `/api/env/config` 라우트 유지
- `internal/pipeline/pipeline.go`: `PriceAlertWatcher` 인터페이스 + `SetPriceAlertWatcher` + `analyzeSymbol`에 가격 체크 연결
- `cmd/server/main.go`: candlestick 룰 14개 등록, pricealert.New() 초기화 연결
- `config/rules.yaml`: candlestick 룰 14개 항목 추가
- `web/src/App.tsx`: "가격 알림" 탭 추가 (en/ko/ja i18n, 30초 자동 갱신)
- `.gitignore`: `config/settings.yaml` 추가

---

## [1.5.0] - 2026-03-08

### Added
- `internal/paper/trader.go`: 실시간 페이퍼 트레이딩 엔진 (PaperTrader, PaperPosition, PaperSummary)
- `internal/paper/trader_test.go`: 10개 테스트 PASS (오픈/중복방지/제로진입/TP/SL/구바 무시/룰필터/요약/Long-Short레벨/멀티심볼)
- `internal/storage/paper.go`: DB CRUD (SavePaperPosition, GetOpenPositions, GetAllOpenPositions, ClosePaperPosition, GetClosedPositions)
- `internal/storage/db.go`: `paper_positions` 테이블 + 인덱스 스키마 추가

### Changed
- `internal/pipeline/pipeline.go`: PaperTrader 인터페이스 + SetPaperTrader + analyzeSymbol에 OnSignals/CheckPositions 연결
- `internal/api/server.go`: PaperStore 인터페이스 + GET /api/paper/positions, /history, /summary 엔드포인트
- `cmd/server/main.go`: paper.New() 초기화 + pipe.SetPaperTrader + apiSrv.WithPaperStore
- `web/src/App.tsx`: PaperTab 컴포넌트 (요약 카드 6개 + 오픈 포지션 테이블 + 청산 히스토리 테이블)

---

## [1.4.0] - 2026-03-08

### Added
- `internal/collector/tiingo.go`: Tiingo REST 수집기 (1D/1W = daily endpoint, 1H/4H = IEX intraday endpoint)
- `internal/config/config.go`: TiingoConfig 추가 (TIINGO_API_KEY, TIINGO_POLL_INTERVAL)
- `.env.example`: TIINGO_API_KEY, TIINGO_POLL_INTERVAL 항목 추가

### Changed
- `cmd/server/main.go`: TIINGO_API_KEY 설정 시 Tiingo 수집기 우선 사용, 미설정 시 Yahoo fallback
- PRD.md: Phase 2-5 방향 → "Yahoo → Tiingo 대체"로 업데이트

---

## [1.3.0] - 2026-03-08

### Added
- `pkg/models/signal.go`: EntryPrice, TP, SL 필드 추가 (ATR 기반 거래 레벨)
- `internal/pipeline/pipeline.go`: enrichSignalLevels() — 신호 발생 시 진입가/TP/SL 자동 계산
- `internal/notifier/format.go`: fmtPrice() 헬퍼 + formatTelegram에 💰 진입/TP/SL 라인 추가
- `internal/notifier/discord.go`: Discord embed fields에 진입가/TP/SL 항목 추가
- `internal/notifier/notifier_test.go`: 포맷 테스트 3개 추가 (WithLevels, NoLevelsWhenZero, ContainsFields)
- `docs/research/20260308_free_data_sources.md`: 무료 데이터 소스 VERIFIED 리서치 (Tiingo 1순위 권고)

### Research
- 무료 데이터 소스 조사 완료: Tiingo(1순위) > Polygon.io(주식 전용) > Alpha Vantage(낮은 한도) → VERIFIED

---

## [1.2.0] - 2026-03-08

### Changed
- AGENTS.md v0.3: TraderAdvisor 에이전트 추가 (실전 트레이더 자문 역할)
- PRD.md: Phase 2-5 Bloomberg → `[BLOCKED]` (유료 API 계약 불가)
- Orchestrator 트리거: VERIFIED 기법 → TraderAdvisor 실전 코멘트 연결
- Orchestrator 트리거: 새 UI 기능 → TraderAdvisor 유용성 검토 추가

---

## [1.1.0] - 2026-03-08

### Added
- `internal/backtest/engine.go`: 슬라이딩 윈도우 룰 재실행 엔진 (ATR 기반 TP/SL 시뮬레이션)
- `internal/backtest/stats.go`: ComputeStats — 승률, 평균손익비, 수익팩터, MDD, 샤프비율, 누적수익률, 최대연속손실
- `internal/backtest/runner.go`: OHLCVLoader 인터페이스 + Runner (스토리지 + 엔진 통합)
- `internal/backtest/engine_test.go`: 10개 테스트 PASS (Empty, InsufficientBars, LongTP, ShortTP, LongSL, Timeout, Filter, Stats, StatsEmpty, MaxDrawdown)
- `internal/storage/ohlcv.go`: GetOHLCVAll — 전체 바 오름차순 조회 (백테스트 전용)
- `internal/api/server.go`: BacktestRunner 인터페이스 + WithBacktestRunner + `POST /api/backtest` 핸들러
- `web/src/App.tsx`: BacktestTab 컴포넌트 (설정 폼, 통계 카드 6개, 거래 목록 테이블)
- `web/src/App.css`: 백테스트 탭 스타일 (.backtest-controls, .run-btn, .backtest-stats, .backtest-table)

### Changed
- `cmd/server/main.go`: allRules 슬라이스 도입 (룰 엔진 + 백테스트 엔진 공유), BacktestEngine/Runner 연결
- PRD.md: Phase 2-4 → `[DONE]`
- docs/STATUS.md: 팀 상태 갱신

---

## [1.0.0] - 2026-03-08

### Added
- `internal/storage/signals.go`: SaveSignal, GetSignals — 신호 영속성
- `internal/pipeline/pipeline.go`: SignalSaver 인터페이스 + SetSignalSaver + 신호 자동 저장
- `internal/api/server.go`: ChartStore 인터페이스 + `GET /api/ohlcv/{symbol}/{tf}` + `GET /api/signals` 엔드포인트
- `web/src/App.tsx`: ChartTab 컴포넌트 (TradingView Lightweight Charts v5, 캔들차트 + 신호 마커)
- `web/src/App.css`: 차트 컨트롤 스타일 (chart-controls, tf-group, chart-area) + .badge-smc
- `lightweight-charts` npm 패키지 (v5.1.0)

### Changed
- `internal/storage/db.go`: signals 테이블 스키마 추가
- `cmd/server/main.go`: DB → API WithChartStore, Pipeline SetSignalSaver 연결
- PRD.md: Phase 2-3 → `[DONE]`

---

## [0.9.0] - 2026-03-08

### Added
- `internal/methodology/smc/helpers.go`: trendDir, structuralHigh, structuralLow 공통 헬퍼
- `internal/methodology/smc/bos.go`: SMCBOSRule — Break of Structure (추세 지속 신호)
- `internal/methodology/smc/choch.go`: SMCChoCHRule — Change of Character (추세 전환 신호)
- `internal/methodology/smc/smc_test.go`: SMC 패키지 테스트 14개 전체 PASS
- `config/rules.yaml`: smc_bos (strength: 5.0), smc_choch (strength: 6.0) 항목 추가
- `cmd/server/main.go`: SMCBOSRule, SMCChoCHRule 룰 엔진 등록

### Changed
- PRD.md: Phase 2-2 → `[DONE]`
- docs/STATUS.md: 팀 상태 갱신

---

## [0.3.0] - 2026-03-07

### Added
- `internal/config/config.go`: .env + YAML 통합 설정 로더
- `internal/storage/db.go`: SQLite 초기화, WAL 모드, 스키마 마이그레이션
- `internal/storage/ohlcv.go`: OHLCV CRUD (SaveOHLCV, SaveOHLCVBatch, GetOHLCV, GetOHLCVSince)
- `internal/collector/binance.go`: Binance WebSocket 수집기 (자동 재연결, 확정 캔들만 저장)
- `internal/collector/yahoo.go`: Yahoo Finance REST 수집기 (장중/장외 시간 구분)
- `internal/collector/timeframe.go`: 1H → 4H/1D/1W 자동 재구성 유틸리티
- `cmd/server/main.go`: 수집기 goroutine 연결, SIGTERM graceful shutdown
- 테스트 11개 (collector 6, storage 5) — 전체 PASS

### Changed
- PRD.md: 1-1, 1-2, 1-3 → `[DONE]`
- 의존성 추가: `modernc.org/sqlite`, `gorilla/websocket`, `yaml.v3`, `godotenv`

---

## [0.2.0] - 2026-03-07

### Added
- Go 프로젝트 디렉토리 구조 생성 (`cmd/`, `internal/`, `pkg/`, `config/`, `web/`)
- `go.mod` 초기화 (`github.com/Ju571nK/Chatter`, Go 1.26)
- `cmd/server/main.go` — zerolog 구조화 로깅 포함 서버 진입점
- `pkg/models/signal.go` — 공유 데이터 모델 (`Signal`, `OHLCV`, `AnalysisContext`)
- `internal/rule/interface.go` — `AnalysisRule` 플러그인 인터페이스 정의
- `config/rules.yaml` — 전체 룰 설정 파일 (모든 룰 비활성 상태로 초기 세팅)
- `config/watchlist.yaml` — 모니터링 종목 설정 (BTC, ETH 활성화)
- `Dockerfile` — 멀티 스테이지 빌드 (builder + alpine 런타임)
- `docker-compose.yml` — SQLite 볼륨 마운트 + 헬스체크 포함
- `.env.example` — 환경변수 템플릿
- `.gitignore` — `.env`, 바이너리, SQLite 데이터 제외

### Changed
- PRD.md: Phase 0 → `[DONE]`, Phase 1 → `[IN PROGRESS]`

## [0.4.0] - 2026-03-07

### Added
- `internal/indicator/` 패키지 — 인디케이터 엔진 (Phase 1-4)
  - `indicator.go`: `Compute(bars map[string][]OHLCV) map[string]float64` — 전체 TF 인디케이터 일괄 계산, 키 형식 `"{TF}:{지표명}"` (예: `"1H:RSI_14"`)
  - `rsi.go`: RSI(14) — Wilder's smoothing
  - `ema_sma.go`: EMA(9/20/50/200), SMA(20/50/200), VolumeMA(20)
  - `macd.go`: MACD(12,26,9) — line/signal/histogram
  - `bb.go`: Bollinger Bands(20, 2σ) — upper/middle/lower/width/%B
  - `obv.go`: OBV (누적 거래량 방향 지표)
  - `atr.go`: ATR(14) — Wilder's smoothing
  - `swing.go`: Swing High/Low (lookback=5)
  - `fibonacci.go`: Fibonacci 7레벨 (0/23.6/38.2/50/61.8/78.6/100%)
  - `indicator_test.go`: 14개 테스트 — 전체 PASS
- `internal/engine/` 패키지 — 룰 엔진 (Phase 1-5)
  - `config.go`: `RuleConfig`, `RuleEntry`, `TFWeight()` (1W=2.0/1D=1.5/4H=1.2/1H=1.0)
  - `engine.go`: `RuleEngine` — Register/Run, RequiredIndicators 검증, Score=룰점수×TF가중치×룰가중치, 내림차순 정렬
  - `engine_test.go`: 10개 테스트 — 전체 PASS

### Changed
- PRD.md: Phase 1-4, 1-5 → `[DONE]`
- 전체 테스트: 25개 PASS (기존 11개 유지 + 신규 14개)

---

## [0.5.0] - 2026-03-07

### Added
- `internal/methodology/general_ta/` 패키지 — 일반 기술적분석 플러그인 (Phase 1-6)
  - `helpers.go`: 패키지 내부 유틸리티 (`rollingRSI`, `rollingEMA`, `swingLowPair`, `swingHighPair`)
  - `rsi_overbought_oversold.go`: RSI(14)≥70 → SHORT, ≤30 → LONG, 전 TF 스캔
  - `rsi_divergence.go`: 가격/RSI 다이버전스 감지 (강세/약세), rollingRSI 내부 계산
  - `ema_cross.go`: EMA(9)/EMA(20) 골든크로스·데드크로스 감지
  - `support_resistance_breakout.go`: SWING_HIGH/LOW 돌파 감지
  - `fibonacci_confluence.go`: 가격이 주요 피보나치 레벨(0.5% 허용오차) 근처일 때 신호
  - `volume_spike.go`: 거래량 2×MA20 초과 시 방향 신호
  - `general_ta_test.go`: 18개 테스트 — 전체 PASS
- `internal/methodology/ict/` 패키지 — ICT 방법론 플러그인 (Phase 1-8)
  - `order_block.go`: 마지막 약세/강세 캔들 → 충격파 → 가격 복귀 시 신호
  - `fair_value_gap.go`: 3캔들 불균형 갭(FVG) 감지, 가격이 갭 진입 시 신호
  - `liquidity_sweep.go`: 스윙 레벨 위/아래 위크 돌파 후 복귀 — 유동성 스윕 신호
  - `breaker_block.go`: 실패한 오더블록(브레이커) 감지 — 반대 방향 신호
  - `kill_zone.go`: 런던(08:00-11:00 UTC) / 뉴욕(13:00-16:00 UTC) 킬존 시간 감지
  - `ict_test.go`: 15개 테스트 — 전체 PASS
- `internal/methodology/wyckoff/` 패키지 — Wyckoff 방법론 플러그인 (Phase 1-9)
  - `accumulation.go`: 좁은 레인지(<8%) + EMA50 하단 + 낮은 거래량 → LONG
  - `distribution.go`: 좁은 레인지 + EMA50 상단 + 낮은 거래량 → SHORT
  - `spring.go`: 스윙저점 아래 위크 후 복귀 + 고거래량 → LONG
  - `upthrust.go`: 스윙고점 위 위크 후 반전 + 고거래량 → SHORT
  - `volume_anomaly.go`: 거래량 2.5×MA20 초과 → 방향 신호
  - `wyckoff_test.go`: 14개 테스트 — 전체 PASS

### Changed
- `config/rules.yaml`: 16개 룰 모두 `enabled: true` 활성화 (구현 완료)
- `PRD.md`: Phase 1-6, 1-8, 1-9 → `[DONE]`
- 전체 테스트: 82개 PASS (기존 35개 유지 + 신규 47개)

---

## [0.6.0] - 2026-03-07

### Added
- `internal/notifier/` 패키지 — Telegram/Discord 알림 시스템 (Phase 1-7)
  - `notifier.go`: `Notifier` — 스코어 임계값 필터, 쿨다운 검사, 멀티 Sender 디스패치
  - `cooldown.go`: `Cooldown` — `{symbol}|{rule}` 키 기반 in-memory 쿨다운 (기본 4시간)
  - `format.go`: `formatTelegram`, `discordColor`, `directionIcon` 메시지 포매터
  - `telegram.go`: `TelegramSender` — Bot API `/sendMessage`, HTML parse_mode
  - `discord.go`: `DiscordSender` — Webhook embed 메시지 (컬러 코딩: 녹색/빨간/황색)
  - `notifier_test.go`: 18개 테스트 — 전체 PASS (httptest.Server로 실제 HTTP 검증 포함)

### Design
- `Sender` 인터페이스로 백엔드 교체/확장 가능 (Slack 등 추후 추가 용이)
- HTTP 클라이언트 주입 가능 → 테스트에서 실제 API 호출 없음
- 쿨다운 시계(`now` func) 주입 가능 → 만료 테스트 가능

### Changed
- 전체 테스트: 100개 PASS (기존 82개 유지 + 신규 18개)

---

## [0.7.0] - 2026-03-07

### Added
- `internal/api/` 패키지 — Go REST API 서버 (Phase 1-10)
  - `server.go`: `Server` — 5개 엔드포인트, YAML 파일 읽기/쓰기, CORS 미들웨어
    - `GET /api/status` — 시스템 요약 (phase, symbols, rules, tests)
    - `GET /api/symbols` — 전체 종목 목록 (crypto + stock)
    - `PUT /api/symbols/{symbol}` — 종목 enabled 토글
    - `GET /api/rules` — 전체 룰 목록
    - `PUT /api/rules/{name}` — 룰 enabled 토글
    - `GET /` — React 정적 파일 서빙 (web/dist/)
  - `api_test.go`: 16개 테스트 — 전체 PASS
- `web/` — React + TypeScript 설정 UI
  - `vite.config.ts`: 개발 모드에서 `/api/*` → Go 서버 프록시
  - `src/App.tsx`: 3탭 UI (종목 / 룰 / 상태), 실시간 토글 반영
  - `src/App.css`: 다크 테마, 방법론별 컬러 배지
  - `npm run build` → `web/dist/` 빌드 성공 (27 모듈, 148KB JS)

### Changed
- `cmd/server/main.go`: HTTP API 서버 goroutine 추가 (port `:8080`)
- `PRD.md`: Phase 1-10 → `[DONE]`, Phase 1 전체 → `[DONE]`
- 전체 테스트: 116개 PASS (기존 100개 유지 + 신규 16개)

### 🎉 Phase 1: Core MVP 완료

## [0.8.0] - 2026-03-08

### Added
- `internal/interpreter/` 패키지 — Claude AI 해석 레이어 (Phase 2-1)
  - `interpreter.go`: `Interpreter` — `New(apiKey, minScore, clientOpts...)`, `Enrich(ctx, []SignalGroup) []Signal`
  - SignalGroup 총 스코어 ≥ minScore 일 때만 Claude API 호출 (비용 절감)
  - API 키 미설정 시 자동 비활성화 (파이프라인 전체 정상 동작 유지)
  - 모델: `claude-opus-4-6` / max_tokens: 600 / 언어: 한국어 200자
  - 오류 시 원본 신호 그대로 반환 (Graceful degradation)
  - `interpreter_test.go`: 7개 테스트 — 전체 PASS (httptest.Server 기반)
- `internal/pipeline/` 패키지 — 분석 파이프라인 (Phase 2-1)
  - `pipeline.go`: SQLite → 인디케이터 → 룰 엔진 → AI 해석 → 알림 전체 연결
  - `OHLCVReader` 인터페이스로 DB 의존성 분리 (테스트 용이)
  - 1분 간격 ticker, 심볼별 독립 처리 (한 심볼 실패가 다른 심볼에 영향 없음)
  - `pipeline_test.go`: 6개 테스트 — 전체 PASS

### Changed
- `pkg/models/signal.go`: `Signal`에 `AIInterpretation string` 필드 추가
- `internal/notifier/format.go`: AI 해석이 있으면 Telegram 메시지 끝에 `💡 <i>해석</i>` 추가
- `internal/notifier/discord.go`: AI 해석이 있으면 embed description에 `💡 해석` 추가
- `internal/config/config.go`: `AnthropicConfig{APIKey, MinScore}` 추가 + `parseFloat` 헬퍼
- `cmd/server/main.go`: 룰 엔진 플러그인 전체 등록, Notifier/Interpreter 초기화, 파이프라인 goroutine 시작, `toEngineConfig()` 변환 함수
- `.env.example`: `ANTHROPIC_API_KEY`, `AI_MIN_SCORE` 추가
- 의존성 추가: `github.com/anthropics/anthropic-sdk-go v1.26.0`
- 전체 테스트: 129개 PASS (기존 116개 유지 + 신규 13개)

### Design
- 룰 엔진 (마이크로초, 무료, 24/7) → AI (레이턴시, 유료, 선택) 1차/2차 필터 구조
- `ANTHROPIC_API_KEY` 미설정 = Phase 1 동작과 100% 동일 (하위 호환)

---

<!-- 이후 항목은 Recorder가 자동으로 추가한다 -->

## [0.11.0] - 2026-03-13

### Added
- MTF 합의 필터 — 동일 방향 신호가 N개 TF에서 발생할 때만 알림/페이퍼 진입 (기본값 2)
  - `internal/pipeline/filter_test.go` — 5개 테스트 PASS
  - `config/alert.yaml` — 알림 설정 파일 신규 생성
- 알림 설정 웹 UI '알림' 탭 — 스코어 임계값, 쿨다운, MTF 합의 수 실시간 변경
  - `GET/PUT /api/alert/config` 엔드포인트
- Quant 에이전트 팀 합류 — 신호 품질 분석 + 정량 파라미터 설계 담당 (AGENTS.md v0.4)

### Changed
- `internal/config/config.go` — AlertConfig, AlertConfigHolder 추가
- `internal/pipeline/pipeline.go` — MTFConsensusMin 동적 적용, filterMTFConsensus 함수
- `internal/notifier/notifier.go` — ScoreThreshold 동적 읽기 (AlertConfigHolder)
- `internal/api/server.go` — 알림 설정 API, alertHolder 연동
- `cmd/server/main.go` — alertHolder 생성 및 전체 와이어링

### Fixed
- 단일 TF 역추세 신호 남발 문제 → MTF 합의 필터로 해결 (페이퍼 트레이딩 승률 33.3% 개선 예상)

## [0.10.0] - 2026-03-13

### Added
- 신호 히스토리 탭 — 종목·방향·건수 필터로 과거 신호 테이블 조회
  - `GET /api/history?symbol=ALL&direction=ALL&limit=100` 엔드포인트
  - `GetSignalsFiltered` 스토리지 메서드
- 백테스트 TP/SL ATR 배율 수동 입력 (기본값 TP×2.0 / SL×1.0)
  - POST /api/backtest에 `tp_mult`, `sl_mult` 파라미터 추가
  - 결과 헤더에 적용된 배율 표시

### Changed
- `internal/backtest/engine.go` — `Clone`, `NewWithConfig` 추가
- `internal/backtest/runner.go` — RunBacktest 시그니처 변경 (tpMult, slMult)
- `internal/api/server.go` — SignalBar에 symbol 필드 추가, 인터페이스 갱신
- `web/src/App.tsx` — HistoryTab 컴포넌트, 히스토리 탭, BacktestTab TP/SL 입력 추가

## [0.9.0] - 2026-03-13

### Added
- 주식 전용 일일 리포트 기능 (Research: `docs/research/20260312_daily_stock_report_discussion.md`)
  - `internal/report/daily.go` — 리포트 생성 로직 (신호 집계, 종가, Telegram 포맷)
  - `internal/report/scheduler.go` — KST 기반 cron 스케줄러 (time.AfterFunc, Reset 지원)
  - `internal/report/daily_test.go` — 14개 테스트 PASS
  - `config/report.yaml` — 일일 리포트 설정 파일 (enabled, time, timezone, ai_min_score 등)
  - `GET /api/report/config`, `PUT /api/report/config` — 웹 UI 연동 REST 엔드포인트
  - 웹 UI '리포트' 탭 — 6개 설정 필드 + 저장 버튼 + 저장 성공 플래시

### Changed
- `internal/config/config.go` — `DailyReportConfig` 구조체 추가, report.yaml 로딩
- `internal/storage/` — `GetSignalsByDate` 메서드 추가
- `internal/api/server.go` — 리포트 설정 API + `WithReportScheduler` 연동
- `cmd/server/main.go` — 리포트 스케줄러 고루틴 등록
- `web/src/App.tsx` — ReportTab 컴포넌트, '리포트' 탭 추가
- `web/src/App.css` — `.report-field`, `.report-input`, `.save-success` 클래스 추가

### Docs
- `docs/queue/APPROVED_daily_stock_report.md` — Owner 승인 스펙 (2026-03-13)
- `docs/pending/PENDING_daily_stock_report.md` — 승인 완료로 삭제