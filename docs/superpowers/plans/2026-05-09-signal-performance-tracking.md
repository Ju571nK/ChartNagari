# Signal Performance Tracking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users mark each fired alert as Took/Skipped → Win/Loss/BE via Telegram inline buttons + web UI, then aggregate per rule/symbol/methodology/timeframe so personal hit rate is visible alongside backtest stats.

**Architecture:** New SQLite table `signal_marks` lazy-created on first mark, FSM-validated. Telegram callback_query handler updates DB and edits message keyboard in-place. Aggregator runs SQL GROUP BY queries for stats. New `<MyTradesTab>` and extended `<PerformanceTab>` consume `/api/marks/*`. New MCP tool `get_my_performance` exposes the same stats to LLM clients.

**Tech Stack:** Go 1.26+ (modernc.org/sqlite, zerolog), React 18 + TypeScript + Vite + react-i18next, Vitest + Testing Library.

**Spec:** `docs/superpowers/specs/2026-05-09-signal-performance-tracking-design.md`

**Branch:** `feat/signal-performance-tracking` (already created, spec committed at `caf219d`)

---

## File Structure

**New files:**

| Path | Responsibility |
|---|---|
| `internal/storage/signal_marks.go` | `SignalMarkStore` — CRUD + FSM-validated `Mark(signalID, action)` |
| `internal/storage/signal_marks_test.go` | FSM transition matrix + roundtrip tests |
| `internal/marks/aggregator.go` | `Aggregator.Rollup(by, since)` — GROUP BY queries with HitRate/SkipRate calc |
| `internal/marks/aggregator_test.go` | 4-way grouping tests |
| `internal/api/marks_handler.go` | POST /api/marks/{signal_id}, GET pending/recent/rollup |
| `internal/api/marks_handler_test.go` | Auth, validation, FSM-error path tests |
| `internal/mcp/get_my_performance.go` | New MCP tool |
| `web/src/MarkActions.tsx` | Shared marking button row (Pending/History/Performance reuse) |
| `web/src/MarkActions.test.tsx` | Optimistic UI + rollback tests |
| `web/src/MyTradesTab.tsx` | New tab: Rollup/Pending/History subtabs |
| `web/src/MyTradesTab.test.tsx` | Subtab navigation + data fetching |

**Modified files:**

| Path | Change |
|---|---|
| `pkg/models/signal.go` | Add `ID int64` field to `Signal` struct |
| `internal/storage/signals.go` | `SaveSignal` returns `(int64, error)` from `LastInsertId()` |
| `internal/storage/db.go` | Add `signal_marks` CREATE TABLE to `migrate()` |
| `internal/pipeline/pipeline.go` | Capture `id` from `SaveSignal`, set `sig.ID = id` before notifier dispatch |
| `internal/notifier/telegram.go` | `SendAlert` returns `(messageID int64, err error)`; emits inline keyboard |
| `internal/notifier/telegram_bot.go` | `tgUpdate` adds `CallbackQuery`; new `handleCallback`, `answerCallback`, `editMessageReplyMarkup`, `editMessageText` helpers; constructor takes `MarkStore` interface |
| `internal/notifier/notifier.go` | After Telegram send, call `markStore.SetMessageID(sigID, msgID)` |
| `internal/mcp/registry.go` | (no change — `Register` already supports any tool) |
| `internal/api/server.go` | Add `markStore`, `aggregator` fields + `WithMarkStore`/`WithAggregator` setters; register 4 routes |
| `cmd/server/main.go` | Construct `SignalMarkStore` + `Aggregator`; wire into pipeline, notifier, api, mcp |
| `web/src/App.tsx` | Add `'my-trades'` to `Tab` type, nav button, route; extend `PerformanceTab` with My-stats columns |
| `web/src/i18n/locales/en.json` | ~30 new keys under `my_trades.*` and `performance.my_*` |
| `web/src/i18n/locales/ko.json` | Same keys, Korean |
| `web/src/i18n/locales/ja.json` | Same keys, Japanese |
| `VERSION` | 2.8.1.0 → 2.9.0.0 |
| `CHANGELOG.md` | Prepend v2.9.0.0 entry |
| `docs/MCP_SETUP.md` | Add `get_my_performance` to tools table |

---

## Phase A — Storage & FSM

### Task A1: `Signal.ID` + `SaveSignal` returns id

**Why first:** Telegram callback_data needs `signal_id`. Pipeline must capture the id from SaveSignal and propagate it to the notifier before the alert ships. Foundational.

**Files:**
- Modify: `pkg/models/signal.go`
- Modify: `internal/storage/signals.go`
- Modify: `internal/pipeline/pipeline.go` (call sites)

- [ ] **Step 1: Add ID field to Signal struct**

In `pkg/models/signal.go`, modify `Signal` struct. Add `ID` as the first field:

```go
type Signal struct {
	ID        int64     `json:"id"`        // populated after SaveSignal; 0 means unsaved
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`
	// ... rest unchanged ...
}
```

- [ ] **Step 2: Change SaveSignal signature**

In `internal/storage/signals.go`, change `SaveSignal` from `error` return to `(int64, error)`:

```go
// SaveSignal persists a generated signal to the database and returns the new row id.
func (db *DB) SaveSignal(sig models.Signal) (int64, error) {
	res, err := db.conn.Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sig.Symbol,
		sig.Timeframe,
		sig.Rule,
		sig.Direction,
		sig.Score,
		sig.Message,
		sig.AIInterpretation,
		sig.ZoneLow,
		sig.ZoneHigh,
		sig.HTFTrend,
		sig.ATRPercentile,
		sig.CreatedAt.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("신호 저장 실패 [%s %s]: %w", sig.Symbol, sig.Rule, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("LastInsertId 실패 [%s %s]: %w", sig.Symbol, sig.Rule, err)
	}
	return id, nil
}
```

- [ ] **Step 3: Update all call sites**

Run: `grep -rn "SaveSignal\b" --include='*.go' | grep -v _test`

For every non-test call site, capture and assign the returned id:

```go
// Before:
if err := db.SaveSignal(sig); err != nil { ... }

// After:
id, err := db.SaveSignal(sig)
if err != nil { ... }
sig.ID = id
```

The pipeline call site is the most important — `sig.ID` must be set BEFORE the notifier dispatch.

- [ ] **Step 4: Update existing tests**

```bash
grep -rn "SaveSignal\b" --include='*_test.go'
```

For every test, change:
```go
err := db.SaveSignal(sig)
```
to:
```go
_, err := db.SaveSignal(sig)
```

(Tests don't need the id; only production code does.)

- [ ] **Step 5: Verify tests pass**

```bash
go test ./internal/storage/ ./internal/pipeline/ -count=1 -race
```

Expected: PASS.

- [ ] **Step 6: Build full repo**

```bash
go build ./...
```

Expected: clean. If any caller was missed, the compile error names the file — fix it and rerun.

- [ ] **Step 7: Commit**

```bash
git add pkg/models/signal.go internal/storage/signals.go internal/storage/signals_test.go internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
# Add any other files the grep found
git commit -m "refactor(storage): SaveSignal returns row id

Required by upcoming signal_marks feature: Telegram callback_data
must include signal_id, which the pipeline now captures from
LastInsertId() and propagates via sig.ID before notifier dispatch."
```

---

### Task A2: `signal_marks` SQLite table

**Files:**
- Modify: `internal/storage/db.go`

- [ ] **Step 1: Add CREATE TABLE to migrations**

In `internal/storage/db.go`, find the `schema := \`...\`` block. Append before the closing backtick:

```sql

	-- Signal performance tracking (Phase: signal-performance-tracking).
	-- Lazy-created on first mark; absent row = implicit PENDING.
	CREATE TABLE IF NOT EXISTS signal_marks (
		signal_id      INTEGER PRIMARY KEY,
		status         TEXT    NOT NULL DEFAULT 'PENDING',
		took_at        INTEGER,
		outcome_at     INTEGER,
		outcome_pnl_r  REAL,
		notes          TEXT,
		tg_message_id  INTEGER,
		updated_at     INTEGER NOT NULL,
		FOREIGN KEY (signal_id) REFERENCES signals(id)
	);
	CREATE INDEX IF NOT EXISTS idx_signal_marks_status ON signal_marks(status);
```

- [ ] **Step 2: Verify schema applies**

```bash
go test ./internal/storage/ -run TestNew -count=1 -race
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/storage/db.go
git commit -m "feat(storage): add signal_marks table

Empty table on existing installs; idempotent migration. Lazy-create
semantic — absent row means signal is implicitly PENDING."
```

---

### Task A3: `SignalMarkStore` CRUD + FSM

**Files:**
- Create: `internal/storage/signal_marks.go`
- Create: `internal/storage/signal_marks_test.go`

- [ ] **Step 1: Write failing FSM test**

Create `internal/storage/signal_marks_test.go`:

