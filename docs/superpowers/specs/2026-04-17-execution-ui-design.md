# Execution Plugin Phase 4 — React UI Design

**Date:** 2026-04-17
**Author:** Justin Kwon
**Status:** APPROVED (ready for implementation plan)
**Supersedes:** Phase 4 section of `docs/execution-plugin-architecture.md`

## Problem

Phases 1–3 shipped the execution plugin plumbing (TradeSignal envelope, dispatcher with HMAC + dedup, Alpaca paper adapter). The server runs, the Alpaca sidecar runs, but there is no UI surface. Operators today verify behaviour through `curl` and server logs only. There is no way to:

- Confirm an Alpaca adapter is receiving signals and filling orders.
- Pause dispatch quickly when something is wrong.
- Rotate a plugin secret.
- Tune `min_score` or `symbols` without editing YAML and restarting.

The gap blocks the operational feedback loop and blocks any non-Alpaca adapter work (we would just be adding more invisible sidecars).

## Goal

Ship a React UI that makes execution visible and operable from the browser:

- View plugin health (24h SUBMITTED / FILLED / REJECTED counts, last failure).
- Pause/resume all dispatch via a prominent kill switch.
- Full CRUD on plugins (add, edit, delete, toggle, rotate secret).
- Edit global execution settings (`max_dispatched`, `dedup_window`, `symbol_map`).
- Browse recent order feedback with filters by plugin/status/symbol.

## Non-Goals

- WebSocket real-time push (polling only; deferred to Phase 5 TODO).
- Playwright E2E infrastructure (deferred to separate PR).
- Detailed metrics: active position count, average dispatch latency, dedup skip counter (deferred to Phase 5 TODO).
- Multi-user optimistic locking via ETag (simple `version` field is sufficient).
- Bulk symbol_map import/export.
- Additional broker adapters (IBKR, Binance paper) — separate scope.

## Architecture

### Frontend file layout

```
web/src/
├── ExecutionTab.tsx          container, state orchestration
├── KillSwitch.tsx            banner + confirm modal + toggle button
├── PluginCard.tsx            per-plugin health card
├── PluginEditModal.tsx       create/edit form + secret generate
├── GlobalConfigForm.tsx      max_dispatched / dedup_window / symbol_map
└── FeedbackTable.tsx         filtered feedback list + refresh
```

Each file is ≤ ~200 lines. Precedent: `AnalysisTab.tsx` is already a separate file; this continues that pattern rather than bloating `App.tsx` (which is already 4000+ lines).

### Backend surface

**New endpoints** (added to `internal/api/execution_handler.go`):

- `GET /api/execution/feedback?plugin=&status=&symbol=&limit=100`
- `GET /api/execution/plugins/stats?window=24h`

**Modified endpoints:**

- `PUT /api/execution/config` — now requires `version: N` in request body; returns 409 with `current_version` on mismatch; response includes bumped version.
- `POST /api/execution/kill` — writes `killed_at` to SQLite `execution_state` table (not YAML).
- `GET /api/execution/config` — response includes `killed_at` and `version` from state table and config.

**New table** (`internal/db` migration):

```sql
CREATE TABLE IF NOT EXISTS execution_state (
  key         TEXT PRIMARY KEY,
  value       TEXT NOT NULL,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Used as generic key-value store for runtime state. First two keys: `killed_at`, `config_version`.

**Schema change:**

```sql
ALTER TABLE feedback_idempotency ADD COLUMN symbol TEXT;
CREATE INDEX idx_feedback_created_at ON feedback_idempotency(created_at);
```

Pre-existing rows have NULL `symbol`. Symbol filter `WHERE symbol = ?` excludes NULL; acceptable (decision 9C — 24h window naturally cycles NULLs out).

### Integration in `cmd/server/main.go`

Wire a new `ExecutionStateStore` alongside the existing dispatcher/dedup/feedback stores and pass it to the API handler.

## Component Contracts

### `ExecutionTab.tsx`

State:
- `config: ExecutionConfig | null`
- `stats: PluginStats[]`
- `feedback: Feedback[]`
- `feedbackFilters: { plugin, status, symbol }`
- `loading`, `error` per section

Lifecycle:
- Mount: `Promise.allSettled([loadConfig(), loadStats(), loadFeedback()])`.
- Polling (30s): `loadStats()` + `loadFeedback()` in `Promise.allSettled`. Paused when `document.visibilityState === 'hidden'`.
- After any mutation (PUT config / POST kill / delete): reload affected sections.

Layout:
```
┌───────────────────────────────────────────┐
│ <KillSwitch>                              │
├───────────────────────────────────────────┤
│ <PluginCard> x N    [+ Add plugin]        │
├───────────────────────────────────────────┤
│ <GlobalConfigForm>                        │
├───────────────────────────────────────────┤
│ <FeedbackTable>                           │
└───────────────────────────────────────────┘
```

### `KillSwitch.tsx`

Props: `killed: boolean`, `killedAt: string | null`, `onToggle: () => Promise<void>`

- `killed=true`: full-width red banner `EXECUTION KILLED — Last killed: <timestamp>` with `Re-enable` button.
- `killed=false`: minimal bar with red `Kill Switch` button.
- Either direction shows a confirm modal: "Really disable/enable all plugin dispatch?" / Cancel + Confirm.
- Design tokens: `--danger` banner, `--safe` microcopy on enable.

### `PluginCard.tsx`

Props: `plugin`, `stats`, `onEdit`, `onDelete`, `onToggleEnabled`

Single-line card:
```
[● toggle] alpaca-paper  http://localhost:9100  [24h: 12 filled / 1 rejected]  [Edit] [Delete]
```
- `stats` undefined → `no activity` microcopy.
- `last_failure_msg` present → `--danger` border + tooltip.
- `enabled=false` → card content opacity 0.4, toggle itself full opacity.
- Delete → confirm modal.

### `PluginEditModal.tsx`

Props: `plugin: PluginConfig | null` (null = create), `onSave`, `onCancel`, `existingNames: string[]`

Fields:
- `name` — editable in create, readonly in edit (treated as id).
- `url` — text input; validates `^https?://`.
- `secret` — password field with `[Generate]` + `[Show/Hide]` buttons. Create mode required. Edit mode optional with placeholder `Leave blank to keep current secret`.
- `enabled` — checkbox.
- `symbols` — comma-separated input; parsed to uppercase array.
- `min_score` — number input; validates `≥ 0`.
- `direction_filter` — select: `Both | LONG | SHORT`.

