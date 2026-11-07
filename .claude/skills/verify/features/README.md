# ChartNagari verification map

Maintained source for verifying user-facing workspace behavior. Read this index, then the matching feature file.

## Baseline preconditions

- Launch with `./.claude/skills/verify/scripts/launch.sh` and `source /tmp/chartnagari-verify-latest/state.env`.
- `./.claude/skills/verify/scripts/doctor.sh` must print `doctor=pass`.
- Drive only `VERIFY_BASE_URL` from that state file (never the operator's daily `:8080` instance).
- UI language: English (`en`) unless a feature says otherwise.
- `agent-browser` is on `PATH`.
- Proof files go under `.claude/skills/verify/artifacts/<feature-id>/`.

## Driving conventions

- Start from the launched empty watchlist unless a feature lists more preconditions.
- Prefer role + accessible name (`Symbols`, `Chart`, `Settings`, `Add`).
- Re-snapshot after navigation; refs from `agent-browser snapshot -i` go stale.
- Restore mutated watchlist symbols after a mutating journey (DELETE via UI or API on the **scratch** instance only).
- Cleanup removes the scratch server; it must not delete artifacts.

## Proof and skip reporting

- Capture the user action and the resulting state, not only the final screen.
- UI proof: ARIA snapshot + screenshot with ChartNagari chrome visible.
- Mutation proof: second view (Chart symbol list or `GET /api/symbols`).
- Record feature ID and entry point with every artifact.
- An unreachable path is a failure. Record the unmet precondition. Do not mark the journey passed by taking a different path.

## Feature entry contract

Each feature file starts with an H1 and one paragraph, then exactly four H2 headings: `Sub-features`, `How to get to it (user POV)`, `Driving it with agent-browser`, and `Gotchas`.

## Features

- [Register a symbol](./symbols-register.md). Add a Forex pair from Symbols and confirm it appears.
- [Open the chart](./chart-view.md). Open Chart with a registered symbol and see the chart surface.
- [Open settings](./settings-view.md). Reach Settings and see the settings workspace.
- [Workspace navigation](./workspace-nav.md). Move between primary nav destinations without losing the app shell.