```go
package storage

import (
	"strings"
	"testing"
)

// FSM matrix from spec §3.2 / §9.1
//
// from \ action | took | skip | win | loss | be | undo
// (no row)      | TOOK | SKIP | err | err  | err| err
// TOOK          | err  | err  | WIN | LOSS | BE | (delete)
// SKIPPED       | err  | err  | err | err  | err| (delete)
// WIN           | err  | err  | WIN | LOSS | BE | TOOK
// LOSS          | err  | err  | WIN | LOSS | BE | TOOK
// BE            | err  | err  | WIN | LOSS | BE | TOOK

type fsmCase struct {
	from   string // "" means no-row
	action string
	want   string // expected new status, "" for "row deleted", "ERR:..." for expected error substring
}

var fsmCases = []fsmCase{
	{from: "", action: "took", want: "TOOK"},
	{from: "", action: "skip", want: "SKIPPED"},
	{from: "", action: "win", want: "ERR:invalid"},
	{from: "", action: "loss", want: "ERR:invalid"},
	{from: "", action: "be", want: "ERR:invalid"},
	{from: "", action: "undo", want: "ERR:invalid"},

	{from: "TOOK", action: "took", want: "ERR:invalid"},
	{from: "TOOK", action: "skip", want: "ERR:invalid"},
	{from: "TOOK", action: "win", want: "WIN"},
	{from: "TOOK", action: "loss", want: "LOSS"},
	{from: "TOOK", action: "be", want: "BE"},
	{from: "TOOK", action: "undo", want: ""},

	{from: "SKIPPED", action: "took", want: "ERR:invalid"},
	{from: "SKIPPED", action: "skip", want: "ERR:invalid"},
	{from: "SKIPPED", action: "win", want: "ERR:invalid"},
	{from: "SKIPPED", action: "loss", want: "ERR:invalid"},
	{from: "SKIPPED", action: "be", want: "ERR:invalid"},
	{from: "SKIPPED", action: "undo", want: ""},

	{from: "WIN", action: "win", want: "WIN"},
	{from: "WIN", action: "loss", want: "LOSS"},
	{from: "WIN", action: "be", want: "BE"},
	{from: "WIN", action: "undo", want: "TOOK"},
	{from: "LOSS", action: "win", want: "WIN"},
	{from: "LOSS", action: "loss", want: "LOSS"},
	{from: "LOSS", action: "be", want: "BE"},
	{from: "LOSS", action: "undo", want: "TOOK"},
	{from: "BE", action: "win", want: "WIN"},
	{from: "BE", action: "loss", want: "LOSS"},
	{from: "BE", action: "be", want: "BE"},
	{from: "BE", action: "undo", want: "TOOK"},
}

// seedSignal inserts a signal so signal_marks FK is satisfied.
// Returns the signal_id.
func seedSignal(t *testing.T, db *DB) int64 {
	t.Helper()
	_, err := db.conn.Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at)
		VALUES ('BTCUSDT','1H','ict_test','LONG',10.0,'msg','',0,0,'',0,0)`)
	if err != nil {
		t.Fatalf("seed signal: %v", err)
	}
	var id int64
	if err := db.conn.QueryRow(`SELECT id FROM signals ORDER BY id DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("get last id: %v", err)
	}
	return id
}

func TestSignalMarkStore_FSM(t *testing.T) {
	for _, tc := range fsmCases {
		name := tc.from
		if name == "" {
			name = "NOROW"
		}
		t.Run(name+"_"+tc.action, func(t *testing.T) {
			db := newTestDB(t)
			store := NewSignalMarkStore(db)
			id := seedSignal(t, db)

			// Seed the from-state if not no-row.
			if tc.from != "" {
				if err := store.directSetStatus(id, tc.from); err != nil {
					t.Fatalf("seed from-state %q: %v", tc.from, err)
				}
			}

			got, err := store.Mark(id, tc.action)
			if strings.HasPrefix(tc.want, "ERR:") {
				if err == nil {
					t.Fatalf("expected error containing %q, got newStatus=%q nil err", tc.want[4:], got)
				}
				if !strings.Contains(err.Error(), tc.want[4:]) {
					t.Errorf("error mismatch: got %q, want substring %q", err.Error(), tc.want[4:])
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("newStatus = %q, want %q", got, tc.want)
			}
			// For "deleted" case, verify row gone.
			if tc.want == "" {
				row, _ := store.Get(id)
				if row != nil {
					t.Errorf("expected deleted row, got %#v", row)
				}
			}
		})
	}
}

func TestSignalMarkStore_GetMissing(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	got, err := store.Get(99999)
	if err != nil {
		t.Fatalf("Get(missing): %v", err)
	}
	if got != nil {
		t.Errorf("Get(missing) = %#v, want nil", got)
	}
}

func TestSignalMarkStore_SetMessageID(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id := seedSignal(t, db)
	if _, err := store.Mark(id, "took"); err != nil {
		t.Fatalf("Mark took: %v", err)
	}
	if err := store.SetMessageID(id, 4242); err != nil {
		t.Fatalf("SetMessageID: %v", err)
	}
	row, _ := store.Get(id)
	if row == nil || row.TgMessageID == nil || *row.TgMessageID != 4242 {
		t.Errorf("TgMessageID mismatch: %#v", row)
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/storage/ -run TestSignalMarkStore -count=1
```

Expected: FAIL with `undefined: NewSignalMarkStore`, `undefined: SignalMark`, etc.

- [ ] **Step 3: Implement SignalMarkStore**

Create `internal/storage/signal_marks.go`:

```go
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SignalMark is one row of the signal_marks table.
type SignalMark struct {
	SignalID     int64
	Status       string  // PENDING | TOOK | SKIPPED | WIN | LOSS | BE
	TookAt       *int64  // unix sec
	OutcomeAt    *int64
	OutcomePnLR  *float64
	Notes        *string
	TgMessageID  *int64
	UpdatedAt    int64
}

// SignalMarkStore performs CRUD on signal_marks with FSM enforcement.
type SignalMarkStore struct {
	db *DB
}

// NewSignalMarkStore creates a store backed by the given DB.
func NewSignalMarkStore(db *DB) *SignalMarkStore {
	return &SignalMarkStore{db: db}
}

// Get returns the mark for a signal, or (nil, nil) if no row exists.
func (s *SignalMarkStore) Get(signalID int64) (*SignalMark, error) {
	row := s.db.conn.QueryRow(`
		SELECT signal_id, status, took_at, outcome_at, outcome_pnl_r, notes, tg_message_id, updated_at
		  FROM signal_marks WHERE signal_id = ?`, signalID)
	var (
		out      SignalMark
		took     sql.NullInt64
		outc     sql.NullInt64
		pnl      sql.NullFloat64
		notes    sql.NullString
		msg      sql.NullInt64
	)
	err := row.Scan(&out.SignalID, &out.Status, &took, &outc, &pnl, &notes, &msg, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query signal_marks: %w", err)
	}
	if took.Valid {
		v := took.Int64
		out.TookAt = &v
	}
	if outc.Valid {
		v := outc.Int64
		out.OutcomeAt = &v
	}
	if pnl.Valid {
		v := pnl.Float64
		out.OutcomePnLR = &v
	}
	if notes.Valid {
		v := notes.String
		out.Notes = &v
	}
	if msg.Valid {
		v := msg.Int64
		out.TgMessageID = &v
	}
	return &out, nil
}

// Mark applies an action to a signal. Returns the new status, or "" if the row was deleted.
// Validates the FSM transition; invalid transitions return an error containing "invalid".
func (s *SignalMarkStore) Mark(signalID int64, action string) (string, error) {
	cur, err := s.Get(signalID)
	if err != nil {
		return "", err
	}
	from := ""
	if cur != nil {
		from = cur.Status
	}
	newStatus, deleteRow, err := nextFSMState(from, action)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()

	if deleteRow {
		_, err := s.db.conn.Exec(`DELETE FROM signal_marks WHERE signal_id = ?`, signalID)
		if err != nil {
			return "", fmt.Errorf("delete signal_marks: %w", err)
		}
		return "", nil
	}

	// Upsert. took_at is set on TOOK/WIN/LOSS/BE if not already set; outcome_at on WIN/LOSS/BE.
	tookAt := sql.NullInt64{}
	outcomeAt := sql.NullInt64{}
	if cur != nil && cur.TookAt != nil {
		tookAt = sql.NullInt64{Int64: *cur.TookAt, Valid: true}
	}
	switch newStatus {
	case "TOOK":
		if !tookAt.Valid {
			tookAt = sql.NullInt64{Int64: now, Valid: true}
		}
	case "WIN", "LOSS", "BE":
		if !tookAt.Valid {
			tookAt = sql.NullInt64{Int64: now, Valid: true}
		}
		outcomeAt = sql.NullInt64{Int64: now, Valid: true}
	}

	_, err = s.db.conn.Exec(`
		INSERT INTO signal_marks (signal_id, status, took_at, outcome_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(signal_id) DO UPDATE SET
		  status     = excluded.status,
		  took_at    = excluded.took_at,
		  outcome_at = excluded.outcome_at,
		  updated_at = excluded.updated_at`,
		signalID, newStatus, tookAt, outcomeAt, now)
	if err != nil {
		return "", fmt.Errorf("upsert signal_marks: %w", err)
	}
	return newStatus, nil
}

// SetMessageID records the Telegram message_id for a signal so the bot can
// editMessageReplyMarkup later. Caller usually invokes this immediately after
// the alert is sent. Creates a stub PENDING row if no mark exists yet.
func (s *SignalMarkStore) SetMessageID(signalID int64, msgID int64) error {
	now := time.Now().Unix()
	_, err := s.db.conn.Exec(`
		INSERT INTO signal_marks (signal_id, status, tg_message_id, updated_at)
		VALUES (?, 'PENDING', ?, ?)
		ON CONFLICT(signal_id) DO UPDATE SET
		  tg_message_id = excluded.tg_message_id,
		  updated_at    = excluded.updated_at`,
		signalID, msgID, now)
	if err != nil {
		return fmt.Errorf("set message_id: %w", err)
	}
	return nil
}

// directSetStatus is a test-only helper for seeding from-states in FSM tests.
func (s *SignalMarkStore) directSetStatus(signalID int64, status string) error {
	now := time.Now().Unix()
	_, err := s.db.conn.Exec(`
		INSERT INTO signal_marks (signal_id, status, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(signal_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`,
		signalID, status, now)
	return err
}

// nextFSMState returns (newStatus, deleteRow, error).
func nextFSMState(from, action string) (string, bool, error) {
	switch from {
	case "":
		switch action {
		case "took":
			return "TOOK", false, nil
		case "skip":
			return "SKIPPED", false, nil
		default:
			return "", false, fmt.Errorf("invalid transition: (no row) → %q", action)
		}
	case "TOOK":
		switch action {
		case "win":
			return "WIN", false, nil
		case "loss":
			return "LOSS", false, nil
		case "be":
			return "BE", false, nil
		case "undo":
			return "", true, nil
		default:
			return "", false, fmt.Errorf("invalid transition: TOOK → %q", action)
		}
	case "SKIPPED":
		if action == "undo" {
			return "", true, nil
		}
		return "", false, fmt.Errorf("invalid transition: SKIPPED → %q", action)
	case "WIN", "LOSS", "BE":
		switch action {
		case "win":
			return "WIN", false, nil
		case "loss":
			return "LOSS", false, nil
		case "be":
			return "BE", false, nil
		case "undo":
			return "TOOK", false, nil
		default:
			return "", false, fmt.Errorf("invalid transition: %s → %q", from, action)
		}
	}
	return "", false, fmt.Errorf("invalid transition: unknown from-state %q", from)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/storage/ -run TestSignalMarkStore -count=1 -race -v
```

Expected: All FSM cases + Get + SetMessageID PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/signal_marks.go internal/storage/signal_marks_test.go
git commit -m "feat(storage): SignalMarkStore with FSM transitions

Lazy-create semantics (no row = implicit PENDING). Mark() validates
state transitions per spec §3.2. SetMessageID stub-creates a PENDING
row when called before any user mark, so the Telegram message_id is
captured even if the user never marks the alert."
```

---

### Task A4: List queries (Pending + Marked)

**Files:**
- Modify: `internal/storage/signal_marks.go` (add ListPending, ListMarked)
- Modify: `internal/storage/signal_marks_test.go` (add tests)

- [ ] **Step 1: Write failing list tests**

Append to `internal/storage/signal_marks_test.go`:

```go
func TestSignalMarkStore_ListPending(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id1 := seedSignal(t, db)
	id2 := seedSignal(t, db)
	id3 := seedSignal(t, db)
	// id1 is marked TOOK, id2/id3 unmarked.
	if _, err := store.Mark(id1, "took"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListPending(time.Time{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(pending) = %d, want 2 (id2,id3)", len(rows))
	}
}

func TestSignalMarkStore_ListMarked(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id1 := seedSignal(t, db)
	id2 := seedSignal(t, db)
	_, _ = store.Mark(id1, "took")
	_, _ = store.Mark(id1, "win")
	_, _ = store.Mark(id2, "skip")

	rows, err := store.ListMarked(time.Time{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(marked) = %d, want 2", len(rows))
	}
	// Check status values present.
	statuses := map[string]bool{}
	for _, r := range rows {
		statuses[r.Mark.Status] = true
	}
	if !statuses["WIN"] || !statuses["SKIPPED"] {
		t.Errorf("expected WIN and SKIPPED in results, got %v", statuses)
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/storage/ -run "TestSignalMarkStore_ListPending|TestSignalMarkStore_ListMarked" -count=1
```

Expected: FAIL with `undefined: ListPending`, etc.

- [ ] **Step 3: Implement list queries**

Append to `internal/storage/signal_marks.go`:

```go
import "time"  // ensure time is imported

// SignalMarkRow joins a mark with its underlying signal for list queries.
type SignalMarkRow struct {
	Signal models.Signal // Symbol, Timeframe, Rule, Direction, Score, CreatedAt populated
	Mark   *SignalMark   // nil for pending rows
}

// ListPending returns signals with no mark row, created at or after `since`.
// Pass time.Time{} (zero) to disable the filter.
func (s *SignalMarkStore) ListPending(since time.Time, limit int) ([]SignalMarkRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sinceUnix := int64(0)
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}
	rows, err := s.db.conn.Query(`
		SELECT s.id, s.symbol, s.timeframe, s.rule, s.direction, s.score, s.message, s.created_at
		FROM signals s
		LEFT JOIN signal_marks m ON m.signal_id = s.id
		WHERE m.signal_id IS NULL AND s.created_at >= ?
		ORDER BY s.created_at DESC
		LIMIT ?`, sinceUnix, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	defer rows.Close()
	out := []SignalMarkRow{}
	for rows.Next() {
		var sig models.Signal
		var createdUnix int64
		if err := rows.Scan(&sig.ID, &sig.Symbol, &sig.Timeframe, &sig.Rule, &sig.Direction, &sig.Score, &sig.Message, &createdUnix); err != nil {
			return nil, err
		}
		sig.CreatedAt = time.Unix(createdUnix, 0).UTC()
		out = append(out, SignalMarkRow{Signal: sig, Mark: nil})
	}
	return out, rows.Err()
}

// ListMarked returns marked signals (any status) with updated_at >= since.
func (s *SignalMarkStore) ListMarked(since time.Time, limit int) ([]SignalMarkRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sinceUnix := int64(0)
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}
	rows, err := s.db.conn.Query(`
		SELECT s.id, s.symbol, s.timeframe, s.rule, s.direction, s.score, s.message, s.created_at,
		       m.status, m.took_at, m.outcome_at, m.tg_message_id, m.updated_at
		FROM signal_marks m
		JOIN signals s ON s.id = m.signal_id
		WHERE m.updated_at >= ?
		ORDER BY m.updated_at DESC
		LIMIT ?`, sinceUnix, limit)
	if err != nil {
		return nil, fmt.Errorf("list marked: %w", err)
	}
	defer rows.Close()
	out := []SignalMarkRow{}
	for rows.Next() {
		var sig models.Signal
		var createdUnix int64
		var mark SignalMark
		var took, outc, msg sql.NullInt64
		if err := rows.Scan(&sig.ID, &sig.Symbol, &sig.Timeframe, &sig.Rule, &sig.Direction, &sig.Score, &sig.Message, &createdUnix, &mark.Status, &took, &outc, &msg, &mark.UpdatedAt); err != nil {
			return nil, err
		}
		sig.CreatedAt = time.Unix(createdUnix, 0).UTC()
		mark.SignalID = sig.ID
		if took.Valid {
			v := took.Int64
			mark.TookAt = &v
		}
		if outc.Valid {
			v := outc.Int64
			mark.OutcomeAt = &v
		}
		if msg.Valid {
			v := msg.Int64
			mark.TgMessageID = &v
		}
		out = append(out, SignalMarkRow{Signal: sig, Mark: &mark})
	}
	return out, rows.Err()
}
```

Add `"github.com/Ju571nK/Chatter/pkg/models"` to imports if not already there.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/storage/ -run TestSignalMarkStore -count=1 -race -v
```

Expected: All tests including the two new ones PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/signal_marks.go internal/storage/signal_marks_test.go
git commit -m "feat(storage): ListPending + ListMarked queries

ListPending uses LEFT JOIN to find signals without a mark row.
ListMarked joins mark + signal in one query for the My Trades tab."
```

---

## Phase B — Aggregator

### Task B1: Rollup queries

**Files:**
- Create: `internal/marks/aggregator.go`
- Create: `internal/marks/aggregator_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/marks/aggregator_test.go`:

```go
package marks

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/storage"
)

func newAgg(t *testing.T) (*storage.DB, *Aggregator) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agg.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := storage.NewSignalMarkStore(db)
	return db, NewAggregator(db, store)
}

// seedAndMark inserts a signal and applies a sequence of mark actions.
func seedAndMark(t *testing.T, db *storage.DB, store *storage.SignalMarkStore, rule, symbol, tf string, actions ...string) {
	t.Helper()
	_, err := db.Conn().Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at)
		VALUES (?, ?, ?, 'LONG', 10.0, '', '', 0, 0, '', 0, ?)`,
		symbol, tf, rule, time.Now().Unix())
	if err != nil {
		t.Fatalf("seed signal: %v", err)
	}
	var id int64
	_ = db.Conn().QueryRow(`SELECT id FROM signals ORDER BY id DESC LIMIT 1`).Scan(&id)
	for _, a := range actions {
		if _, err := store.Mark(id, a); err != nil {
			t.Fatalf("mark %q: %v", a, err)
		}
	}
}

func TestAggregator_RollupByRule(t *testing.T) {
	db, agg := newAgg(t)
	store := storage.NewSignalMarkStore(db)

	// Rule A: 3 took, 2 win, 1 loss; 1 skip. HitRate = 2/3, SkipRate = 1/4.
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "took", "loss")
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "skip")
	// Rule B: 1 took, 1 BE.
	seedAndMark(t, db, store, "rule_b", "ETH", "4H", "took", "be")

	rows, err := agg.Rollup(GroupByRule, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	byKey := map[string]RollupRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	a := byKey["rule_a"]
	if a.Took != 3 || a.Wins != 2 || a.Losses != 1 || a.Skipped != 1 {
		t.Errorf("rule_a counts wrong: %+v", a)
	}
	wantHit := 2.0 / 3.0
	if abs(a.HitRate-wantHit) > 0.001 {
		t.Errorf("rule_a HitRate = %.3f, want %.3f", a.HitRate, wantHit)
	}
	if abs(a.SkipRate-0.25) > 0.001 {
		t.Errorf("rule_a SkipRate = %.3f, want 0.25", a.SkipRate)
	}

	b := byKey["rule_b"]
	if b.Took != 1 || b.BreakEvens != 1 || b.HitRate != 0.0 {
		t.Errorf("rule_b: %+v (BE in denominator only, HitRate should be 0)", b)
	}
}

func TestAggregator_RollupBySymbol(t *testing.T) {
	db, agg := newAgg(t)
	store := storage.NewSignalMarkStore(db)
	seedAndMark(t, db, store, "rule_x", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "rule_y", "BTC", "1H", "took", "loss")
	seedAndMark(t, db, store, "rule_x", "ETH", "4H", "skip")

	rows, _ := agg.Rollup(GroupBySymbol, time.Time{})
	byKey := map[string]RollupRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	if byKey["BTC"].Took != 2 || byKey["ETH"].Skipped != 1 {
		t.Errorf("symbol rollup wrong: %+v", byKey)
	}
}

func TestAggregator_RollupByMethodology(t *testing.T) {
	db, agg := newAgg(t)
	store := storage.NewSignalMarkStore(db)
	seedAndMark(t, db, store, "ict_order_block", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "wyckoff_spring", "BTC", "1H", "took", "loss")

	rows, _ := agg.Rollup(GroupByMethodology, time.Time{})
	byKey := map[string]RollupRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	if byKey["ict"].Wins != 1 {
		t.Errorf("ict wins = %d, want 1", byKey["ict"].Wins)
	}
	if byKey["wyckoff"].Losses != 1 {
		t.Errorf("wyckoff losses = %d, want 1", byKey["wyckoff"].Losses)
	}
}

func TestAggregator_EmptyResult(t *testing.T) {
	_, agg := newAgg(t)
	rows, err := agg.Rollup(GroupByRule, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("empty DB should return empty rollup, got %d rows", len(rows))
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/marks/ -count=1
```

Expected: FAIL with `undefined: NewAggregator`, etc.

- [ ] **Step 3: Implement Aggregator**

Create `internal/marks/aggregator.go`:

```go
// Package marks computes rolled-up statistics from the signal_marks table.
package marks

import (
	"fmt"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
)

// GroupBy controls the dimension Rollup aggregates over.
type GroupBy string

const (
	GroupByRule        GroupBy = "rule"
	GroupBySymbol      GroupBy = "symbol"
	GroupByMethodology GroupBy = "methodology"
	GroupByTimeframe   GroupBy = "timeframe"
)

// RollupRow is one row of aggregated personal stats.
type RollupRow struct {
	Key        string  `json:"key"`
	Took       int     `json:"took"`
	Skipped    int     `json:"skipped"`
	Wins       int     `json:"wins"`
	Losses     int     `json:"losses"`
	BreakEvens int     `json:"bes"`
	HitRate    float64 `json:"hit_rate"`
	SkipRate   float64 `json:"skip_rate"`
}

// Aggregator computes rollup stats from signal_marks.
type Aggregator struct {
	db    *storage.DB
	store *storage.SignalMarkStore
}

// NewAggregator creates an aggregator backed by the given DB.
func NewAggregator(db *storage.DB, store *storage.SignalMarkStore) *Aggregator {
	return &Aggregator{db: db, store: store}
}

// Rollup returns aggregated counts grouped by the given dimension.
// Pass time.Time{} (zero) to include all marks regardless of date.
func (a *Aggregator) Rollup(by GroupBy, since time.Time) ([]RollupRow, error) {
	sinceUnix := int64(0)
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}

	switch by {
	case GroupByRule:
		return a.rollupByColumn("s.rule", sinceUnix)
	case GroupBySymbol:
		return a.rollupByColumn("s.symbol", sinceUnix)
	case GroupByTimeframe:
		return a.rollupByColumn("s.timeframe", sinceUnix)
	case GroupByMethodology:
		return a.rollupByMethodology(sinceUnix)
	default:
		return nil, fmt.Errorf("unknown groupBy: %q", by)
	}
}

func (a *Aggregator) rollupByColumn(col string, sinceUnix int64) ([]RollupRow, error) {
	q := fmt.Sprintf(`
		SELECT %s,
		       SUM(CASE WHEN m.status IN ('TOOK','WIN','LOSS','BE') THEN 1 ELSE 0 END) AS took,
		       SUM(CASE WHEN m.status = 'SKIPPED' THEN 1 ELSE 0 END) AS skipped,
		       SUM(CASE WHEN m.status = 'WIN'  THEN 1 ELSE 0 END) AS wins,
		       SUM(CASE WHEN m.status = 'LOSS' THEN 1 ELSE 0 END) AS losses,
		       SUM(CASE WHEN m.status = 'BE'   THEN 1 ELSE 0 END) AS bes
		FROM signal_marks m
		JOIN signals s ON s.id = m.signal_id
		WHERE m.updated_at >= ?
		GROUP BY %s
		ORDER BY took DESC`, col, col)

	rows, err := a.db.Conn().Query(q, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("rollup query: %w", err)
	}
	defer rows.Close()
	out := []RollupRow{}
	for rows.Next() {
		var r RollupRow
		if err := rows.Scan(&r.Key, &r.Took, &r.Skipped, &r.Wins, &r.Losses, &r.BreakEvens); err != nil {
			return nil, err
		}
		r.HitRate, r.SkipRate = computeRates(r.Wins, r.Losses, r.BreakEvens, r.Took, r.Skipped)
		out = append(out, r)
	}
	return out, rows.Err()
}

// rollupByMethodology runs a rule-level rollup then re-aggregates by methodology
// using RuleMethodology (no SQL UDF available in modernc.org/sqlite).
func (a *Aggregator) rollupByMethodology(sinceUnix int64) ([]RollupRow, error) {
	ruleRows, err := a.rollupByColumn("s.rule", sinceUnix)
	if err != nil {
		return nil, err
	}
	merged := map[string]*RollupRow{}
	for _, r := range ruleRows {
		method := appconfig.RuleMethodology(r.Key)
		m := merged[method]
		if m == nil {
			m = &RollupRow{Key: method}
			merged[method] = m
		}
		m.Took += r.Took
		m.Skipped += r.Skipped
		m.Wins += r.Wins
		m.Losses += r.Losses
		m.BreakEvens += r.BreakEvens
	}
	out := []RollupRow{}
	for _, m := range merged {
		m.HitRate, m.SkipRate = computeRates(m.Wins, m.Losses, m.BreakEvens, m.Took, m.Skipped)
		out = append(out, *m)
	}
	return out, nil
}

// computeRates returns (hitRate, skipRate). Both clamped to 0 when denominator is 0.
// HitRate = wins / (wins+losses+bes). SkipRate = skipped / (took+skipped).
func computeRates(wins, losses, bes, took, skipped int) (float64, float64) {
	hit := 0.0
	if d := wins + losses + bes; d > 0 {
		hit = float64(wins) / float64(d)
	}
	skip := 0.0
	if d := took + skipped; d > 0 {
		skip = float64(skipped) / float64(d)
	}
	return hit, skip
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/marks/ -count=1 -race -v
```

Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/marks/aggregator.go internal/marks/aggregator_test.go
git commit -m "feat(marks): Aggregator with 4-way rollup

GROUP BY rule/symbol/timeframe via SQL; methodology via Go
post-aggregation using existing RuleMethodology mapper. HitRate
puts BE in denominator only (real exit, zero P&L)."
```

---

## Phase C — HTTP API

### Task C1: Marks handlers (TDD)

**Files:**
- Create: `internal/api/marks_handler.go`
- Create: `internal/api/marks_handler_test.go`
- Modify: `internal/api/server.go` (add fields + setters + routes)

- [ ] **Step 1: Write failing handler tests**

Create `internal/api/marks_handler_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Ju571nK/Chatter/internal/marks"
	"github.com/Ju571nK/Chatter/internal/storage"
)

func newMarksTestServer(t *testing.T, apiToken string) (*Server, *storage.DB, *storage.SignalMarkStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "marks.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := storage.NewSignalMarkStore(db)
	agg := marks.NewAggregator(db, store)
	s := &Server{
		apiToken:   apiToken,
		markStore:  store,
		aggregator: agg,
	}
	return s, db, store
}

func seedSignalForAPI(t *testing.T, db *storage.DB) int64 {
	t.Helper()
	_, err := db.Conn().Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at)
		VALUES ('BTCUSDT','1H','ict_order_block','LONG',10.0,'','',0,0,'',0,0)`)
	if err != nil {
		t.Fatalf("seed signal: %v", err)
	}
	var id int64
	_ = db.Conn().QueryRow(`SELECT id FROM signals ORDER BY id DESC LIMIT 1`).Scan(&id)
	return id
}

func TestPostMark_Took(t *testing.T) {
	s, db, store := newMarksTestServer(t, "")
	id := seedSignalForAPI(t, db)
	body := []byte(`{"action":"took"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	got, _ := store.Get(id)
	if got == nil || got.Status != "TOOK" {
		t.Errorf("DB state = %#v, want status TOOK", got)
	}
}

func TestPostMark_InvalidAction(t *testing.T) {
	s, db, _ := newMarksTestServer(t, "")
	id := seedSignalForAPI(t, db)
	body := []byte(`{"action":"explode"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPostMark_UnknownSignal(t *testing.T) {
	s, _, _ := newMarksTestServer(t, "")
	body := []byte(`{"action":"took"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/99999", bytes.NewReader(body))
	req.SetPathValue("signal_id", "99999")
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestPostMark_AuthRequired(t *testing.T) {
	s, db, _ := newMarksTestServer(t, "secret-token")
	id := seedSignalForAPI(t, db)
	body := []byte(`{"action":"took"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", w.Code)
	}
	// With valid token.
	req2 := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req2.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	req2.Header.Set("Authorization", "Bearer secret-token")
	w2 := httptest.NewRecorder()
	s.postMark(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("with token: status = %d, want 200", w2.Code)
	}
}

func TestGetPending(t *testing.T) {
	s, db, store := newMarksTestServer(t, "")
	id1 := seedSignalForAPI(t, db)
	id2 := seedSignalForAPI(t, db)
	_, _ = store.Mark(id1, "took") // id1 marked, id2 unmarked

	req := httptest.NewRequest(http.MethodGet, "/api/marks/pending", nil)
	w := httptest.NewRecorder()
	s.getPending(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Fatalf("len(pending) = %d, want 1", len(resp))
	}
	sig := resp[0]["signal"].(map[string]any)
	if int64(sig["id"].(float64)) != id2 {
		t.Errorf("expected unmarked id %d, got %v", id2, sig["id"])
	}
}

func TestGetRollup(t *testing.T) {
	s, db, store := newMarksTestServer(t, "")
	id := seedSignalForAPI(t, db)
	_, _ = store.Mark(id, "took")
	_, _ = store.Mark(id, "win")

	req := httptest.NewRequest(http.MethodGet, "/api/marks/rollup?by=rule", nil)
	w := httptest.NewRecorder()
	s.getRollup(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["by"] != "rule" {
		t.Errorf("by = %v, want rule", resp["by"])
	}
	rows := resp["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0].(map[string]any)
	if row["key"] != "ict_order_block" {
		t.Errorf("key = %v", row["key"])
	}
	if int(row["wins"].(float64)) != 1 {
		t.Errorf("wins = %v", row["wins"])
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/api/ -run "TestPostMark|TestGetPending|TestGetRollup" -count=1
```

Expected: FAIL — `s.markStore`, `s.aggregator`, `s.postMark` undefined.

- [ ] **Step 3: Add fields and setters to Server**

In `internal/api/server.go`, add to `Server` struct (next to other store fields):

```go
markStore  *storage.SignalMarkStore // optional; set via WithMarkStore
aggregator *marks.Aggregator        // optional; set via WithAggregator
```

Add the imports: `"github.com/Ju571nK/Chatter/internal/marks"` if not present.

Add setters (matching existing `WithXxx` style):

```go
func (s *Server) WithMarkStore(store *storage.SignalMarkStore) *Server {
	s.markStore = store
	return s
}

func (s *Server) WithAggregator(a *marks.Aggregator) *Server {
	s.aggregator = a
	return s
}
```

- [ ] **Step 4: Implement handlers**

Create `internal/api/marks_handler.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/internal/marks"
)

var validActions = map[string]struct{}{
	"took": {}, "skip": {}, "win": {}, "loss": {}, "be": {}, "undo": {},
}

type markRequest struct {
	Action string `json:"action"`
}

type markResponse struct {
	SignalID  int64  `json:"signal_id"`
	Status    string `json:"status"` // "" when row deleted (after undo)
	UpdatedAt int64  `json:"updated_at"`
}

func (s *Server) postMark(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.markStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mark store not configured")
		return
	}
	idStr := r.PathValue("signal_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid signal_id")
		return
	}

	var req markRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if _, ok := validActions[req.Action]; !ok {
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("invalid action: %q (must be took|skip|win|loss|be|undo)", req.Action))
		return
	}

	// Verify signal exists.
	var exists int
	if err := s.dbHandle().QueryRow(`SELECT 1 FROM signals WHERE id = ?`, id).Scan(&exists); err != nil {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("signal not found: %d", id))
		return
	}

	newStatus, err := s.markStore.Mark(id, req.Action)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("mark: %v", err))
		return
	}
	jsonOK(w, markResponse{SignalID: id, Status: newStatus, UpdatedAt: time.Now().Unix()})
}

func (s *Server) getPending(w http.ResponseWriter, r *http.Request) {
	if s.markStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mark store not configured")
		return
	}
	since, limit := parseSinceLimit(r, 24*time.Hour, 50)
	rows, err := s.markStore.ListPending(since, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, rows)
}

func (s *Server) getRecent(w http.ResponseWriter, r *http.Request) {
	if s.markStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mark store not configured")
		return
	}
	since, limit := parseSinceLimit(r, 30*24*time.Hour, 50)
	rows, err := s.markStore.ListMarked(since, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, rows)
}

type rollupResponse struct {
	By    string             `json:"by"`
	Since string             `json:"since"`
	Rows  []marks.RollupRow  `json:"rows"`
}

func (s *Server) getRollup(w http.ResponseWriter, r *http.Request) {
	if s.aggregator == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "aggregator not configured")
		return
	}
	by := r.URL.Query().Get("by")
	if by == "" {
		by = "rule"
	}
	since, _ := parseSinceLimit(r, 30*24*time.Hour, 50)
	rows, err := s.aggregator.Rollup(marks.GroupBy(by), since)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "max-age=60")
	jsonOK(w, rollupResponse{By: by, Since: since.UTC().Format(time.RFC3339), Rows: rows})
}

// parseSinceLimit reads `since` (ISO 8601) and `limit` query params with defaults.
func parseSinceLimit(r *http.Request, defaultSinceAgo time.Duration, defaultLimit int) (time.Time, int) {
	sinceStr := r.URL.Query().Get("since")
	since := time.Now().Add(-defaultSinceAgo)
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}
	limit := defaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	return since, limit
}

// dbHandle is a small accessor returning the *sql.DB so handler can verify FK existence
// without exposing the storage layer further. Server holds the *storage.DB elsewhere
// (via markStore.db) but here we accept that markStore is the only DB injection
// point in marks-related handlers; expose by adding a getter on SignalMarkStore.
func (s *Server) dbHandle() interface {
	QueryRow(query string, args ...any) *sql.Row
} {
	// markStore was checked non-nil by the caller.
	return s.markStore.DB().Conn()
}

// Note: SignalMarkStore.DB() needs to be added — Step 5.
```

Wait — that last block has a forward dependency. Let me restructure.

Actually the cleanest approach: pass a separate dependency or use the existing `storage.DB` injection. Looking at the existing `Server` fields like `execDB *sql.DB`, the codebase already passes `*sql.DB` into the server for verification queries. Let me use the same pattern.

Replace the dbHandle helper with: add a new `markDB *sql.DB` field set via `WithMarkStore` (have the setter capture both store and db handle):

In `internal/api/server.go`, modify `WithMarkStore`:

```go
func (s *Server) WithMarkStore(store *storage.SignalMarkStore) *Server {
	s.markStore = store
	return s
}
```

Then change the handler to verify existence via the store rather than direct SQL:

In the handler, replace the existence check:

```go
// Verify signal exists.
var exists int
if err := s.dbHandle().QueryRow(...).Scan(&exists); err != nil { ... }
```

with: add a helper on SignalMarkStore.

In `internal/storage/signal_marks.go` append:

```go
// SignalExists returns true if a signal with the given id is in the signals table.
// Used by the API layer to validate FK before attempting Mark.
func (s *SignalMarkStore) SignalExists(signalID int64) (bool, error) {
	var n int
	err := s.db.conn.QueryRow(`SELECT 1 FROM signals WHERE id = ?`, signalID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("signal exists: %w", err)
	}
	return true, nil
}
```

And in `internal/api/marks_handler.go` replace the existence-check block with:

```go
	exists, err := s.markStore.SignalExists(id)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("signal not found: %d", id))
		return
	}
```

Remove the `dbHandle()` helper and the trailing comment about it. Also remove the `database/sql` import in marks_handler.go since it's no longer used.

- [ ] **Step 5: Register routes**

In `internal/api/server.go`, locate the `Handler()` function. Add 4 routes near the symbols/profiles group:

```go
mux.HandleFunc("POST /api/marks/{signal_id}", s.postMark)
mux.HandleFunc("GET /api/marks/pending",      s.getPending)
mux.HandleFunc("GET /api/marks/recent",       s.getRecent)
mux.HandleFunc("GET /api/marks/rollup",       s.getRollup)
```

- [ ] **Step 6: Run handler tests**

```bash
go test ./internal/api/ -run "TestPostMark|TestGetPending|TestGetRollup" -count=1 -race -v
```

Expected: All 6 tests PASS.

- [ ] **Step 7: Run full Go suite**

```bash
go test ./... -count=1 -race 2>&1 | tail -10
go build ./...
```

Expected: PASS + clean.

- [ ] **Step 8: Commit**

```bash
git add internal/api/server.go internal/api/marks_handler.go internal/api/marks_handler_test.go internal/storage/signal_marks.go
git commit -m "feat(api): GET/POST /api/marks/* endpoints

POST validates action enum + FK + FSM transitions. GET pending uses
LEFT JOIN to find unmarked signals. GET rollup proxies to Aggregator.
GET responses are unauthenticated (consistent with other GETs);
POST goes through the existing requireBearer middleware."
```

---

## Phase D — Telegram Bot Extension

### Task D1: tgUpdate types + callback handler (TDD)

**Files:**
- Modify: `internal/notifier/telegram_bot.go`
- Create: `internal/notifier/telegram_bot_callback_test.go`

- [ ] **Step 1: Write failing callback test**

Create `internal/notifier/telegram_bot_callback_test.go`:

```go
package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeMarkStore captures Mark/SetMessageID calls for verification.
type fakeMarkStore struct {
	mu       sync.Mutex
	calls    []string
	nextErr  error
	nextStat string
}

func (f *fakeMarkStore) Mark(signalID int64, action string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, action)
	if f.nextErr != nil {
		return "", f.nextErr
	}
	if f.nextStat != "" {
		return f.nextStat, nil
	}
	return strings.ToUpper(action), nil
}

func TestTelegramBot_HandleCallback_RoutesToMarkStore(t *testing.T) {
	// Stub Telegram API server so we can observe outbound calls.
	var apiCalls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls = append(apiCalls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	store := &fakeMarkStore{nextStat: "TOOK"}
	bot := &TelegramBot{
		token:   "test",
		chatID:  "12345",
		client:  srv.Client(),
		mark:    store,
		baseURL: srv.URL + "/bot", // injected for tests
	}

	cb := &tgCallbackQuery{
		ID:   "cb1",
		Data: "took:42",
		From: tgUser{ID: 7},
		Message: &tgMessage{
			MessageID: 100,
			Text:      "🔔 BTCUSDT alert",
			Chat:      tgChat{ID: 12345},
		},
	}
	bot.handleCallback(context.Background(), cb)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.calls) != 1 || store.calls[0] != "took" {
		t.Fatalf("Mark calls = %v, want [took]", store.calls)
	}
	// Expect three API calls: answerCallbackQuery + editMessageText + editMessageReplyMarkup.
	if len(apiCalls) < 2 {
		t.Errorf("apiCalls = %v, want >= 2", apiCalls)
	}
}

func TestTelegramBot_HandleCallback_RejectsWrongChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("should not call Telegram API for wrong chat")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &fakeMarkStore{}
	bot := &TelegramBot{
		token: "test", chatID: "12345", client: srv.Client(), mark: store,
		baseURL: srv.URL + "/bot",
	}
	cb := &tgCallbackQuery{
		ID:      "cb1",
		Data:    "took:42",
		Message: &tgMessage{MessageID: 100, Chat: tgChat{ID: 99999}}, // wrong chat
	}
	bot.handleCallback(context.Background(), cb)
	if len(store.calls) != 0 {
		t.Errorf("Mark called for wrong chat: %v", store.calls)
	}
}

func TestParseCallbackData(t *testing.T) {
	cases := []struct {
		in        string
		wantAct   string
		wantID    int64
		wantErr   bool
	}{
		{"took:42", "took", 42, false},
		{"win:123456", "win", 123456, false},
		{"undo:1", "undo", 1, false},
		{"explode:1", "", 0, true},
		{"took:notanumber", "", 0, true},
		{"malformed", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			act, id, err := parseCallbackData(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected err, got act=%q id=%d", act, id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if act != tc.wantAct || id != tc.wantID {
				t.Errorf("got (%q,%d), want (%q,%d)", act, id, tc.wantAct, tc.wantID)
			}
		})
	}
}

// Decode helper to make the inline keyboard JSON inspectable.
func decodeKeyboard(t *testing.T, body []byte) [][]map[string]string {
	t.Helper()
	var payload struct {
		InlineKeyboard [][]map[string]string `json:"inline_keyboard"`
	}
	_ = json.Unmarshal(body, &payload)
	return payload.InlineKeyboard
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/notifier/ -run "TestTelegramBot_HandleCallback|TestParseCallbackData" -count=1
```

Expected: FAIL — `tgCallbackQuery`, `tgUser`, `tgChat`, `parseCallbackData`, `bot.mark`, `bot.baseURL`, `bot.handleCallback` undefined.

- [ ] **Step 3: Extend telegram_bot.go**

In `internal/notifier/telegram_bot.go`:

(a) Add the missing types at the bottom of the existing struct definitions:

```go
type tgChat struct {
	ID int64 `json:"id"`
}

type tgUser struct {
	ID int64 `json:"id"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	Data    string     `json:"data"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message"`
}
```

Update the existing `tgUpdate` to add the `CallbackQuery` field:

```go
type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}
```

Update the existing `tgMessage` to add `MessageID` and use `tgChat`:

```go
type tgMessage struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      tgChat `json:"chat"`
}
```

(b) Add `MarkStore` interface and extend the bot struct:

```go
// MarkStore is the storage interface the bot consumes for marking signals.
// *storage.SignalMarkStore satisfies it.
type MarkStore interface {
	Mark(signalID int64, action string) (newStatus string, err error)
}

// TelegramBot fields — add these:
type TelegramBot struct {
	token   string
	chatID  string
	client  *http.Client
	handler AnalysisHandler
	mark    MarkStore // optional; nil disables callback handling
	baseURL string    // override for tests; defaults to "https://api.telegram.org/bot"
}
```

Update the constructor to accept `MarkStore` and set `baseURL`:

```go
func NewTelegramBot(token, chatID string, handler AnalysisHandler, mark MarkStore) *TelegramBot {
	return &TelegramBot{
		token:   token,
		chatID:  chatID,
		client:  &http.Client{Timeout: 40 * time.Second},
		handler: handler,
		mark:    mark,
		baseURL: "https://api.telegram.org/bot",
	}
}
```

(c) Update `handleUpdate` to dispatch CallbackQuery:

```go
func (b *TelegramBot) handleUpdate(ctx context.Context, u tgUpdate) {
	if u.CallbackQuery != nil {
		b.handleCallback(ctx, u.CallbackQuery)
		return
	}
	if u.Message == nil {
		return
	}
	if fmt.Sprintf("%d", u.Message.Chat.ID) != b.chatID {
		return
	}
	// existing /analysis routing — keep unchanged
	// (paste the existing body here if it was below)
}
```

(d) Add the new helpers:

```go
// handleCallback processes a callback_query: parse data, mark, edit message, answer.
func (b *TelegramBot) handleCallback(ctx context.Context, cb *tgCallbackQuery) {
	if cb.Message == nil {
		return
	}
	if fmt.Sprintf("%d", cb.Message.Chat.ID) != b.chatID {
		return
	}
	if b.mark == nil {
		_ = b.answerCallback(ctx, cb.ID, "Marking disabled on this server")
		return
	}
	action, signalID, err := parseCallbackData(cb.Data)
	if err != nil {
		_ = b.answerCallback(ctx, cb.ID, "Invalid action")
		return
	}
	newStatus, err := b.mark.Mark(signalID, action)
	if err != nil {
		log.Warn().Err(err).Int64("signal_id", signalID).Str("action", action).Msg("[TelegramBot] mark failed")
		_ = b.answerCallback(ctx, cb.ID, "❌ Save failed")
		return
	}
	_ = b.answerCallback(ctx, cb.ID, "✓ "+statusLabel(newStatus))
	// Edit keyboard + append status line to text. Failures are non-fatal.
	if err := b.editKeyboard(ctx, cb.Message.Chat.ID, cb.Message.MessageID, KeyboardForStatus(newStatus, signalID)); err != nil {
		log.Warn().Err(err).Msg("[TelegramBot] editMessageReplyMarkup failed")
	}
	if err := b.appendStatusLine(ctx, cb.Message.Chat.ID, cb.Message.MessageID, cb.Message.Text, newStatus); err != nil {
		log.Warn().Err(err).Msg("[TelegramBot] editMessageText failed")
	}
}

// parseCallbackData splits "{action}:{signal_id}" into (action, id).
func parseCallbackData(data string) (string, int64, error) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("malformed callback_data: %q", data)
	}
	switch parts[0] {
	case "took", "skip", "win", "loss", "be", "undo":
	default:
		return "", 0, fmt.Errorf("unknown action: %q", parts[0])
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid signal_id: %q", parts[1])
	}
	return parts[0], id, nil
}

func statusLabel(status string) string {
	switch status {
	case "TOOK":
		return "Took"
	case "SKIPPED":
		return "Skipped"
	case "WIN":
		return "Win"
	case "LOSS":
		return "Loss"
	case "BE":
		return "BE"
	case "":
		return "Reset"
	default:
		return status
	}
}

// answerCallback sends answerCallbackQuery to dismiss the spinner.
func (b *TelegramBot) answerCallback(ctx context.Context, callbackID, text string) error {
	body := map[string]any{"callback_query_id": callbackID, "text": text}
	return b.postTG(ctx, "answerCallbackQuery", body)
}

// editKeyboard sends editMessageReplyMarkup with the given inline keyboard.
func (b *TelegramBot) editKeyboard(ctx context.Context, chatID int64, messageID int64, keyboard any) error {
	if keyboard == nil {
		return nil
	}
	body := map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"reply_markup": keyboard,
	}
	return b.postTG(ctx, "editMessageReplyMarkup", body)
}

// appendStatusLine edits the message text to append a status journal line.
// Idempotency: if the existing text already contains "✓ {label}", do nothing.
func (b *TelegramBot) appendStatusLine(ctx context.Context, chatID int64, messageID int64, currentText string, newStatus string) error {
	label := statusLabel(newStatus)
	stamp := time.Now().Format("15:04 MST")
	addition := fmt.Sprintf("✓ %s at %s", label, stamp)
	if newStatus == "WIN" || newStatus == "LOSS" || newStatus == "BE" {
		addition = "→ " + map[string]string{"WIN": "💰", "LOSS": "💸", "BE": "⚖"}[newStatus] + " " + label + " at " + stamp
	}
	newText := currentText
	if !strings.Contains(newText, addition) {
		newText = newText + "\n" + addition
	}
	body := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       newText,
		"parse_mode": "HTML",
	}
	return b.postTG(ctx, "editMessageText", body)
}

// postTG sends a JSON POST to baseURL/{token}/{method}.
func (b *TelegramBot) postTG(ctx context.Context, method string, body map[string]any) error {
	buf, _ := json.Marshal(body)
	url := fmt.Sprintf("%s%s/%s", b.baseURL, b.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram %s returned %d: %s", method, resp.StatusCode, string(bb))
	}
	return nil
}

// KeyboardForStatus returns the inline keyboard appropriate for the new status,
// embedding the signal_id in callback_data.
func KeyboardForStatus(status string, signalID int64) any {
	idStr := strconv.FormatInt(signalID, 10)
	switch status {
	case "TOOK":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "💰 Win", "callback_data": "win:" + idStr},
			{"text": "💸 Loss", "callback_data": "loss:" + idStr},
			{"text": "⚖ BE", "callback_data": "be:" + idStr},
			{"text": "↺ Undo", "callback_data": "undo:" + idStr},
		}}}
	case "SKIPPED":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "↺ Undo", "callback_data": "undo:" + idStr},
		}}}
	case "WIN", "LOSS", "BE":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "↺ Edit", "callback_data": "undo:" + idStr},
		}}}
	case "PENDING", "":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "✅ Took", "callback_data": "took:" + idStr},
			{"text": "❌ Skipped", "callback_data": "skip:" + idStr},
		}}}
	}
	return nil
}
```

Imports needed: `bytes`, `encoding/json`, `io`, `strconv`, `strings`, `time` — add to existing import block.

(e) Update `getUpdates` URL to use `b.baseURL`:

```go
func (b *TelegramBot) getUpdates(ctx context.Context, offset int64, timeout int) ([]tgUpdate, error) {
	url := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=%d", b.baseURL, b.token, offset, timeout)
	// ... rest unchanged
}
```

- [ ] **Step 4: Update existing call sites**

```bash
grep -rn "NewTelegramBot\b" --include='*.go' | grep -v _test
```

Update each call site to pass a `MarkStore` (use `nil` for now in tests; production wiring comes in Task G2):

In `cmd/server/main.go` (or equivalent), find the existing `notifier.NewTelegramBot(token, chatID, handler)` call and add the new `mark` parameter:

```go
tgBot := notifier.NewTelegramBot(token, chatID, handler, markStore)
```

(Wiring `markStore` is in Phase G; for now use `nil` and the build will compile.)

If `markStore` is not yet declared at the call site, pass `nil`:

```go
tgBot := notifier.NewTelegramBot(token, chatID, handler, nil)
```

This temporary nil is replaced in Phase G.

- [ ] **Step 5: Run callback tests**

```bash
go test ./internal/notifier/ -run "TestTelegramBot_HandleCallback|TestParseCallbackData" -count=1 -race -v
```

Expected: All PASS.

- [ ] **Step 6: Run full Go suite**

```bash
go test ./... -count=1 -race 2>&1 | tail -8
go build ./...
```

Expected: PASS + clean.

- [ ] **Step 7: Commit**

```bash
git add internal/notifier/telegram_bot.go internal/notifier/telegram_bot_callback_test.go cmd/server/main.go
git commit -m "feat(notifier): Telegram callback_query handling for marking