Behaviour:
- `plugin` prop change → `useEffect` resets form state (decision 6A).
- Generate click: `crypto.getRandomValues(new Uint8Array(32))` → hex → fills field → `navigator.clipboard.writeText` → toast. On clipboard failure: toast `Copy manually: <first 12 chars>...` + keep field populated (decision 14A).
- Save: `PUT /api/execution/config` with full config + `version` field. 409 → keep modal open, show banner `Config changed elsewhere`. 422 → inline field errors.

### `GlobalConfigForm.tsx`

Props: `config`, `onSave: (partial) => Promise<void>`

Fields:
- `max_dispatched` — number input.
- `dedup_window` — text input, duration string (`"5m"`, `"1h"`). Backend parses; 422 error renders inline (decision 7B).
- `symbol_map` — collapsible section with per-row editor: `[symbol] → [broker:ticker, ...]`. Add/Remove row buttons. Duplicate symbol keys and empty values rejected client-side before submit.

Save button enabled only when the form is dirty. Discard resets to last-fetched config.

### `FeedbackTable.tsx`

Props: `feedback`, `filters`, `onFiltersChange`, `onRefresh`, `pluginNames`

- Columns: Time · Plugin · Signal ID (8-char prefix) · Symbol · Status · Order ID · Message.
- Status colors: FILLED/PARTIAL_FILL = `--safe`, REJECTED/ERROR = `--danger`, SUBMITTED/RECEIVED = `--muted`, CANCELLED = `--slate`.
- Three filter dropdowns + `Refresh` button + `Last updated: Xs ago`.
- Filter change → `setFeedback([])` synchronously before refetch (v2.4.0.2 regression pattern).

## Data Flow

### Initial load

```
mount
 ├─ GET /api/execution/config    → setConfig (includes version, killed_at)
 ├─ GET /api/execution/plugins/stats?window=24h
 └─ GET /api/execution/feedback?limit=100
```

Each independent via `Promise.allSettled`. Section-level failure renders `Unable to load` without blocking siblings.

### Mutations

| Action | Request | On success | On 409 | On 422 |
|--------|---------|-----------|--------|--------|
| Toggle kill | `POST /api/execution/kill` | `loadConfig()` | — | — |
| Toggle plugin enabled | `PUT /api/execution/config` (with version) | `loadConfig()` + `loadStats()` | banner | inline |
| Edit plugin | `PUT /api/execution/config` | `loadConfig()`, close modal | banner, keep modal | inline |
| Delete plugin | `PUT /api/execution/config` (omit plugin) | optimistic remove + `loadConfig()` | banner, rollback | — |
| Save global config | `PUT /api/execution/config` | `loadConfig()` | banner | inline |
| Change feedback filter | `GET /api/execution/feedback?…` | replace feedback | — | — |

### Polling

```
setInterval(30_000) {
  if (document.visibilityState !== 'visible') return;
  Promise.allSettled([loadStats(), loadFeedback()]);
}
```

Response cache: `GET /api/execution/plugins/stats` emits `Cache-Control: max-age=60` (decision 2A) to amortise aggregation cost.

## Error Handling

### Backend status codes

| Condition | Code | Body |
|-----------|------|------|
| Missing/invalid bearer token | 401 | `{"error":"unauthorized"}` |
| Malformed JSON | 400 | `{"error":"invalid json","detail":"..."}` |
| Field validation failed | 422 | `{"error":"validation","fields":{...}}` |
| Config version mismatch | 409 | `{"error":"version_conflict","current_version":N}` |
| Duplicate plugin name | 422 | `{"error":"validation","fields":{"name":"already exists"}}` |
| Unknown plugin id in kill path | 404 | `{"error":"plugin_not_found"}` |
| DB failure | 500 | `{"error":"internal"}` (details logged only) |

