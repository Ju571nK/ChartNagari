---
name: verify
description: "Drive ChartNagari's local web UI (Go server and React workspace) the way a user does. Launch an isolated instance, exercise Symbols, Chart, and Settings journeys, and capture proof. Use when proving a change works, reproducing a UI bug, or checking user-visible behavior end to end."
---

# Verify ChartNagari (web workspace)

The primary surface is the browser workspace served by `cmd/server` at `/`. That workspace includes Symbols, Chart, Analysis, Backtest, and Settings. Secondary surfaces stay out of scope unless a feature file says otherwise. Those surfaces are HTTP `/api/*`, Telegram, Discord, MCP, and the Alpaca plugin. Never drive the user's everyday `localhost:8080` session. Drive only instances started by this skill's launch helper.

Proof files that survive cleanup live in `.claude/skills/verify/artifacts/`.

Run the helper scripts from the repo root. They live in `.claude/skills/verify/scripts/`.

## Launch

Requires Go 1.26+, a built `web/dist` (`npm --prefix web ci && npm --prefix web run build` once), and free loopback ports near `18080`.

```sh
./.claude/skills/verify/scripts/launch.sh
# prints VERIFY_RUN_ID, VERIFY_BASE_URL, and VERIFY_STATE
source /tmp/chartnagari-verify-latest/state.env
```

A short-lived Cursor or agent shell may reap the server when the command ends. Start `launch.sh` in a long-lived background shell that waits on the PID, or re-exec the binary with its working directory set to `$VERIFY_ROOT`. Drive from other shells. Run `doctor.sh` before every drive.

What launch does:

- Creates `/tmp/chartnagari-verify-<RUN_ID>/` with its own `config/` and `data/`
- Copies committed YAML templates and starts from an empty watchlist
- Symlinks `web/` from the repo so `web/dist` resolves from the scratch working directory
- Builds `chart-analyzer` into `/tmp/chartnagari-verify-bin/` when missing/stale
- Binds `127.0.0.1:<port>` (default search from 18080)
- Ready when `GET /health` succeeds

Tear down with `./.claude/skills/verify/scripts/cleanup.sh`. That command keeps `artifacts/`.

Two runs can coexist on different ports. Do not point verification at a shared `config/settings.yaml` under the repo.

## Doctor

```sh
./.claude/skills/verify/scripts/doctor.sh
```

Pass criteria are a live process, `/health` OK, that PID owning the port, and `VERIFY_ROOT` under `/tmp/chartnagari-verify-*`. Run doctor before driving when anything looks wrong.

These scripts do not read flags. `doctor.sh --help` still looks up the state file. If `VERIFY_STATE` is unset and `/tmp/chartnagari-verify-latest/state.env` is missing, `doctor.sh` prints `doctor: no state file` and exits 1. That path does not start a server and does not call `/health`. Point `VERIFY_STATE` at a missing file to check `cleanup.sh` the same way. It prints `cleanup: nothing to do` and exits 0. Do not pass `--help` to `launch.sh`. It ignores the flag and starts a server.

## Drive

The drive harness is the installed `agent-browser` CLI. Prefer accessible names from the English UI (`i18n` en).

After launch and a passing doctor, run:

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

Read the feature map under `features/` before choosing a journey. Drive the real user path through navigation and forms. Leave internal React setters alone. An HTTP check may confirm a side effect, such as `GET /api/symbols`. The UI journey is the proof.

## Evidence

For each feature ID under `.claude/skills/verify/artifacts/<feature-id>/`:

1. Action proof: ARIA snapshot before/after (`agent-browser snapshot -i` redirected to a `.aria.txt` file) and a screenshot (`agent-browser screenshot <path>.png`) showing ChartNagari chrome.
2. Result proof: the changed UI state and a side-effect check. For example, `curl -fsS "$VERIFY_BASE_URL/api/symbols"` contains the new symbol.
3. Record `VERIFY_RUN_ID`, `VERIFY_BASE_URL`, and the feature file entry point used.

Proof standards follow the user path. Capture the action and the resulting state. Check persistence or the API side effect. Do not mock Yahoo or Binance when the journey needs live validation. Forex client-side validation is allowed for pairs such as EURUSD. If a precondition is missing, fail the journey and note it in that feature's Gotchas. Do not weaken the recipe.

## Cleanup

```sh
./.claude/skills/verify/scripts/cleanup.sh
```

Cleanup kills only the PID recorded in `state.env`. It removes the scratch root under `/tmp/chartnagari-verify-*` and leaves `.claude/skills/verify/artifacts/` intact. Never `pkill chart-analyzer` by name.

## Helpers

| Script | Purpose |
| --- | --- |
| `scripts/launch.sh` | Isolated instance |
| `scripts/doctor.sh` | Read-only readiness |
| `scripts/cleanup.sh` | Teardown without deleting artifacts |

Optional: `VERIFY_PORT=18090 ./.claude/skills/verify/scripts/launch.sh` to pin a port. `VERIFY_KEEP_ROOT=1` on cleanup keeps the scratch tree for debugging.

## Feature map

Index: [features/README.md](features/README.md). Maintain with `/maintain-verification-skill` as the product changes.