handleCallback validates chat_id, parses callback_data, calls
MarkStore.Mark, then issues answerCallbackQuery + editMessageText
+ editMessageReplyMarkup. Failures of edit calls are logged-and-
ignored so DB state stays consistent even when message edit fails
(deleted message, etc).

KeyboardForStatus is exported so the alert sender can mount the
PENDING keyboard on initial dispatch in a follow-up commit."
```

---

### Task D2: Inline keyboard in alert send + message_id capture

**Files:**
- Modify: `internal/notifier/telegram.go`
- Modify: `internal/notifier/notifier.go`

- [ ] **Step 1: Inspect current sender to know edit points**

```bash
grep -n "func.*Send\|sendMessage\|formatTelegram" internal/notifier/telegram.go | head -10
```

Note the current `Send` signature and where the HTTP POST happens.

- [ ] **Step 2: Add inline keyboard + return message_id**

In `internal/notifier/telegram.go`, change the public Send method (likely `Send(sig models.Signal) error`) to include inline keyboard and return `(int64, error)`:

```go
// SendAlert posts an alert with inline marking keyboard and returns the message_id.
// The message_id allows the bot to edit the message later (callback handler).
func (s *TelegramSender) SendAlert(sig models.Signal) (int64, error) {
	text := formatTelegram(sig)
	keyboard := KeyboardForStatus("PENDING", sig.ID)

	body := map[string]any{
		"chat_id":      s.chatID,
		"text":         text,
		"parse_mode":   "HTML",
		"reply_markup": keyboard,
	}
	buf, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.token)
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		return 0, fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("telegram %d: %s", resp.StatusCode, string(bb))
	}
	var out struct {
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("decode send response: %w", err)
	}
	return out.Result.MessageID, nil
}
```

If the existing public method is `Send(sig models.Signal) error`, retain it as a thin wrapper that drops the message_id, so existing callers compile:

```go
func (s *TelegramSender) Send(sig models.Signal) error {
	_, err := s.SendAlert(sig)
	return err
}
```

- [ ] **Step 3: Update Notifier to capture message_id**

In `internal/notifier/notifier.go`, find the Telegram dispatch site. Replace:

```go
if err := s.telegram.Send(sig); err != nil { ... }
```

with:

```go
msgID, err := s.telegram.SendAlert(sig)
if err != nil {
	// existing error handling
}
if s.markStore != nil && msgID != 0 {
	if err := s.markStore.SetMessageID(sig.ID, msgID); err != nil {
		n.log.Warn().Err(err).Int64("signal_id", sig.ID).Msg("set message_id failed")
	}
}
```

Add a new field + setter on Notifier:

```go
markStore MarkStoreSet // optional

