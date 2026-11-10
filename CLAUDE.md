# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Summary

ChartNagari is a local-run platform that automatically detects ICT/Wyckoff/SMC and general TA signals
across multiple timeframes (1W/1D/4H/1H) for US stocks and crypto, and sends alerts via Telegram/Discord.
Go backend (Go 1.26+, SQLite) + TypeScript/React 18/Vite frontend.

The Go module path is `github.com/Ju571nK/Chatter` (not ChartNagari/Chartter) — use it in imports.

## Commands

```bash
# Backend
go run ./cmd/server                          # API + pipeline on 127.0.0.1:8080
go test ./...                                # all Go tests (CI also runs `go vet ./...`)
go test ./internal/methodology/ict -run TestOrderBlock -v   # single test
go test ./internal/methodology/... -v -race  # what CI runs on rule PRs
make build | build-all | test | lint | run   # binary is ./chart-analyzer

# Frontend (in web/)
npm run dev                                  # Vite on :5173, proxies /api and /ws to :8080
npm run build                                # tsc + vite build — this is the only type check/lint
npx vitest run                               # all tests once (`npm test` starts watch mode)
npx vitest run src/ExecutionTab.test.tsx     # single test file

# Docker
./restart.sh [quick|frontend|backend]        # no arg = full rebuild; quick = restart only (YAML changes)
```

New installs need `cp config/settings.example.yaml config/settings.yaml && chmod 600 config/settings.yaml`.
Never overwrite an existing `config/settings.yaml` (gitignored, holds plaintext secrets).

## Architecture

Per symbol, per tick (`internal/pipeline`):

```
collectors → SQLite OHLCV → indicator.Compute → engine.Run (rules) → interpreter (MTF scoring, optional LLM)
           → storage (signals) / notifier (Telegram, Discord) / hub (WebSocket) / paper trader / execution dispatcher
```

- `cmd/server/main.go` is the single wiring point: it builds collectors, the rule list, pipeline,
  API server and MCP tool registry. Optional pipeline dependencies are attached with `Set*` methods,
  and the pipeline depends on small interfaces that `*storage.DB` satisfies.
- **Collectors** (`internal/collector`): Binance WebSocket for crypto; Tiingo (if key set) or Yahoo for
  stocks; index symbols like `^VIX` always use Yahoo daily.
- **Rules**: each implements `rule.AnalysisRule` (`Name`, `RequiredIndicators`, `Analyze`; return
  `nil, nil` for no signal). `engine` evaluates only rules enabled in `config/rules.yaml`.
- **Shared types** live in `pkg/models` (`Signal`, `OHLCV`, `AnalysisContext`, `TradeSignal`).
- **API** (`internal/api`) serves REST + the built frontend from `web/dist`, and persists settings to YAML.
- **Frontend chart overlays are separate from backend signals**: FVG/OB candidates drawn on the chart
  are computed in the browser (`web/src/chartPatterns.ts`) from the visible candles, not from the engine.
- **Separate processes**: `cmd/chartnagari-mcp` (MCP bridge) and `cmd/plugin-alpaca` (Alpaca paper
  order adapter). Saving their settings in the UI does not start them or enable execution.
- **Order plugins**: `internal/execution` dispatches HMAC-signed, deduplicated trade signals to external
  plugins registered in `config/execution.yaml`. The public SDK is `pkg/brokerplugin` (must not import
  `internal/`); contract in `docs/api/order-plugin-v1.openapi.yaml`, guides in `docs/order-plugins/`,
  conformance via `cmd/plugin-check`.

### Configuration

`config/settings.yaml` (general settings + credentials, edited through the web Settings tab) is the
source of truth. `.env` is legacy: it is imported once on migration and ignored afterwards
(`version: 1` marker — do not remove). General settings are **not hot-reloaded**; restart to apply.
Watchlist, rules, alerts, signal tuning, symbol profiles and execution each have their own YAML in
`config/`. See `SETTINGS.md`.

## Adding a Trading Rule

1. Create `internal/methodology/<category>/<rule>.go` (categories: `ict`, `wyckoff`, `smc`,
   `general_ta`, `candlestick`), with `var _ rule.AnalysisRule = (*MyRule)(nil)`.
   Reference implementation: `internal/methodology/ict/order_block.go`.
2. Add it to the `allRules` slice in `cmd/server/main.go` (shared with the backtest engine).
3. Add an entry to `config/rules.yaml` whose `name` matches `Name()`.
4. Table-driven tests in the category's `_test.go`.

CI fails if `CONTRIBUTING.md` does not mention every method of `rule.AnalysisRule` — update it when
changing `internal/rule/interface.go`.

## Conventions

- Run `go test ./...` before submitting any code change — all tests must pass. For frontend changes
  also `cd web && npx vitest run` and `npm run build`.
- zerolog structured logging (log messages in English), context propagation, table-driven tests.
- User-facing frontend strings go through i18n: add keys to all three of
  `web/src/i18n/locales/{en,ko,ja}.json`.
- Update `CHANGELOG.md` under `## [Unreleased]`. `VERSION` is 4-part (`MAJOR.MINOR.PATCH.MICRO`).

## Agent Team

This project uses a multi-agent team architecture. The `orchestrator` agent coordinates 4 specialists:

- **go-backend** — Go code (cmd/, internal/)
- **react-frontend** — React/TS code (web/src/)
- **trading-analyst** — Trading rule design & tuning (internal/methodology/)
- **release-engineer** — VERSION, CHANGELOG, PRs, CI

For cross-domain tasks, spawn the orchestrator which will create a team and dispatch work.
For single-domain tasks, spawn the specialist directly.

Agent definitions: `.claude/agents/`

## Design System
Always read DESIGN.md before making any visual or UI decisions.
All font choices, colors, spacing, and aesthetic direction are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that doesn't match DESIGN.md.

## gstack
Use /browse from gstack for all web browsing.
Never use mcp__claude-in-chrome__* tools.
Available skills: /office-hours, /plan-ceo-review, /plan-eng-review,
 /plan-design-review, /design-consultation, /review, /ship, /browse, /qa,
 /qa-only, /design-review, /setup-browser-cookies, /retro, /investigate,
 /document-release, /codex, /careful, /freeze, /guard, /unfreeze,
 /gstack-upgrade