### Frontend UX

| Failure | UX |
|---------|----|
| Initial section fetch fails | Section renders `Unable to load` + `Retry` button. Sibling sections unaffected. |
| Stats polling fails | Last known value retained. `Last updated: Xm ago` stops advancing. 3 consecutive failures → red dot indicator. |
| Feedback filter fetch fails | Table shows `Unable to load` row after the synchronous `setFeedback([])`. |
| `PUT config` 409 | Modal stays open. Top-of-modal red banner with `Reload` + `Save Anyway` buttons. |
| `PUT config` 422 | Inline field error text next to each failing input. |
| `POST kill` timeout/error | Button shows `Failed — retry` for 3s, then reverts. |
| `clipboard.writeText` rejection | Toast: `Copy manually: <secret prefix>...` |
| `crypto.getRandomValues` failure (theoretical) | Toast: `Generation failed — use openssl rand -hex 32` |

### Toast / banner tokens

- `--danger` — kill banner, 409 banner.
- `--warning` — 3-in-a-row polling failure.
- `--safe` — `Saved`, `Copied`.
- `--muted` — informational (`Config reloaded`).

### Confirmation required

- Delete plugin.
- Kill toggle (either direction).
- `Save Anyway` on 409 conflict.

## Testing Strategy

### Backend (Go, table-driven)

- `TestListFeedback` — all three filters, limit bounds, auth.
- `TestPluginStats` — 24h aggregation correctness, empty plugin, `last_failure_msg`, `Cache-Control: max-age=60` header.
- `TestUpdateConfigVersion` — match, mismatch (409), missing field (400).
- `TestKillStoresInStateTable` — writes to `execution_state`, idempotent, read path reflects it.
- `TestStateGetSet` (new `internal/execution/state_test.go`) — missing key, upsert semantics, `-race` clean.
- `TestNoSecretLeakInExecutionEndpoints` (decision 8A) — hits every `/api/execution/*` endpoint, asserts raw secret never appears in any response body.
- `TestMigrationFeedbackIdempotencySymbolColumn` + `TestMigrationExecutionStateTable` — follow existing `internal/db` migration test pattern.

Coverage target: 80% on new handlers. Reference: existing `internal/execution/` is 82.2%.

### Frontend (Vitest + Testing Library)

Each new component file ships with a sibling `*.test.tsx`:

- `ExecutionTab.test.tsx` — parallel fetch on mount, visibility-pause polling, 409 banner propagation.
- `KillSwitch.test.tsx` — killed/not-killed rendering, modal confirm/cancel flow.
- `PluginCard.test.tsx` — missing stats, failure border, disabled opacity, delete confirm.
- `PluginEditModal.test.tsx` — create/edit modes, Generate + clipboard success and fallback, all validations, useEffect reset on prop change.
- `GlobalConfigForm.test.tsx` — invalid dedup_window inline error, symbol_map add/remove, dirty-state Save enablement.
- `FeedbackTable.test.tsx` — filter-change synchronous clear (v2.4.0.2 pattern), status color classes, refresh timestamp.

Coverage target: 70% on new components.

### i18n

All new strings added to `web/src/i18n/{en,ko,ja}.json`. If a key-parity check script exists, it catches missing translations; otherwise add a simple CI check.

### Regression tests (critical)

1. Stale-rows pattern (v2.4.0.2) — covered by `FeedbackTable` filter-change test.
2. Secret redaction — covered by `TestNoSecretLeakInExecutionEndpoints` blanket test.

## Open Decisions

None. Eng review on 2026-04-17 resolved 14 review items and closed all clarifying questions.

## What Already Exists (reused)

- Backend: HMAC `Sign`/`Verify`, `RedactSecrets`, `MergeIncomingSecrets`, config merge logic, auth middleware — all in `internal/execution/` and `internal/api/` — reused unchanged.
- Frontend: History tab stale-rows pattern (v2.4.0.2), SettingsTab/StatusTab card layout, DESIGN.md token system, i18n pipeline — reused.

## Deferred (tracked in TODOS.md)

- WebSocket feedback push (Phase 5).
- Playwright E2E infrastructure.
- Execution detail metrics (active positions, avg dispatch latency, dedup skip count).

## Implementation Order (hint for writing-plans)

1. Backend: migration (`symbol` column, `created_at` index, `execution_state` table) + `ExecutionStateStore`.
2. Backend: `GET /api/execution/feedback` + `GET /api/execution/plugins/stats` endpoints with tests.
3. Backend: `PUT /api/execution/config` version-check + `POST /api/execution/kill` state-table rewrite.
4. Backend: `TestNoSecretLeakInExecutionEndpoints` blanket test.
5. Frontend: `ExecutionTab` scaffold + wire new top-level tab.
6. Frontend: `KillSwitch`, `PluginCard`, `FeedbackTable` (read paths).
7. Frontend: `PluginEditModal` (CRUD + Generate).
8. Frontend: `GlobalConfigForm` (including symbol_map editor).
9. i18n keys added to en/ko/ja.
10. Full test pass, CHANGELOG entry, version bump.