// MarkStoreSet is just SetMessageID — separate from MarkStore (which the bot uses).
// Defined here to avoid an import cycle.
type MarkStoreSet interface {
	SetMessageID(signalID int64, msgID int64) error
}

func (n *Notifier) WithMarkStore(s MarkStoreSet) *Notifier {
	n.markStore = s
	return n
}
```

- [ ] **Step 4: Build + test**

```bash
go build ./...
go test ./internal/notifier/ -count=1 -race
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/notifier/telegram.go internal/notifier/notifier.go
git commit -m "feat(notifier): inline keyboard on alert send + message_id capture

SendAlert returns the message_id so the Notifier can persist it via
MarkStoreSet.SetMessageID. The bot uses message_id later for
editMessageReplyMarkup. Existing Send wrapper retained for callers
that don't need the id."
```

---

## Phase E — MCP Tool

### Task E1: get_my_performance tool (TDD)

**Files:**
- Create: `internal/mcp/get_my_performance.go`
- Create: `internal/mcp/get_my_performance_test.go`

- [ ] **Step 1: Write failing tool test**

Create `internal/mcp/get_my_performance_test.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/marks"
)

type fakeRollupSource struct {
	rows  []marks.RollupRow
	err   error
	gotBy marks.GroupBy
}

func (f *fakeRollupSource) Rollup(by marks.GroupBy, since time.Time) ([]marks.RollupRow, error) {
	f.gotBy = by
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func TestGetMyPerformance_RendersTable(t *testing.T) {
	src := &fakeRollupSource{rows: []marks.RollupRow{
		{Key: "ict_liquidity_sweep", Took: 12, Skipped: 6, Wins: 8, Losses: 3, BreakEvens: 1, HitRate: 0.667, SkipRate: 0.333},
	}}
	tool := NewGetMyPerformance(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{"by":"rule","since_days":30}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "ict_liquidity_sweep") {
		t.Errorf("missing rule name: %q", text)
	}
	if !strings.Contains(text, "66.7%") {
		t.Errorf("missing hit rate %%: %q", text)
	}
	if src.gotBy != marks.GroupByRule {
		t.Errorf("got by = %q, want rule", src.gotBy)
	}
}

func TestGetMyPerformance_EmptyResult(t *testing.T) {
	src := &fakeRollupSource{rows: []marks.RollupRow{}}
	tool := NewGetMyPerformance(src)
	res, _ := tool.Call(context.Background(), json.RawMessage(`{"by":"rule"}`))
	text := res.Content[0].Text
	if !strings.Contains(text, "No marked trades") {
		t.Errorf("expected empty message, got %q", text)
	}
}

func TestGetMyPerformance_FilterByMethodology(t *testing.T) {
	src := &fakeRollupSource{rows: []marks.RollupRow{
		{Key: "ict_a", Took: 1, Wins: 1},
		{Key: "wyckoff_b", Took: 1, Losses: 1},
	}}
	tool := NewGetMyPerformance(src)
	res, _ := tool.Call(context.Background(), json.RawMessage(`{"by":"rule","filter":{"methodology":"ict"}}`))
	text := res.Content[0].Text
	if !strings.Contains(text, "ict_a") {
		t.Errorf("ict_a should be present: %q", text)
	}
	if strings.Contains(text, "wyckoff_b") {
		t.Errorf("wyckoff_b should be filtered out: %q", text)
	}
}

func TestGetMyPerformance_InvalidBy(t *testing.T) {
	src := &fakeRollupSource{}
	tool := NewGetMyPerformance(src)
	_, err := tool.Call(context.Background(), json.RawMessage(`{"by":"explode"}`))
	if err == nil {
		t.Fatal("expected error for invalid by")
	}
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != ErrCodeInvalidParams {
		t.Errorf("want InvalidParams, got %v", err)
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/mcp/ -run TestGetMyPerformance -count=1
```

Expected: FAIL with `undefined: NewGetMyPerformance`.

- [ ] **Step 3: Implement the tool**

Create `internal/mcp/get_my_performance.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/marks"
)

// RollupSource is the minimal interface get_my_performance needs.
// *marks.Aggregator satisfies it.
type RollupSource interface {
	Rollup(by marks.GroupBy, since time.Time) ([]marks.RollupRow, error)
}

type getMyPerformanceParams struct {
	By        string `json:"by"`
	SinceDays int    `json:"since_days"`
	Filter    *struct {
		Rule        string `json:"rule"`
		Symbol      string `json:"symbol"`
		Methodology string `json:"methodology"`
	} `json:"filter"`
}

type GetMyPerformanceTool struct {
	src RollupSource
}

func NewGetMyPerformance(src RollupSource) *GetMyPerformanceTool {
	return &GetMyPerformanceTool{src: src}
}

func (t *GetMyPerformanceTool) Name() string { return "get_my_performance" }

func (t *GetMyPerformanceTool) Description() string {
	return "Personal trading performance from user-marked alerts. Aggregates Took/Skipped/Win/Loss/BE counts grouped by rule, symbol, methodology, or timeframe over a recent window."
}

func (t *GetMyPerformanceTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"by": { "type": "string", "enum": ["rule","symbol","methodology","timeframe"], "default": "rule" },
			"since_days": { "type": "integer", "minimum": 1, "maximum": 730, "default": 30 },
			"filter": {
				"type": "object",
				"properties": {
					"rule": { "type": "string" },
					"symbol": { "type": "string" },
					"methodology": { "type": "string", "enum": ["ict","wyckoff","smc","general_ta","candlestick"] }
				}
			}
		}
	}`)
}

func (t *GetMyPerformanceTool) Call(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var p getMyPerformanceParams
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &p); err != nil {
			return ToolResult{}, NewInvalidParams("invalid params: "+err.Error(), "")
		}
	}
	if p.By == "" {
		p.By = "rule"
	}
	switch p.By {
	case "rule", "symbol", "methodology", "timeframe":
	default:
		return ToolResult{}, NewInvalidParams(fmt.Sprintf("invalid 'by' value: %q", p.By), "")
	}
	if p.SinceDays <= 0 {
		p.SinceDays = 30
	}
	if p.SinceDays > 730 {
		p.SinceDays = 730
	}

	since := time.Now().Add(-time.Duration(p.SinceDays) * 24 * time.Hour)
	rows, err := t.src.Rollup(marks.GroupBy(p.By), since)
	if err != nil {
		return ToolResult{}, &Error{Code: ErrCodeInternalError, Message: err.Error()}
	}

	// Apply filter (post-rollup; small dataset, simple).
	if p.Filter != nil {
		rows = filterRollup(rows, p.By, p.Filter.Rule, p.Filter.Symbol, p.Filter.Methodology)
	}

	header := fmt.Sprintf("**Personal Performance · last %d days · by %s**\n\n", p.SinceDays, p.By)
	if len(rows) == 0 {
		return TextResult(header + "**No marked trades in window.** _(Mark some alerts via Telegram or the My Trades tab first.)_"), nil
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{
			r.Key,
			fmt.Sprintf("%d", r.Took),
			fmt.Sprintf("%d", r.Wins),
			fmt.Sprintf("%d", r.Losses),
			fmt.Sprintf("%d", r.BreakEvens),
			fmt.Sprintf("%.1f%%", r.HitRate*100),
			fmt.Sprintf("%.1f%%", r.SkipRate*100),
		})
	}
	headers := []string{titleByDimension(p.By), "Took", "Win", "Loss", "BE", "Hit Rate", "Skip Rate"}
	table := MarkdownTable(headers, tableRows)
	footer := "\n_Hit Rate = Wins / (Wins + Losses + BE).  Skip Rate = Skipped / (Took + Skipped)._"
	return TextResult(header + table + footer), nil
}

func filterRollup(rows []marks.RollupRow, by, ruleFilter, symbolFilter, methodFilter string) []marks.RollupRow {
	out := make([]marks.RollupRow, 0, len(rows))
	for _, r := range rows {
		if methodFilter != "" {
			if by == "rule" {
				if appconfig.RuleMethodology(r.Key) != methodFilter {
					continue
				}
			} else if by == "methodology" {
				if r.Key != methodFilter {
					continue
				}
			}
		}
		if ruleFilter != "" && by == "rule" && r.Key != ruleFilter {
			continue
		}
		if symbolFilter != "" && by == "symbol" && !strings.EqualFold(r.Key, symbolFilter) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func titleByDimension(by string) string {
	switch by {
	case "rule":
		return "Rule"
	case "symbol":
		return "Symbol"
	case "methodology":
		return "Methodology"
	case "timeframe":
		return "Timeframe"
	}
	return by
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/mcp/ -run TestGetMyPerformance -count=1 -race -v
```

Expected: All 4 tests PASS.

- [ ] **Step 5: Verify full suite**

```bash
go test ./... -count=1 -race 2>&1 | tail -6
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/get_my_performance.go internal/mcp/get_my_performance_test.go
git commit -m "feat(mcp): get_my_performance tool

Returns a markdown table of personal stats grouped by
rule/symbol/methodology/timeframe over the requested day window.
Filter sub-object lets the LLM scope to a specific rule, symbol, or
methodology. Reuses MarkdownTable helper from v2.7."
```

---

## Phase F — Frontend

### Task F1: i18n keys (en/ko/ja)

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/ko.json`
- Modify: `web/src/i18n/locales/ja.json`

- [ ] **Step 1: Add `my_trades.*` and `performance.my_*` keys to en.json**

Insert before the closing brace of `web/src/i18n/locales/en.json`:

```json
  "my_trades.title": "My Trades",
  "my_trades.summary.took": "Took",
  "my_trades.summary.win_rate": "Win Rate",
  "my_trades.summary.skipped": "Skipped",
  "my_trades.summary.top_rule": "Top Rule",
  "my_trades.subtab.rollup": "Rollup",
  "my_trades.subtab.pending": "Pending",
  "my_trades.subtab.history": "History",
  "my_trades.groupby.rule": "Rule",
  "my_trades.groupby.symbol": "Symbol",
  "my_trades.groupby.methodology": "Methodology",
  "my_trades.groupby.timeframe": "Timeframe",
  "my_trades.period.7d": "7 days",
  "my_trades.period.30d": "30 days",
  "my_trades.period.90d": "90 days",
  "my_trades.period.365d": "1 year",
  "my_trades.period.all": "All time",
  "my_trades.action.took": "✅ Took",
  "my_trades.action.skipped": "❌ Skipped",
  "my_trades.action.win": "💰 Win",
  "my_trades.action.loss": "💸 Loss",
  "my_trades.action.be": "⚖ BE",
  "my_trades.action.undo": "↺ Undo",
  "my_trades.action.edit": "↺ Edit",
  "my_trades.empty.pending": "No pending signals — all caught up 🎉",
  "my_trades.empty.history": "No marked trades yet.",
  "my_trades.column.hit_rate": "Hit Rate",
  "my_trades.column.skip_rate": "Skip Rate",
  "my_trades.save_failed": "Save failed",
  "performance.my_took": "My Took",
  "performance.my_hit_rate": "My Hit Rate",
  "performance.my_skip_rate": "My Skip Rate",
  "performance.delta_vs_bt": "Δ vs BT"
```

- [ ] **Step 2: Korean (ko.json)**

```json
  "my_trades.title": "내 거래 통계",
  "my_trades.summary.took": "들어감",
  "my_trades.summary.win_rate": "승률",
  "my_trades.summary.skipped": "스킵",
  "my_trades.summary.top_rule": "최다 룰",
  "my_trades.subtab.rollup": "통계",
  "my_trades.subtab.pending": "대기 중",
  "my_trades.subtab.history": "이력",
  "my_trades.groupby.rule": "룰",
  "my_trades.groupby.symbol": "심볼",
  "my_trades.groupby.methodology": "메소드",
  "my_trades.groupby.timeframe": "타임프레임",
  "my_trades.period.7d": "7일",
  "my_trades.period.30d": "30일",
  "my_trades.period.90d": "90일",
  "my_trades.period.365d": "1년",
  "my_trades.period.all": "전체",
  "my_trades.action.took": "✅ 들어감",
  "my_trades.action.skipped": "❌ 스킵",
  "my_trades.action.win": "💰 익절",
  "my_trades.action.loss": "💸 손절",
  "my_trades.action.be": "⚖ BE",
  "my_trades.action.undo": "↺ 되돌리기",
  "my_trades.action.edit": "↺ 수정",
  "my_trades.empty.pending": "대기 중인 시그널 없음 🎉",
  "my_trades.empty.history": "아직 마킹된 거래가 없습니다.",
  "my_trades.column.hit_rate": "적중률",
  "my_trades.column.skip_rate": "스킵률",
  "my_trades.save_failed": "저장 실패",
  "performance.my_took": "내 거래 수",
  "performance.my_hit_rate": "내 적중률",
  "performance.my_skip_rate": "내 스킵률",
  "performance.delta_vs_bt": "BT 대비 Δ"
```

- [ ] **Step 3: Japanese (ja.json)**

```json
  "my_trades.title": "マイトレード",
  "my_trades.summary.took": "エントリー",
  "my_trades.summary.win_rate": "勝率",
  "my_trades.summary.skipped": "スキップ",
  "my_trades.summary.top_rule": "最多ルール",
  "my_trades.subtab.rollup": "集計",
  "my_trades.subtab.pending": "保留中",
  "my_trades.subtab.history": "履歴",
  "my_trades.groupby.rule": "ルール",
  "my_trades.groupby.symbol": "シンボル",
  "my_trades.groupby.methodology": "メソッド",
  "my_trades.groupby.timeframe": "タイムフレーム",
  "my_trades.period.7d": "7日",
  "my_trades.period.30d": "30日",
  "my_trades.period.90d": "90日",
  "my_trades.period.365d": "1年",
  "my_trades.period.all": "全期間",
  "my_trades.action.took": "✅ エントリー",
  "my_trades.action.skipped": "❌ スキップ",
  "my_trades.action.win": "💰 勝ち",
  "my_trades.action.loss": "💸 負け",
  "my_trades.action.be": "⚖ BE",
  "my_trades.action.undo": "↺ 戻す",
  "my_trades.action.edit": "↺ 編集",
  "my_trades.empty.pending": "保留中のシグナルなし 🎉",
  "my_trades.empty.history": "まだマークされた取引はありません。",
  "my_trades.column.hit_rate": "勝率",
  "my_trades.column.skip_rate": "スキップ率",
  "my_trades.save_failed": "保存失敗",
  "performance.my_took": "私のエントリー",
  "performance.my_hit_rate": "私の勝率",
  "performance.my_skip_rate": "私のスキップ率",
  "performance.delta_vs_bt": "BT比 Δ"
```

- [ ] **Step 4: Verify JSON validity + key parity**

```bash
node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/en.json','utf8')); console.log('en OK')"
node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/ko.json','utf8')); console.log('ko OK')"
node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/ja.json','utf8')); console.log('ja OK')"
node -e "
const en = Object.keys(require('./web/src/i18n/locales/en.json')).filter(k => k.startsWith('my_trades.') || k.startsWith('performance.my_')).sort();
const ko = Object.keys(require('./web/src/i18n/locales/ko.json')).filter(k => k.startsWith('my_trades.') || k.startsWith('performance.my_')).sort();
const ja = Object.keys(require('./web/src/i18n/locales/ja.json')).filter(k => k.startsWith('my_trades.') || k.startsWith('performance.my_')).sort();
const ok = JSON.stringify(en)===JSON.stringify(ko) && JSON.stringify(ko)===JSON.stringify(ja);
console.log({ ok, count: en.length });
"
```

Expected: 3 OK + `{ok:true, count:33}`.

- [ ] **Step 5: Commit**

```bash
git add web/src/i18n/locales/en.json web/src/i18n/locales/ko.json web/src/i18n/locales/ja.json
git commit -m "i18n(my_trades): add 33 keys for signal performance tracking (en/ko/ja)"
```

---

### Task F2: MarkActions component (TDD)

**Files:**
- Create: `web/src/MarkActions.tsx`
- Create: `web/src/MarkActions.test.tsx`

- [ ] **Step 1: Write failing test**

Create `web/src/MarkActions.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import i18n from './i18n'
import { MarkActions } from './MarkActions'

const wrap = (ui: React.ReactElement) => (
  <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
)

beforeEach(() => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({ signal_id: 1, status: 'TOOK', updated_at: 0 }),
  } as Response)
})
afterEach(() => { vi.restoreAllMocks() })

