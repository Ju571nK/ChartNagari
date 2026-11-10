# ChartNagari improvement stack plan

Five ordered PRs for a self-hosting maintainer. Users get a working Execution tab contract, safer PR gates, clearer alert drops, Discord/Paper parity, and small docs/MCP fixes. Rule: one PR is one verifiable change. Do not auto-apply HTF penalties. Do not add paid hosting.

PR ids in order: `pr-execution-schema` (already on main as #53), `pr-ci-vitest-race`, `pr-alert-drop-reasons`, `pr-discord-paper-review`, `pr-docs-mcp-hygiene`.

## How to read this

One box is one unit of work. Check a box only when its evidence exists. The body is a how-to.

The program runs `playbooks/autopilot-stack.md`. The operator lands the PRs. The agent opens merge-ready PRs and stops at handoff.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

- [x] State the protocol and this plan. Operator said proceed in this order (`작업을 진행`).
- [x] Note `pr-execution-schema` landed as https://github.com/Ju571nK/ChartNagari/pull/53 on `main` (2026-10-19). Skip re-implementation. Merge `origin/main` before later PRs.
- [ ] On hold or stand-down, stop writes.

### Spawn owners

- [ ] `pr-ci-vitest-race` from current trunk after merge of main.
- [ ] `pr-alert-drop-reasons` after CI PR is open or stacked on it.
- [ ] `pr-discord-paper-review` after alert-drop PR.
- [ ] `pr-docs-mcp-hygiene` last (independent of alert/discord but last for blast radius).

File boundaries:

- `pr-ci-vitest-race` touches `.github/workflows/ci.yml`, `web/package.json`, `CONTRIBUTING.md` only as needed.
- `pr-alert-drop-reasons` touches pipeline/notifier/storage/api/web status surfaces.
- `pr-discord-paper-review` touches notifier Discord path, paper API/UI, marks store.
- `pr-docs-mcp-hygiene` touches MCP version strings, AWS restore path, SECURITY.md, execution.yaml comments.

### PR mechanics, for every PR

- [ ] Forge `gh`.
- [ ] Open ready PRs to `main`.
- [ ] Run lint/typecheck/tests before push.
- [ ] Deslop before commit.

### Verdict and merge, for every PR

- [ ] Operator merges after CI green. Agent does not merge under autopilot-stack.

### Boot recipe, for every live lane

- [ ] `git fetch && git checkout <head SHA>`
- [ ] Start `go run ./cmd/server` with disposable config when UI is in scope.
- [ ] Drive with `agent-browser` or project `.claude/skills/verify` helpers.
- [ ] Screenshots under `/tmp/swarm-<pr-id>/`.

## Align execution schema (`pr-execution-schema`)

**Depends on.** None. **Status.** Done on main as #53.

**Files.**

- [x] `web/src/ExecutionTab.tsx` and related form/card/modal/i18n (merged).

**Build.**

- [x] UI uses `id`, `dedup_window_sec`, flat optional `symbol_map`.

**You see.**

- [x] Plugin ids render. Dedup saves use server field names.

**Verify, unit.**

- [x] Vitest on Execution forms (done in #53).

**Verify, live.**

- [ ] Optional manual Execution tab against a running server (left open in #53). Not blocking this stack.

**Verify, perf.**

- [x] None. No interaction timing change claimed.

**Review gate.** None for re-do. Already merged.

## Add PR CI vitest and race (`pr-ci-vitest-race`)

**Depends on.** Merge `origin/main` into the working branch first.

**Files.**

- [ ] Edit `.github/workflows/ci.yml`.
- [ ] Edit `web/package.json` (`test` script to `vitest run` or add `test:ci`).
- [ ] Edit `CONTRIBUTING.md` so local frontend test command matches CI.

**Build.**

- [ ] Backend job runs `go test -race ./...` (or scoped packages if full race is too slow; prefer full `./...` first).
- [ ] Frontend job runs `npx vitest run` after `npm ci`, before or after build.

**You see.**

- [ ] A PR that breaks a Vitest case fails the Frontend job.
- [ ] Race detector runs on every PR to main.

**Verify, unit.**

- [ ] Push a branch and confirm CI workflow YAML is valid (`actionlint` if present, else `gh` workflow run).
- [ ] Locally: `cd web && npx vitest run` and `go test -race ./...` (or document timeout if scoped).

**Verify, live.**

- [ ] Lane 1. Open the CI file on GitHub Actions UI after push. Screenshot green Frontend + Go jobs. Pass: both green on a no-op or real PR head.
- [ ] Regression lane against trunk. Trunk lacks vitest on PR. Record that fact. Gate: new job appears only on head.

**Verify, perf.**

- [ ] Measure CI wall clock on head vs prior Go-test-only job. Fail if Go race job exceeds 25 minutes on ubuntu-latest without a documented package scope change.

**Review gate.** None. No product interaction change.

## Expose alert drop reasons (`pr-alert-drop-reasons`)

**Depends on.** Prefer after CI PR so new tests gate the feature.

**Files.**

- [ ] Pipeline/notifier filter sites that currently debug-log only.
- [ ] Storage table or structured log API for drop reasons.
- [ ] Web surface (Status or signal inspector) to show last drops for a symbol.

**Build.**

- [ ] Persist reason codes (score, HTF, cooldown, daily limit, timeframe, MTF, profile, delivery fail).
- [ ] Read API + minimal UI list.

**You see.**

- [ ] After a suppressed signal, UI or API shows a reason code, not only server logs.

**Verify, unit.**

- [ ] Table-driven Go tests for reason assignment. Vitest for the list component.

**Verify, live.** Ten lanes via verify/agent-browser at PR head (abbreviated list for plan; expand at execution).

- [ ] Lane. Force a cooldown drop on scratch instance. API returns that reason. Screenshot.
- [ ] Regression lane against trunk. Trunk has no reason API. Gate new endpoint + empty-state UI.

**Verify, perf.**

- [ ] Drop-reason write path budget: under 5ms median in unit bench or documented no-op when store disabled.

**Review gate.** Yes. Operator reviews screenshots of the drop-reason UI before merge.

## Discord marks and paper manual close (`pr-discord-paper-review`)

**Depends on.** None hard. Prefer after alert-drop if shared mark types change.

**Files.**

- [ ] Discord notifier interaction or link-back to web Pending.
- [ ] Paper close API + `CLOSED_MANUAL`.
- [ ] Optional `outcome_pnl_r` write path.

**Build.**

- [ ] Discord path offers Took/Skipped parity with Telegram or a deep link to My Trades Pending.
- [ ] PaperTab can close a position manually.

**You see.**

- [ ] Discord user can mark or open Pending. Paper position status becomes `CLOSED_MANUAL`.

**Verify, unit.**

- [ ] Notifier and paper tests for the new paths.

**Verify, live.**

- [ ] Lane. Paper open then manual close. Screenshot status.
- [ ] Regression lane against trunk.

**Verify, perf.**

- [ ] None absolute. Interaction-only.

**Review gate.** Yes.

## Docs and MCP hygiene (`pr-docs-mcp-hygiene`)

**Depends on.** None.

**Files.**

- [ ] MCP `ServerInfo.Version` from `VERSION`.
- [ ] `get_analysis` description timeframes.
- [ ] `docs/aws-personal-always-on.md` restore compose path.
- [ ] `SECURITY.md` settings.yaml + remote_access.
- [ ] `config/execution.yaml` `/ws/execution` comment.

**Build.**

- [ ] Text and docs match runtime.

**You see.**

- [ ] MCP initialize reports current VERSION. AWS restore uses `deploy/aws-personal/docker-compose.yml`.

**Verify, unit.**

- [ ] Go test that MCP version equals VERSION file contents.

**Verify, live.**

- [ ] Lane. `curl` MCP initialize or unit equivalent. No UI gate.

**Verify, perf.**

- [ ] None.

**Review gate.** None.

## Appendix A. Prototypes

No new prototype. Execution schema mismatch was verified by contrasting `web/src/ExecutionTab.tsx` on this branch with `origin/main` after #53.

## Appendix B. Decision log

- Skip redoing Execution UI. #53 already fixed the contract on main.
- Execution playbook is autopilot-stack. Operator merges.
- #32 auto-calibrate stays out of this stack.

## Appendix C. Risks

- Full `go test -race ./...` may be slow. Scope packages if CI hits the 25 minute budget.
- Alert-drop and Discord PRs change interaction. Review-gated.
- No `control-ui` from cursor-team-kit in this environment. Live lanes use `agent-browser` and `.claude/skills/verify`.
