# Signal Performance Tracking — Design Spec

**Date:** 2026-05-09
**Target version:** v2.9.0.0
**Branch (proposed):** `feat/signal-performance-tracking`
**Status:** Approved (sections 1–6 confirmed by user)

---

## 1. Problem & Goal

ChartNagari already has three layers of "did the signal work?" data:
- **Forward returns** — automatic 5d/10d/20d/40d returns persisted on every signal.
- **Paper trading** — every live signal becomes a virtual position with TP/SL exits.
- **Backtest stats** — `WinRate / AvgRR / ProfitFactor` per rule from historical replay.

None of these reflect **what the user actually traded**. A user typically skips ~80% of fired alerts (gut feel, news context, position-sizing limits) and only takes ~20%. The signals they actually entered, and the outcomes of those entries, are the user's true edge — and that data does not exist anywhere today.

The user's stated mental model: "내 손에서 ICT Liquidity Sweep은 4/12, Wyckoff Spring은 8/10" — Wins / Took, per rule, personal only.

**Goal:** Let users mark each fired alert as `Took / Skipped`, then for `Took` alerts mark the outcome as `Win / Loss / BE`. Aggregate per rule / symbol / methodology / timeframe so users can see their personal hit rate and tune accordingly. Surface in the existing Performance tab (alongside backtest), a new dedicated **My Trades** tab, and a new MCP tool.

**Non-goals (v1):**
- Replacing forward returns or paper trading. Those stay; this is additive.
- ±X R precision input. v1 is 3-state (Win/Loss/BE).
- Auto-tuning rule weights from personal hit rate (deferred to v2.11+).
- Multi-user / multi-chat-id. Single user assumption (consistent with current notifier).

---

## 2. Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│ Telegram (mobile, primary marking surface)                             │
│   Alert msg ──[✅ Took / ❌ Skipped]──> callback_query                 │
│                                          ↓                              │
│                                   editMessageReplyMarkup                │
│                                   (Win/Loss/BE buttons appear)         │
└──────────────────────────────────────┬─────────────────────────────────┘
                                       │ callback → MarkStore.Mark
                                       ▼