describe('MarkActions', () => {
  it('PENDING shows Took + Skipped', () => {
    render(wrap(<MarkActions signalId={1} status="PENDING" />))
    expect(screen.getByRole('button', { name: /Took|들어감|エントリー/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /Skipped|스킵|スキップ/ })).toBeDefined()
  })

  it('TOOK shows Win/Loss/BE/Undo', () => {
    render(wrap(<MarkActions signalId={1} status="TOOK" />))
    expect(screen.getByRole('button', { name: /Win|익절|勝ち/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /Loss|손절|負け/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /BE/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /Undo|되돌리기|戻す/ })).toBeDefined()
  })

  it('clicking Took fires POST and calls onMarked', async () => {
    const onMarked = vi.fn()
    render(wrap(<MarkActions signalId={42} status="PENDING" onMarked={onMarked} />))
    const btn = screen.getByRole('button', { name: /Took|들어감|エントリー/ })
    fireEvent.click(btn)
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    const lastCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(lastCall[0]).toBe('/api/marks/42')
    expect((lastCall[1] as RequestInit).method).toBe('POST')
    const body = JSON.parse((lastCall[1] as RequestInit).body as string)
    expect(body.action).toBe('took')
    await waitFor(() => expect(onMarked).toHaveBeenCalledWith('TOOK'))
  })

  it('rolls back on POST failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false, status: 500, json: async () => ({ error: 'boom' }),
    } as Response)
    const onMarked = vi.fn()
    render(wrap(<MarkActions signalId={1} status="PENDING" onMarked={onMarked} />))
    fireEvent.click(screen.getByRole('button', { name: /Took|들어감|エントリー/ }))
    await waitFor(() => expect(screen.queryByText(/Save failed|저장 실패|保存失敗/)).toBeDefined())
    expect(onMarked).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run failing test**

```bash
cd web && bun run test --run MarkActions
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement component**

Create `web/src/MarkActions.tsx`:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

export type MarkStatus = 'PENDING' | 'TOOK' | 'SKIPPED' | 'WIN' | 'LOSS' | 'BE'

interface Props {
  signalId: number
  status: MarkStatus
  onMarked?: (newStatus: string) => void
  apiToken?: string
}

const ACTIONS_BY_STATUS: Record<MarkStatus, { action: string; labelKey: string }[]> = {
  PENDING:  [{ action: 'took', labelKey: 'my_trades.action.took' }, { action: 'skip', labelKey: 'my_trades.action.skipped' }],
  TOOK:     [
    { action: 'win',  labelKey: 'my_trades.action.win' },
    { action: 'loss', labelKey: 'my_trades.action.loss' },
    { action: 'be',   labelKey: 'my_trades.action.be' },
    { action: 'undo', labelKey: 'my_trades.action.undo' },
  ],
  SKIPPED:  [{ action: 'undo', labelKey: 'my_trades.action.undo' }],
  WIN:      [{ action: 'undo', labelKey: 'my_trades.action.edit' }],
  LOSS:     [{ action: 'undo', labelKey: 'my_trades.action.edit' }],
  BE:       [{ action: 'undo', labelKey: 'my_trades.action.edit' }],
}

export function MarkActions({ signalId, status, onMarked, apiToken }: Props) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (action: string) => {
    setBusy(true); setError(null)
    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (apiToken) headers['Authorization'] = `Bearer ${apiToken}`
      const res = await fetch(`/api/marks/${signalId}`, {
        method: 'POST',
        headers,
        body: JSON.stringify({ action }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({} as { error?: string }))
        throw new Error(data.error ?? `HTTP ${res.status}`)
      }
      const data = await res.json() as { status: string }
      onMarked?.(data.status)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'unknown')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ display: 'inline-flex', gap: 4 }}>
      {ACTIONS_BY_STATUS[status].map(({ action, labelKey }) => (
        <button
          key={action}
          disabled={busy}
          onClick={() => submit(action)}
          style={{
            padding: '4px 10px',
            border: '1px solid var(--mint)',
            background: 'transparent',
            color: 'var(--text)',
            borderRadius: 4,
            cursor: busy ? 'wait' : 'pointer',
            fontSize: '0.78rem',
          }}
        >{t(labelKey)}</button>
      ))}
      {error && (
        <span style={{ color: 'var(--danger)', fontSize: '0.72rem', marginLeft: 6 }}>
          {t('my_trades.save_failed')}: {error}
        </span>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && bun run test --run MarkActions
```

Expected: 4/4 PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/MarkActions.tsx web/src/MarkActions.test.tsx
git commit -m "feat(ui): MarkActions shared button row

Renders the appropriate button set for each FSM status. Calls
POST /api/marks/{signal_id} and surfaces errors inline.
Reused by MyTradesTab Pending/History rows and PerformanceTab."
```

---

### Task F3: MyTradesTab component (TDD)

**Files:**
- Create: `web/src/MyTradesTab.tsx`
- Create: `web/src/MyTradesTab.test.tsx`

- [ ] **Step 1: Write failing test**

Create `web/src/MyTradesTab.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import i18n from './i18n'
import { MyTradesTab } from './MyTradesTab'

const ROLLUP_RESPONSE = {
  by: 'rule',
  since: '2026-04-09T00:00:00Z',
  rows: [
    { key: 'ict_liquidity_sweep', took: 12, skipped: 6, wins: 8, losses: 3, bes: 1, hit_rate: 0.667, skip_rate: 0.333 },
    { key: 'wyckoff_spring',      took: 8,  skipped: 5, wins: 4, losses: 3, bes: 1, hit_rate: 0.5,   skip_rate: 0.385 },
  ],
}

const wrap = (ui: React.ReactElement) => (
  <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
)

beforeEach(() => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (url) => {
    const u = String(url)
    if (u.includes('/api/marks/rollup')) {
      return { ok: true, status: 200, json: async () => ROLLUP_RESPONSE } as Response
    }
    if (u.includes('/api/marks/pending')) {
      return { ok: true, status: 200, json: async () => [] } as Response
    }
    if (u.includes('/api/marks/recent')) {
      return { ok: true, status: 200, json: async () => [] } as Response
    }
    return { ok: false, status: 404, json: async () => ({}) } as Response
  })
})
afterEach(() => { vi.restoreAllMocks() })

