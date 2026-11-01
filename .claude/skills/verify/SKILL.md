---
name: verify
description: "Drive ChartNagari's local web UI (Go server + React workspace) the way a user does — launch an isolated instance, exercise Symbols/Chart/Settings journeys, and capture proof. Use when proving a change works, reproducing a UI bug, or checking user-visible behavior end to end."
---

# Verify ChartNagari (web workspace)

Primary surface: the **browser workspace** served by `cmd/server` at `/` (Symbols, Chart, Analysis, Backtest, Settings, …). Secondary surfaces (HTTP `/api/*`, Telegram/Discord, MCP, Alpaca plugin) are out of scope unless a feature file says otherwise. Never drive the user's everyday `localhost:8080` session — only instances started by this skill's launch helper.

Evidence directory (survives cleanup): `.claude/skills/verify/artifacts/`

Helper scripts (run from repo root): `.claude/skills/verify/scripts/`

## Launch

Requires Go 1.26+, a built `web/dist` (`npm --prefix web ci && npm --prefix web run build` once), and free loopback ports near `18080`.

```sh
./.claude/skills/verify/scripts/launch.sh
# → VERIFY_RUN_ID=… VERIFY_BASE_URL=http://127.0.0.1:<port> VERIFY_STATE=/tmp/chartnagari-verify-…/state.env
source /tmp/chartnagari-verify-latest/state.env
```

**Harness note (Cursor / agent shells):** a short-lived shell may reap the server when the command ends. Keep the instance alive by starting launch (or re-exec of the binary with cwd=`$VERIFY_ROOT`) in a long-lived background shell that waits on the PID, then drive from other shells. Confirm with `doctor.sh` before every drive.

What launch does:

- Creates `/tmp/chartnagari-verify-<RUN_ID>/` with its own `config/` and `data/`
- Copies committed YAML templates; starts from an empty watchlist
- Symlinks `web/dist` from the repo
- Builds `chart-analyzer` into `/tmp/chartnagari-verify-bin/` when missing/stale
- Binds `127.0.0.1:<port>` (default search from 18080)
- Ready when `GET /health` succeeds

Teardown: `./.claude/skills/verify/scripts/cleanup.sh` (keeps artifacts/).

Isolation: two runs can coexist on different ports. Do **not** point verification at a shared `config/settings.yaml` under the repo.

## Doctor

```sh
./.claude/skills/verify/scripts/doctor.sh
```

Pass criteria: process alive, `/health` OK, port owned by that PID, `VERIFY_ROOT` under `/tmp/chartnagari-verify-*`. Run doctor whenever anything looks off before driving.

## Drive

Harness: **`agent-browser`** (installed CLI). Prefer accessible names from English UI (`i18n` en).

Baseline after launch + doctor:

```sh
source /tmp/chartnagari-verify-latest/state.env
agent-browser open "${VERIFY_BASE_URL}/"
agent-browser wait --load networkidle
agent-browser snapshot -i
```

Stable handles:

| Control | How to find it |
| --- | --- |
| Workspace nav | `nav` named `Workspace navigation` |
| Symbols | button named `Symbols` |
| Chart | button named `Chart` |
| Settings | button named `Settings` |
| Add-symbol input | placeholder from `symbol_placeholder_nvda` (EN: stock ticker hint) |
| Add | button named `Add` (disabled until validation succeeds) |
| Type select | native `<select>` in the add-symbol form (`Stock` / `Crypto` / `Forex`) |
| Chart symbol | combobox / select labelled with the symbol column name |
| Chart surface | `section` labelled `Chart` |

URL deep links (after load): `/?view=symbols`, `/?view=chart`, `/?view=settings`.

Read the feature map under `features/` before choosing a journey. Drive the real user path (nav + forms), not internal React setters. HTTP checks may **confirm** side effects (`GET /api/symbols`) but are not a substitute for the UI journey.

## Evidence

For each feature ID under `.claude/skills/verify/artifacts/<feature-id>/`:

1. Action proof: ARIA snapshot before/after (`agent-browser snapshot -i` redirected to a `.aria.txt` file) and a screenshot (`agent-browser screenshot <path>.png`) showing ChartNagari chrome.
2. Result proof: the changed UI state **and** a side-effect check (e.g. `curl -fsS "$VERIFY_BASE_URL/api/symbols"` contains the new symbol).
3. Record `VERIFY_RUN_ID`, `VERIFY_BASE_URL`, and the feature file entry point used.

Proof standards: exercise the user path; capture action + resulting state; verify persistence/API side effects; no mocks for Yahoo/Binance when the journey needs live validation — Forex client-side validation is allowed for EURUSD-style pairs. If a precondition is missing, fail the journey and note it in that feature's Gotchas; do not weaken the recipe.

## Cleanup

```sh
./.claude/skills/verify/scripts/cleanup.sh
```

Kills only the PID recorded in `state.env`, removes the scratch root under `/tmp/chartnagari-verify-*`, and leaves `.claude/skills/verify/artifacts/` intact. Never `pkill chart-analyzer` by name.

## Helpers

| Script | Purpose |
| --- | --- |
| `scripts/launch.sh` | Isolated instance |
| `scripts/doctor.sh` | Read-only readiness |
| `scripts/cleanup.sh` | Teardown without deleting artifacts |

Optional: `VERIFY_PORT=18090 ./.claude/skills/verify/scripts/launch.sh` to pin a port. `VERIFY_KEEP_ROOT=1` on cleanup keeps the scratch tree for debugging.

## Feature map

Index: [features/README.md](features/README.md). Maintain with `/maintain-verification-skill` as the product changes.