┌────────────────────────────────────────────────────────────────────────┐
│ Backend (Go)                                                            │
│  TelegramBot ──► SignalMarkStore ──► SQLite (signal_marks table)       │
│  HTTP handlers (POST/GET /api/marks/*)  ▲                               │
│  MCP tool (get_my_performance) ─► Aggregator                            │
│                                                                         │
│  Notifier dispatch updates: SendAlert returns message_id, persisted     │
│  via SetMessageID. message_id enables future editMessageReplyMarkup.   │
└────────────────────────────────────────┬────────────────────────────────┘
                                         │ HTTP / MCP
                                         ▼
┌────────────────────────────────────────────────────────────────────────┐
│ Frontend (React)                                                        │
│  - My Trades tab (NEW): Rollup / Pending / History subtabs              │
│  - Performance tab (extended): My Took / My Hit Rate / Skip Rate / Δ   │
│  - Shared <MarkActions> component (used in Pending/History/Performance)│
└────────────────────────────────────────────────────────────────────────┘
```

**Source of truth split:** Forward returns, paper positions, backtest stats are unchanged and remain authoritative for their respective concerns. `signal_marks` is the user's intent log — what they took, what they skipped, what came of it.

---

## 3. Data Model

### 3.1 New SQLite table

```sql
CREATE TABLE IF NOT EXISTS signal_marks (
  signal_id      INTEGER PRIMARY KEY,
  status         TEXT NOT NULL DEFAULT 'PENDING',
                 -- 'PENDING' | 'TOOK' | 'SKIPPED' | 'WIN' | 'LOSS' | 'BE'
  took_at        INTEGER,        -- unix sec; NULL for SKIPPED, set on TOOK or any outcome
  outcome_at     INTEGER,        -- unix sec; set on WIN/LOSS/BE
  outcome_pnl_r  REAL,            -- nullable; reserved for v1.5 ±R input. NULL in v1.
  notes          TEXT,            -- nullable; reserved for future free-form note
  tg_message_id  INTEGER,         -- nullable; populated when alert sent via Telegram
  updated_at     INTEGER NOT NULL,
  FOREIGN KEY (signal_id) REFERENCES signals(id)
);

CREATE INDEX IF NOT EXISTS idx_signal_marks_status ON signal_marks(status);
```

**Lazy-create:** No row exists for unmarked signals. The "PENDING" state is implicit — a signal with no row in `signal_marks` is treated as PENDING. The first mark INSERTs a row.

### 3.2 Finite State Machine

```
(no row) ──[took]──> TOOK ──[win|loss|be]──> WIN | LOSS | BE
(no row) ──[skip]──> SKIPPED
SKIPPED ──[undo]──> (row deleted, back to no-row PENDING)
TOOK    ──[undo]──> (row deleted)
WIN/LOSS/BE ──[win|loss|be]──> WIN/LOSS/BE   (outcome correction)
WIN/LOSS/BE ──[undo]──> TOOK                 (one step back)
```

Invalid transitions return 400. The Telegram inline keyboard always shows only the legal next-step buttons — invalid input is reachable only via direct HTTP call.

### 3.3 Policy decisions (locked into spec)

| Policy | Decision | Reason |
|---|---|---|
| Skipped tracked | Yes | Skip Rate per rule reveals user filtering behavior |
| PENDING timeout | None | User can mark anytime, even months later |
| Edit / undo | Yes, all transitions reversible | Keeps marking low-friction; no audit log (YAGNI) |
| Auto-PENDING row | No | Lazy-create on first mark |
| Multi chat_id | No | Single user assumption (consistent with current notifier) |

---

## 4. Backend

### 4.1 New files

| File | Responsibility |
|---|---|
| `internal/storage/signal_marks.go` | `SignalMarkStore` — CRUD + `Mark(signalID, action) (newStatus, error)` with FSM enforcement |
| `internal/storage/signal_marks_test.go` | Roundtrip + full FSM transition matrix |
| `internal/marks/aggregator.go` | `Aggregator.Rollup(by, since) []RollupRow` — SQL GROUP BY for rule/symbol/timeframe; methodology grouped in Go via `RuleMethodology(rule)` mapping |
| `internal/marks/aggregator_test.go` | Table-driven aggregation tests |
| `internal/api/marks_handler.go` | HTTP handlers: POST /api/marks/{signal_id}, GET /api/marks/{pending,recent,rollup} |
| `internal/api/marks_handler_test.go` | Auth, validation, FSM-error path tests |

### 4.2 Modified files

| File | Change |
|---|---|
| `internal/storage/db.go` | Add `CREATE TABLE IF NOT EXISTS signal_marks ...` to `migrate()` |
| `internal/notifier/telegram.go` | `SendAlert(sig)` returns `message_id`; emits `reply_markup` with inline keyboard |
| `internal/notifier/telegram_bot.go` | Handle `CallbackQuery` updates; new `MarkStore` interface dependency; new `editMessageReplyMarkup` and `answerCallbackQuery` helpers |
| `internal/notifier/notifier.go` | After Telegram send, call `markStore.SetMessageID(sig.ID, msgID)` |
| `internal/api/server.go` | Add `markStore` field + setter; register 4 new routes; pass through to MCP registry |
| `internal/mcp/tools.go` | New `GetMyPerformanceTool` with `RollupSource` interface |
| `internal/mcp/registry.go` | Register new tool — count goes 5 → 6 |
| `cmd/server/main.go` | Construct `SignalMarkStore`; wire into notifier, API server, MCP registry |
| `pkg/models/signal.go` | (No change — `Signal.ID` already exists) |

### 4.3 Aggregator SQL (rule example)

```sql
SELECT s.rule,
       SUM(CASE WHEN m.status IN ('TOOK','WIN','LOSS','BE') THEN 1 ELSE 0 END) AS took,
       SUM(CASE WHEN m.status = 'SKIPPED' THEN 1 ELSE 0 END) AS skipped,
       SUM(CASE WHEN m.status = 'WIN'  THEN 1 ELSE 0 END) AS wins,
       SUM(CASE WHEN m.status = 'LOSS' THEN 1 ELSE 0 END) AS losses,
       SUM(CASE WHEN m.status = 'BE'   THEN 1 ELSE 0 END) AS bes
FROM signal_marks m
JOIN signals s ON s.id = m.signal_id
WHERE m.updated_at >= ?
GROUP BY s.rule
ORDER BY took DESC
```

Substitute `s.symbol` / `s.timeframe` for symbol/timeframe groupings. Methodology grouping is post-processed in Go via the existing `RuleMethodology(rule string) string` function in `internal/config/symbol_profiles.go`.

### 4.4 Stats computation

```go
type RollupRow struct {
    Key        string  // rule/symbol/methodology/timeframe value
    Took       int     // status IN (TOOK,WIN,LOSS,BE)
    Skipped    int
    Wins       int
    Losses     int
    BreakEvens int
    HitRate    float64 // wins / (wins+losses+bes)
    SkipRate   float64 // skipped / (took+skipped)
}
```

- `HitRate = wins / (wins + losses + bes)` — BE counted in denominator (real exit happened) but not numerator (P&L was zero).
- `SkipRate = skipped / (took + skipped)` — denominator is total user decisions.
- Both clamped to `0.0` when denominator is 0.

### 4.5 Concurrency

- `*sql.DB` is thread-safe.
- `Mark()` is a single UPSERT — atomic. Telegram callback and web/MCP marks racing for the same `signal_id` is last-write-wins. Single-user assumption makes this acceptable.
- No optimistic concurrency token in v1.

### 4.6 Defensive behavior

| Failure | Behavior |
|---|---|
| Telegram callback DB write fails | `answerCallback` shows ❌ toast, keyboard not edited (user can retry) |
| `editMessageReplyMarkup` fails | WARN log; the DB mark already succeeded. User sees correct status in web UI / next callback. |
| `SetMessageID` fails after alert send | WARN log; alert delivered, but message_id absent → keyboard cannot be edited later. Mark still works. |
| Aggregator query error | 500 + JSON error body |
| Mark of unknown signal_id | 404 + `{"error":"signal not found: <id>"}` |
| Invalid FSM transition | 400 + `{"error":"invalid transition: TOOK → skip"}` |
| Mark write failure | Pipeline alert delivery is **never** halted (alerts must keep flowing) |

---

## 5. Telegram Bot Extension

### 5.1 Inline keyboard send

Alert message gains `reply_markup`:

```json
{
  "inline_keyboard": [[
    {"text": "✅ Took",    "callback_data": "took:{signal_id}"},
    {"text": "❌ Skipped", "callback_data": "skip:{signal_id}"}
  ]]
}
```

`callback_data` format: `{action}:{signal_id}`. Telegram caps at 64 bytes; int64 signal IDs comfortably fit.

### 5.2 Callback handling

`telegram_bot.go:handleUpdate` extended:

```go
func (b *TelegramBot) handleUpdate(ctx context.Context, u tgUpdate) {
    if u.CallbackQuery != nil {
        b.handleCallback(ctx, u.CallbackQuery)
        return
    }
    if u.Message == nil { return }
    // existing /analysis handling
}
```

`handleCallback` flow:
1. Verify `chat_id` matches configured chat (consistent with message handling).
2. Parse `callback_data` → `(action, signalID)`. Unknown action → answerCallback with error toast and return.
3. Call `markStore.Mark(signalID, action)` → new status.
4. `answerCallbackQuery(callback_id, text="✓ {newStatus}")` to dismiss spinner and show transient toast.
5. `editMessageReplyMarkup(chat_id, message_id, nextKeyboardFor(newStatus, signalID))`.

### 5.3 Status → next keyboard mapping

| Current status | Next inline keyboard |
|---|---|
| TOOK | `[💰 Win] [💸 Loss] [⚖ BE] [↺ Undo]` |
| SKIPPED | `[↺ Undo]` |
| WIN / LOSS / BE | `[↺ Edit]` (single button — sends `undo` action, reverts to TOOK; the next message edit then shows the W/L/BE keyboard again) |

The `[↺ Edit]` button on outcome states is a UI convenience that issues the same `undo` action as the SKIPPED Undo button. The HTTP API still accepts direct `win`/`loss`/`be` actions on outcome states (e.g., for automation), but the keyboard does not expose them — fewer buttons reduce mistap risk.

### 5.4 Message text accumulation

The alert text is appended (not replaced) on each transition:

```
🔔 BTCUSDT · 1H · ICT Order Block (LONG)
Score: 14.0
💰 Entry: 67450 | TP: 68200 | SL: 67050
✓ Took at 14:32 KST → 💰 Win at 16:48 KST
```

`editMessageText` accompanies `editMessageReplyMarkup` on each transition so the message becomes a self-contained mini-journal.

Timezone for display: server local time (matches existing alert formatter). UTC is used for storage (`took_at`, `outcome_at` are unix seconds).

### 5.5 New Telegram API helpers

| Function | API endpoint | Used for |
|---|---|---|
| `editMessageReplyMarkup` | `editMessageReplyMarkup` | Replace keyboard after each transition |
| `editMessageText` | `editMessageText` | Append status line to alert text |
| `answerCallbackQuery` | `answerCallbackQuery` | Dismiss spinner + show toast |

All implemented with stdlib `net/http`; no new dependencies.

### 5.6 Discord

Not modified in v1. Discord receives the same text alert without buttons. Discord interaction API support is deferred (out of scope item #4 in §11).

### 5.7 New `MarkStore` interface in notifier package

```go
// MarkStore is the storage interface the bot consumes.
// *storage.SignalMarkStore satisfies it.
type MarkStore interface {
    Mark(signalID int64, action string) (newStatus string, err error)
}
```

Constructor signature change:
```go
func NewTelegramBot(token, chatID string, handler AnalysisHandler, mark MarkStore) *TelegramBot
```

`mark` may be nil for embedders that don't use marking — in that case callbacks log and answerCallback with `(marking disabled on this server)`.

---

## 6. HTTP API

All endpoints under `/api/marks`. POST is bearer-authenticated via existing middleware. GET endpoints are unauthenticated (consistent with all other GET endpoints in this codebase).

### 6.1 Endpoints

| Method | Path | Body / Query | Purpose |
|---|---|---|---|
| POST | `/api/marks/{signal_id}` | `{"action":"<took|skip|win|loss|be|undo>"}` | Apply a mark transition |
| GET | `/api/marks/pending?since=<ISO8601>&limit=<int>` | — | List signals without a mark (or with status PENDING — see lazy-create note below) |
| GET | `/api/marks/recent?since=<ISO8601>&limit=<int>&status=<filter>` | — | List marked signals (any status), optionally filtered |
| GET | `/api/marks/rollup?by=<rule|symbol|methodology|timeframe>&since=<ISO8601>` | — | Aggregated stats |

`since` defaults: pending=24h, recent=30d, rollup=30d. `limit` default 50, max 200.

### 6.2 Lazy-create note for `GET /api/marks/pending`

Since unmarked signals have no row in `signal_marks`, "pending" is computed as a left-join filter:
```sql
SELECT s.* FROM signals s
LEFT JOIN signal_marks m ON m.signal_id = s.id
WHERE m.signal_id IS NULL
  AND s.created_at >= ?
ORDER BY s.created_at DESC
LIMIT ?
```

### 6.3 POST request validation

| Field | Rule |
|---|---|
| `action` | Must be in `{took,skip,win,loss,be,undo}` (lowercase) |
| `signal_id` (path) | Must exist in `signals` table |
| State transition | Must be valid per FSM (§3.2). Server enforces; UI/Telegram-keyboard guard at edge. |

### 6.4 Response shape

POST 200:
```json
{
  "signal_id": 123,
  "status": "TOOK",
  "took_at": 1715252720,
  "outcome_at": null,
  "updated_at": 1715252720
}
```

GET pending/recent: array of:
```json
{
  "signal": { ...full Signal fields... },
  "mark": { "status": "TOOK", "took_at": ..., "outcome_at": ..., "tg_message_id": ... } | null
}
```

GET rollup:
```json
{
  "by": "rule",
  "since": "2026-04-09T00:00:00Z",
  "rows": [
    { "key": "ict_liquidity_sweep", "took": 12, "skipped": 6, "wins": 8, "losses": 3, "bes": 1, "hit_rate": 0.667, "skip_rate": 0.333 },
    ...
  ]
}
```

### 6.5 Caching

`GET /api/marks/rollup` emits `Cache-Control: max-age=60`. Frontend can append `?fresh=1` to bypass. Marking writes do not need to bust the cache server-side; the 60-second window is acceptable lag.

---

## 7. MCP Tool

### 7.1 New tool: `get_my_performance`

**Schema:**
```json
{
  "type": "object",
  "properties": {
    "by":         { "type": "string", "enum": ["rule","symbol","methodology","timeframe"], "default": "rule" },
    "since_days": { "type": "integer", "minimum": 1, "maximum": 730, "default": 30 },
    "filter": {
      "type": "object",
      "properties": {
        "rule":        { "type": "string" },
        "symbol":      { "type": "string" },
        "methodology": { "type": "string", "enum": ["ict","wyckoff","smc","general_ta","candlestick"] }
      }
    }
  }
}
```

### 7.2 Output

Markdown table consistent with `get_analysis` tone:

```markdown
**Personal Performance · last 30 days**

| Rule                     | Took | Win | Loss | BE | Hit Rate | Skip Rate |
|--------------------------|------|-----|------|----|----------|-----------|
| ict_liquidity_sweep      |   12 |   8 |    3 |  1 |    66.7% |     33.3% |
| ict_order_block_bullish  |   17 |   6 |   10 |  1 |    35.3% |     43.3% |
| wyckoff_spring           |    8 |   4 |    3 |  1 |    50.0% |     38.5% |

_Hit Rate = Wins / (Wins + Losses + BE).  Skip Rate = Skipped / (Took + Skipped)._
```

Empty result: `**No marked trades in window.** _(Mark some alerts via Telegram or the My Trades tab first.)_`

### 7.3 Why this matters

A user on a different monitor asks Claude/Codex "내 ICT 어때?" and gets the answer immediately. The LLM can combine `get_analysis` (current chart context) + `get_my_performance` (historical personal edge) for prescriptive output: "TSLA ICT OB just fired, but you've only hit 35% on that rule — consider sizing down or skipping."

### 7.4 Registration

```go
// cmd/server/main.go
mcpReg.Register(mcp.NewGetMyPerformance(markAggregator))
log.Info().Int("tools", 6).Msg("MCP registry wired")  // 5 → 6
```

`docs/MCP_SETUP.md` available-tools table updated to list 6 tools.

---

## 8. Frontend

### 8.1 New / modified files

| File | Action |
|---|---|
| `web/src/MyTradesTab.tsx` | NEW — main tab component |
| `web/src/MyTradesTab.test.tsx` | NEW — Vitest |
| `web/src/MarkActions.tsx` | NEW — shared mark-button component (Pending row, History row, Performance row) |
| `web/src/App.tsx` | Modified — add 'my-trades' Tab type; nav button; route |
| `web/src/App.tsx` (`PerformanceTab`) | Modified — extend table with `My Took / My Hit Rate / My Skip Rate / Δ vs BT` columns |
| `web/src/i18n/locales/{en,ko,ja}.json` | ~30 new keys under `my_trades.*` and `performance.my_*` |

### 8.2 My Trades tab layout

**Top:** 4 summary cards (totals across selected period).

**Middle:** Subtab nav: `Rollup | Pending | History`.

**Subtab — Rollup**
- Controls: `GroupBy [Rule|Symbol|Methodology|Timeframe]`, `Period [7d|30d|90d|365d|all]`.
- Table: Key | Took | Skip rate | Wins | Losses | BE | Hit rate. Sortable columns.

**Subtab — Pending**
- Time-descending list of signals with no mark.
- Each row: time | symbol | rule | TF | score | direction | `<MarkActions signalId status="PENDING" />`.
- Empty state: "No pending signals — all caught up 🎉"

**Subtab — History**
- Marked signals (TOOK/SKIPPED/WIN/LOSS/BE).
- Filters: status, symbol, rule.
- Each row: time | symbol | rule | TF | status badge | `<MarkActions signalId status={current} />` (Edit only).
- Pagination: 50 rows/page.

### 8.3 Performance tab extension

Existing columns preserved. Add (in this order, after Profit Factor):

| New column | Source |
|---|---|
| My Took | `RollupRow.Took` |
| My Hit Rate | `RollupRow.HitRate` |
| Δ vs BT | `MyHitRate − BacktestWinRate` (signed pp) |
| Skip Rate | `RollupRow.SkipRate` |

Period selector applies to `My *` columns only; backtest columns are always full-history.

### 8.4 `<MarkActions>` component contract

```tsx
interface MarkActionsProps {
  signalId: number
  // PENDING is a UI-only concept (no DB row exists yet). The other five
  // map directly to the signal_marks.status column values.
  status: 'PENDING' | 'TOOK' | 'SKIPPED' | 'WIN' | 'LOSS' | 'BE'
  onMarked?: (newStatus: string) => void
  apiToken?: string
}
```

Renders a button row appropriate to the current status (matches the Telegram FSM):
- PENDING → `[Took] [Skipped]`
- TOOK → `[Win] [Loss] [BE] [Undo]`
- SKIPPED → `[Undo]`
- WIN/LOSS/BE → `[Edit]` (issues `undo` to revert to TOOK; the W/L/BE row reappears for re-selection)

Optimistic UI: click → immediate local state change → POST → on failure, roll back and show toast.

### 8.5 i18n keys (~30 total × 3 locales)

```
my_trades.title
my_trades.summary.took
my_trades.summary.win_rate
my_trades.summary.skipped
my_trades.summary.top_rule
my_trades.subtab.rollup
my_trades.subtab.pending
my_trades.subtab.history
my_trades.groupby.rule
my_trades.groupby.symbol
my_trades.groupby.methodology
my_trades.groupby.timeframe
my_trades.period.7d
my_trades.period.30d
my_trades.period.90d
my_trades.period.365d
my_trades.period.all
my_trades.action.took
my_trades.action.skipped
my_trades.action.win
my_trades.action.loss
my_trades.action.be
my_trades.action.undo
my_trades.action.edit
my_trades.empty.pending
my_trades.empty.history
my_trades.column.hit_rate
my_trades.column.skip_rate
my_trades.confirm.edit_outcome
my_trades.save_failed
performance.my_took
performance.my_hit_rate
performance.my_skip_rate
performance.delta_vs_bt
```

### 8.6 Frontend tests

`MyTradesTab.test.tsx` (5–7 tests):
1. Mounts and renders 4 summary cards from mocked GET rollup
2. Changing GroupBy refetches with new query
3. Pending row Took click → optimistic UI shows W/L/BE buttons; PUT issued
4. Edit (Undo) reverts status with appropriate fetch
5. Empty Pending shows "all caught up" message
6. Fetch failure shows save_failed toast and rolls back optimistic UI
7. Period selector triggers refetch

---

## 9. Testing

| Layer | File | Coverage |
|---|---|---|
| Storage CRUD | `internal/storage/signal_marks_test.go` | INSERT/Get/SetMessageID, FK rejection, lazy-create semantics, updated_at auto-set |
| FSM transitions | `internal/storage/signal_marks_test.go` (extension) | Full matrix from §3.2 — every (status × action) → (newStatus or error) |
| Aggregator | `internal/marks/aggregator_test.go` | 4-way GROUP BY, since filter, HitRate/SkipRate (BE in denominator only), zero-division guards, methodology post-process |
| HTTP handler | `internal/api/marks_handler_test.go` | POST happy + invalid action (400) + unknown signal_id (404), GET pending/recent/rollup, auth (POST 401 without token) |
| Telegram callback | `internal/notifier/telegram_bot_test.go` (extension) | callback_query parsing, chat_id rejection, MarkStore call wiring, editMessageReplyMarkup + answerCallbackQuery dispatched |
| MCP tool | `internal/mcp/tools_test.go` (extension) | `get_my_performance` input validation, empty-result markdown, filter (rule/symbol/methodology) |
| Frontend component | `web/src/MyTradesTab.test.tsx` | 7 scenarios above |
| i18n parity | `scripts/check_i18n.sh` | All 3 locales have identical key sets under `my_trades.*` and `performance.my_*` |

**Pass bar:** `go test ./... -race` race-clean across all packages, `bun run test --run` 100%, `tsc --noEmit` clean, `vite build` clean.

### 9.1 FSM transition matrix (test scope)

| from \ action | took | skip | win | loss | be | undo |
|---|---|---|---|---|---|---|
| (no row) | TOOK | SKIPPED | err | err | err | err |
| TOOK | err | err | WIN | LOSS | BE | (delete) |
| SKIPPED | err | err | err | err | err | (delete) |
| WIN | err | err | WIN | LOSS | BE | TOOK |
| LOSS | err | err | WIN | LOSS | BE | TOOK |
| BE | err | err | WIN | LOSS | BE | TOOK |

36 combinations, single table-driven test.

---

## 10. Migration & Compatibility

- **DB:** New `CREATE TABLE IF NOT EXISTS signal_marks ...` line in `internal/storage/db.go` migrations. Idempotent.
- **Data:** No backfill. Empty table starts → all existing signals are implicitly PENDING.
- **YAML:** No config changes.
- **API:** All new endpoints additive. Existing endpoints unchanged.
- **Notifier signature:** `NewTelegramBot` adds a `MarkStore` parameter. Embedders that call `NewTelegramBot` need the new arg. `cmd/server/main.go` is the only caller in this repo. External integrators are out of scope (no public API contract for notifier).
- **Downgrade safety:** Rolling back v2.9 → v2.8 leaves `signal_marks` rows intact. v2.8 ignores the table. Re-upgrading to v2.9 preserves all marks.

---

## 11. Out of Scope (v1)

These are deliberate v1 omissions, named here so future PRs do not re-litigate scope:

1. **±X R precision input** — outcome is 3-state (Win/Loss/BE). Custom R values are v2.10 (column `outcome_pnl_r` already reserved in schema).
2. **Sparkline trend column** — Rollup table will not show a 30-day trend mini-chart. v2.10.
3. **Auto rule-weight tuning** — using personal hit rate to adjust per-symbol score thresholds or rule weights. v2.11+.
4. **Discord interaction-based marking** — Discord users mark via web UI / MCP. v2.10.
5. **Multi-user / multi-chat-id** — single user assumption preserved. Multi-user is a v3 effort.
6. **Trade screenshot attachment** — attaching a chart image at outcome marking. v2.11.
7. **CSV/JSON export** — exporting marked-trade history. v2.10.
8. **Alert grouping** — collapsing rapid-fire same-rule same-symbol alerts into one mark. v2.10.
9. **Web-side ±R input field** — same as #1.

---

## 12. Observability

- INFO log on every mark: `mark recorded signal_id=123 action=took status=TOOK source=telegram` (sources: `telegram | web | mcp`).
- HTTP access logs cover POST/GET via existing zerolog middleware.
- No new metrics in v1. If marking-failure rate becomes a concern, add prometheus-style counters in v2.10.

---

## 13. Documentation

- **CHANGELOG.md** — v2.9.0.0 entry: Added (signal performance tracking, MCP tool, My Trades tab); Changed (notifier signature); Notes (Discord deferred).
- **README** — short "Signal Performance Tracking" section with one screenshot of My Trades Rollup.
- **`docs/MCP_SETUP.md`** — Available tools table updated; 5 tools → 6 tools, `get_my_performance` row.
- **VERSION** — bump 2.8.1.0 → 2.9.0.0 (MINOR — new user-facing feature).

---

## 14. Success Criteria

A user can, after merging v2.9.0.0:

1. Receive a Telegram alert with `[✅ Took] [❌ Skipped]` buttons.
2. Tap `Took` — keyboard updates in-place to `[💰 Win] [💸 Loss] [⚖ BE] [↺ Undo]`; message text shows `✓ Took at HH:MM`.
3. Hours later tap `Win` — keyboard reduces to `[↺ Edit]`; message text shows `✓ Took at HH:MM → 💰 Win at HH:MM`.
4. The next day in the web app My Trades → Rollup, see a row like `ict_liquidity_sweep · Took 12 · Wins 8 · Hit Rate 66.7%`.
5. Performance tab shows the same rule's backtest WinRate alongside the personal Hit Rate, with a `Δ vs BT` column highlighting deviation.
6. From Claude Code: `get_my_performance(by="methodology")` returns a markdown table summarizing personal performance across ICT / Wyckoff / SMC / etc.
7. Telegram alerts that were missed (phone off, etc.) appear in the My Trades → Pending subtab and can be marked there.
8. Period selector (7d / 30d / 90d / 365d / all) recomputes stats live across all surfaces.
9. Editing a wrong outcome (e.g., marked Win but it was actually Loss) is one click via the `[↺ Edit]` button, both in Telegram and the web app.