describe('MyTradesTab', () => {
  it('renders Rollup table from fetch', async () => {
    render(wrap(<MyTradesTab />))
    await waitFor(() => expect(screen.queryByText('ict_liquidity_sweep')).toBeDefined())
    expect(screen.getByText(/66.7%/)).toBeDefined()
  })

  it('changing GroupBy refetches', async () => {
    render(wrap(<MyTradesTab />))
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    const initial = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.length
    const select = screen.getByLabelText(/groupBy|GroupBy|Rule|룰|ルール/) as HTMLSelectElement
    fireEvent.change(select, { target: { value: 'symbol' } })
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls
      const last = calls[calls.length - 1][0] as string
      expect(last).toContain('by=symbol')
    })
    expect((globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(initial)
  })

  it('switches to Pending subtab', async () => {
    render(wrap(<MyTradesTab />))
    const pendingTab = await screen.findByRole('button', { name: /Pending|대기 중|保留中/ })
    fireEvent.click(pendingTab)
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls
      const urls = calls.map(c => String(c[0]))
      expect(urls.some(u => u.includes('/api/marks/pending'))).toBe(true)
    })
  })

  it('shows empty pending message', async () => {
    render(wrap(<MyTradesTab />))
    fireEvent.click(await screen.findByRole('button', { name: /Pending|대기 중|保留中/ }))
    await waitFor(() => expect(screen.queryByText(/all caught up|대기 중인 시그널 없음|保留中のシグナルなし/)).toBeDefined())
  })
})
```

- [ ] **Step 2: Run failing test**

```bash
cd web && bun run test --run MyTradesTab
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement MyTradesTab**

Create `web/src/MyTradesTab.tsx`:

```tsx
import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { MarkActions, MarkStatus } from './MarkActions'

interface RollupRow {
  key: string
  took: number
  skipped: number
  wins: number
  losses: number
  bes: number
  hit_rate: number
  skip_rate: number
}

interface RollupResponse {
  by: string
  since: string
  rows: RollupRow[]
}

interface PendingRow {
  signal: { id: number; symbol: string; timeframe: string; rule: string; direction: string; score: number; created_at: string }
  mark: null
}

interface MarkedRow {
  signal: PendingRow['signal']
  mark: { status: MarkStatus; took_at: number | null; outcome_at: number | null; updated_at: number }
}

type Subtab = 'rollup' | 'pending' | 'history'
type GroupBy = 'rule' | 'symbol' | 'methodology' | 'timeframe'

const PERIODS: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90, '365d': 365, all: 36500 }

export function MyTradesTab() {
  const { t } = useTranslation()
  const [subtab, setSubtab] = useState<Subtab>('rollup')
  const [groupBy, setGroupBy] = useState<GroupBy>('rule')
  const [period, setPeriod] = useState<keyof typeof PERIODS>('30d')
  const [rollup, setRollup] = useState<RollupResponse | null>(null)
  const [pending, setPending] = useState<PendingRow[]>([])
  const [history, setHistory] = useState<MarkedRow[]>([])

  const sinceISO = useCallback(() => {
    const days = PERIODS[period]
    const d = new Date(Date.now() - days * 24 * 3600 * 1000)
    return d.toISOString()
  }, [period])

  // Fetch rollup
  useEffect(() => {
    if (subtab !== 'rollup') return
    let cancelled = false
    fetch(`/api/marks/rollup?by=${groupBy}&since=${encodeURIComponent(sinceISO())}`)
      .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
      .then((data: RollupResponse) => { if (!cancelled) setRollup(data) })
      .catch(() => { /* leave previous */ })
    return () => { cancelled = true }
  }, [subtab, groupBy, period, sinceISO])

  // Fetch pending
  useEffect(() => {
    if (subtab !== 'pending') return
    fetch(`/api/marks/pending?since=${encodeURIComponent(sinceISO())}&limit=200`)
      .then(r => r.ok ? r.json() : [])
      .then((rows: PendingRow[]) => setPending(rows ?? []))
      .catch(() => setPending([]))
  }, [subtab, sinceISO])

  // Fetch history
  useEffect(() => {
    if (subtab !== 'history') return
    fetch(`/api/marks/recent?since=${encodeURIComponent(sinceISO())}&limit=200`)
      .then(r => r.ok ? r.json() : [])
      .then((rows: MarkedRow[]) => setHistory(rows ?? []))
      .catch(() => setHistory([]))
  }, [subtab, sinceISO])

  const refreshSubtab = () => {
    if (subtab === 'rollup') setSubtab('rollup')
    if (subtab === 'pending') setSubtab('pending')
    if (subtab === 'history') setSubtab('history')
  }

  return (
    <div style={{ padding: 12 }}>
      <h2>{t('my_trades.title')}</h2>

      <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
        {(['rollup', 'pending', 'history'] as Subtab[]).map(s => (
          <button
            key={s}
            onClick={() => setSubtab(s)}
            style={{
              padding: '6px 12px',
              border: '1px solid var(--mint)',
              background: subtab === s ? 'var(--mint)' : 'transparent',
              color: subtab === s ? 'var(--bg)' : 'var(--text)',
              borderRadius: 4,
              cursor: 'pointer',
            }}
          >
            {t(`my_trades.subtab.${s}`)}
          </button>
        ))}

        <select
          aria-label="period"
          value={period}
          onChange={e => setPeriod(e.target.value as keyof typeof PERIODS)}
          style={{ marginLeft: 'auto' }}
        >
          {(['7d', '30d', '90d', '365d', 'all'] as const).map(p => (
            <option key={p} value={p}>{t(`my_trades.period.${p}`)}</option>
          ))}
        </select>
      </div>

      {subtab === 'rollup' && (
        <RollupView rollup={rollup} groupBy={groupBy} setGroupBy={setGroupBy} t={t} />
      )}
      {subtab === 'pending' && (
        <PendingView rows={pending} onMarked={refreshSubtab} t={t} />
      )}
      {subtab === 'history' && (
        <HistoryView rows={history} onMarked={refreshSubtab} t={t} />
      )}
    </div>
  )
}

function RollupView({ rollup, groupBy, setGroupBy, t }: {
  rollup: RollupResponse | null
  groupBy: GroupBy
  setGroupBy: (g: GroupBy) => void
  t: (k: string) => string
}) {
  return (
    <>
      <label style={{ display: 'inline-block', marginBottom: 8 }}>
        GroupBy:&nbsp;
        <select
          aria-label="groupBy"
          value={groupBy}
          onChange={e => setGroupBy(e.target.value as GroupBy)}
        >
          {(['rule', 'symbol', 'methodology', 'timeframe'] as GroupBy[]).map(g => (
            <option key={g} value={g}>{t(`my_trades.groupby.${g}`)}</option>
          ))}
        </select>
      </label>
      {rollup && rollup.rows.length === 0 && (
        <p style={{ color: 'var(--muted)' }}>{t('my_trades.empty.history')}</p>
      )}
      {rollup && rollup.rows.length > 0 && (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th align="left">{t(`my_trades.groupby.${groupBy}`)}</th>
              <th>{t('my_trades.summary.took')}</th>
              <th>Win</th>
              <th>Loss</th>
              <th>BE</th>
              <th>{t('my_trades.column.hit_rate')}</th>
              <th>{t('my_trades.column.skip_rate')}</th>
            </tr>
          </thead>
          <tbody>
            {rollup.rows.map(r => (
              <tr key={r.key} style={{ borderTop: '1px solid var(--border)' }}>
                <td>{r.key}</td>
                <td align="right">{r.took}</td>
                <td align="right">{r.wins}</td>
                <td align="right">{r.losses}</td>
                <td align="right">{r.bes}</td>
                <td align="right">{(r.hit_rate * 100).toFixed(1)}%</td>
                <td align="right">{(r.skip_rate * 100).toFixed(1)}%</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}

function PendingView({ rows, onMarked, t }: { rows: PendingRow[]; onMarked: () => void; t: (k: string) => string }) {
  if (rows.length === 0) {
    return <p style={{ color: 'var(--muted)' }}>{t('my_trades.empty.pending')}</p>
  }
  return (
    <ul style={{ listStyle: 'none', padding: 0 }}>
      {rows.map(({ signal }) => (
        <li key={signal.id} style={{ padding: 8, borderBottom: '1px solid var(--border)' }}>
          <strong>{signal.symbol}</strong> · {signal.timeframe} · {signal.rule} · score {signal.score.toFixed(1)}
          {' '}<MarkActions signalId={signal.id} status="PENDING" onMarked={onMarked} />
        </li>
      ))}
    </ul>
  )
}

function HistoryView({ rows, onMarked, t }: { rows: MarkedRow[]; onMarked: () => void; t: (k: string) => string }) {
  if (rows.length === 0) {
    return <p style={{ color: 'var(--muted)' }}>{t('my_trades.empty.history')}</p>
  }
  return (
    <ul style={{ listStyle: 'none', padding: 0 }}>
      {rows.map(({ signal, mark }) => (
        <li key={signal.id} style={{ padding: 8, borderBottom: '1px solid var(--border)' }}>
          <strong>{signal.symbol}</strong> · {signal.timeframe} · {signal.rule} · <em>{mark.status}</em>
          {' '}<MarkActions signalId={signal.id} status={mark.status} onMarked={onMarked} />
        </li>
      ))}
    </ul>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && bun run test --run MyTradesTab
```

Expected: 4/4 PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/MyTradesTab.tsx web/src/MyTradesTab.test.tsx
git commit -m "feat(ui): MyTradesTab — Rollup / Pending / History subtabs

Each subtab fetches its own slice of /api/marks/*. Optimistic-UI
marking via shared MarkActions; on success the subtab refetches
to surface the canonical server state."
```

---

### Task F4: PerformanceTab my-stats columns + App.tsx wiring

**Files:**
- Modify: `web/src/App.tsx` (PerformanceTab + Tab type + nav)

- [ ] **Step 1: Add 'my-trades' to Tab type**

In `web/src/App.tsx` find:
```tsx
type Tab = 'symbols' | 'rules' | 'status' | 'chart' | 'backtest' | 'paper' | 'report' | 'history' | 'alert' | 'performance' | 'analysis' | 'settings' | 'price-alerts' | 'calendar' | 'execution'
```

Replace with (add `'my-trades'`):
```tsx
type Tab = 'symbols' | 'rules' | 'status' | 'chart' | 'backtest' | 'paper' | 'report' | 'history' | 'alert' | 'performance' | 'analysis' | 'settings' | 'price-alerts' | 'calendar' | 'execution' | 'my-trades'
```

- [ ] **Step 2: Add MyTradesTab import**

Near the top of `web/src/App.tsx`:
```tsx
import { MyTradesTab } from './MyTradesTab'
```

- [ ] **Step 3: Add nav button**

Find the existing tab nav row (look for the pattern `tab === 'performance'`). Add a new button after `performance`:

```tsx
<button className={`tab-btn${tab === 'my-trades' ? ' active' : ''}`} onClick={() => setTab('my-trades')}>{t('my_trades.title')}</button>
```

- [ ] **Step 4: Add render branch**

Find the tab render block (e.g., `{tab === 'performance' && <PerformanceTab />}`). Add:

```tsx
{tab === 'my-trades' && <MyTradesTab />}
```

- [ ] **Step 5: Extend PerformanceTab columns**

Find `function PerformanceTab()`. After the existing fetch of `/api/performance/rules`, add a parallel fetch of `/api/marks/rollup?by=rule`:

```tsx
const [myStats, setMyStats] = useState<Record<string, { took: number; hit_rate: number; skip_rate: number }>>({})

useEffect(() => {
  fetch('/api/marks/rollup?by=rule')
    .then(r => r.ok ? r.json() : { rows: [] })
    .then((data: { rows: { key: string; took: number; hit_rate: number; skip_rate: number }[] }) => {
      const m: Record<string, { took: number; hit_rate: number; skip_rate: number }> = {}
      for (const r of data.rows) m[r.key] = { took: r.took, hit_rate: r.hit_rate, skip_rate: r.skip_rate }
      setMyStats(m)
    })
    .catch(() => { /* leave empty */ })
}, [])
```

In the existing performance table, add 4 columns after the existing rule columns (the exact JSX depends on current structure — preserve all existing columns and add new ones in the same `<thead>` and `<tbody>`):

```tsx
// In <thead> after existing headers:
<th>{t('performance.my_took')}</th>
<th>{t('performance.my_hit_rate')}</th>
<th>{t('performance.my_skip_rate')}</th>
<th>{t('performance.delta_vs_bt')}</th>

// In <tbody> for each rule row, after existing cells:
{(() => {
  const me = myStats[rule.rule]
  if (!me || me.took === 0) {
    return (<>
      <td align="right">—</td><td align="right">—</td><td align="right">—</td><td align="right">—</td>
    </>)
  }
  const delta = (me.hit_rate - rule.win_rate) * 100  // BT win_rate is 0..1, hit_rate same
  const sign = delta > 0 ? '+' : ''
  return (<>
    <td align="right">{me.took}</td>
    <td align="right">{(me.hit_rate * 100).toFixed(1)}%</td>
    <td align="right">{(me.skip_rate * 100).toFixed(1)}%</td>
    <td align="right">{sign}{delta.toFixed(1)}pp</td>
  </>)
})()}
```

- [ ] **Step 6: Build + test**

```bash
cd web && bun run build 2>&1 | tail -8
bun run test --run 2>&1 | tail -5
```

Expected: build green, all vitest tests still pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat(ui): My Trades tab nav + Performance tab my-stats columns

PerformanceTab fetches /api/marks/rollup?by=rule and joins to
existing rule rows by rule name. Δ vs BT shows the user's hit
rate delta (in percentage points) against the backtest win rate."
```

---

## Phase G — Wiring & Release

### Task G1: Wire all dependencies in main.go

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Construct mark store + aggregator + wire**

In `cmd/server/main.go`, after the existing override store construction (Phase B3 from v2.8), add:

```go
// Signal performance tracking (Phase: signal-performance-tracking).
markStore := storage.NewSignalMarkStore(db)
aggregator := marks.NewAggregator(db, markStore)
```

After api server construction:
```go
apiSrv.WithMarkStore(markStore).WithAggregator(aggregator)
```

After notifier construction:
```go
notif.WithMarkStore(markStore)  // Notifier interface MarkStoreSet
```

After Telegram bot construction (replace the temporary `nil` from Task D1):
```go
tgBot := notifier.NewTelegramBot(token, chatID, handler, markStore)
```

After MCP registry construction (where the existing 5 tools are registered):
```go
mcpReg.Register(mcp.NewGetMyPerformance(aggregator))
log.Info().Int("tools", 6).Msg("MCP registry wired")
```

Add imports as needed: `"github.com/Ju571nK/Chatter/internal/marks"`.

- [ ] **Step 2: Build + run full Go suite**

```bash
go build ./...
go test ./... -count=1 -race 2>&1 | tail -10
```

Expected: clean + all tests pass.

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(main): wire SignalMarkStore + Aggregator into pipeline/api/notifier/mcp

Single instances constructed at startup; injected via setters into
api.Server, notifier.Notifier, notifier.TelegramBot, and the MCP
registry (NewGetMyPerformance)."
```

---

### Task G2: Release housekeeping

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`
- Modify: `docs/MCP_SETUP.md`

- [ ] **Step 1: Bump VERSION**

Read current `VERSION` (expected `2.8.1.0`). Replace with:
```
2.9.0.0
```

- [ ] **Step 2: Prepend CHANGELOG entry**

In `CHANGELOG.md`, insert IMMEDIATELY ABOVE the existing `## [2.8.1.0]` heading:

```markdown
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
```

- [ ] **Step 3: Update MCP_SETUP.md tools table**

In `docs/MCP_SETUP.md`, find the "Available tools (v1)" section. Update to v2 with 6 tools:

Replace the existing tools table with:

```markdown
## Available tools (v2)

| Tool | Purpose |
|------|---------|
| `list_watchlist` | All symbols tracked, enabled/disabled |
| `get_analysis` | Multi-timeframe analysis for a symbol (fired rules, MTF score, key levels) |
| `get_signal_history` | Recent alerts for a symbol |
| `get_ohlcv` | Raw candles (fallback — prefer `get_analysis`) |
| `get_economic_calendar` | Economic events in a date range |
| `get_my_performance` | Personal trading performance from user-marked alerts (Took/Skipped/Win/Loss/BE counts and hit rate by rule/symbol/methodology/timeframe) |
```

- [ ] **Step 4: Run full test sweep**

```bash
go test ./... -count=1 -race 2>&1 | tail -10
cd web && bun run test --run 2>&1 | tail -5
bun run build 2>&1 | tail -5
```

All four must be green.

- [ ] **Step 5: Commit**

```bash
git add VERSION CHANGELOG.md docs/MCP_SETUP.md
git commit -m "chore(release): bump VERSION to 2.9.0.0 + changelog for signal performance tracking"
```

---

## Self-Review

### Spec coverage check

- §1 Problem & Goal — Phase A–G collectively deliver the user-marked layer + 4-way aggregation + 3 surfaces (My Trades, Performance, MCP).
- §2 Architecture — A1–A4 storage, B1 aggregator, C1 HTTP, D1–D2 Telegram, E1 MCP, F1–F4 frontend, G1–G2 wiring + release.
- §3 Data model — A2 schema, A3 FSM, A3 lazy-create note, A3 SetMessageID stub-create.
- §4 Backend — A3 store, B1 aggregator, C1 handler, plus G1 wiring.
- §5 Telegram — D1 callback handler + types, D2 inline keyboard + message_id.
- §6 HTTP API — C1 covers POST + GET pending/recent/rollup with auth, validation, FSM enforcement, and 60s cache header.
- §7 MCP — E1 covers tool with input schema, markdown output, filter post-processing, registration in G1.
- §8 Frontend — F1 i18n, F2 MarkActions, F3 MyTradesTab (3 subtabs), F4 PerformanceTab columns.
- §9 Testing — Every TDD task has a failing-test step before implementation; FSM 30-case matrix in A3.
- §10 Migration — A2 idempotent table creation; A1 SaveSignal signature change handled with explicit caller updates.
- §11 Out of scope — listed in CHANGELOG Notes; no tasks added for those.
- §12 Observability — covered by zerolog WARN/INFO calls in handler and bot code (no new metrics in v1).
- §13 Documentation — G2 covers VERSION, CHANGELOG, MCP_SETUP.md.
- §14 Success criteria — all 9 outcomes are exercisable after G2.

### Placeholder scan

No `TBD`/`TODO`/`fill in details`/`similar to Task N` strings in code blocks. The phrase "TODO" appears once in the README excerpt for v2.10 features (legitimate v1.5 markers, not plan placeholders).

### Type consistency

- `Signal.ID int64` consistent across A1, A3, A4, D1, D2, F3.
- `MarkStatus` literal union consistent in F2, F3.
- `RollupRow` shape consistent across B1, C1, E1, F3, F4.
- `MarkStore` interface (notifier package) and `SignalMarkStore.Mark` (storage package) signatures consistent.
- `MarkStoreSet` (notifier interface for SetMessageID) consistent with `SignalMarkStore.SetMessageID`.
- `RollupSource` (mcp package) consistent with `Aggregator.Rollup`.
- HTTP body shape `{action: "..."}` consistent between handler test (C1) and frontend MarkActions (F2).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-09-signal-performance-tracking.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — Dispatch a fresh subagent per task with two-stage review (spec compliance, then code quality) between tasks. Fast iteration, isolated context per task, prevents drift on a 13-step plan.

2. **Inline Execution** — Execute tasks in this session using `superpowers:executing-plans` with checkpoints between phases.

**Which approach?**
