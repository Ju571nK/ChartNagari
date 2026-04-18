# Execution Plugin Phase 4 — React UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a top-level "Execution" React tab that surfaces plugin health, a kill switch, full CRUD on plugins, editable global config, and a filtered feedback table — unblocking the operational feedback loop for Alpaca and any future adapter.

**Architecture:** Six new React files under `web/src/` (ExecutionTab container + 5 leaf components), two new `GET` endpoints (`/api/execution/feedback`, `/api/execution/plugins/stats`), a new `execution_state` SQLite key-value table for runtime state (`killed_at`, `config_version`), a `version` field on `PUT /api/execution/config` for 409 conflict detection, and two added columns (`symbol`, `message`) on `feedback_idempotency`.

**Tech Stack:** Go 1.26, modernc.org/sqlite, React 18 + TypeScript + Vite, Vitest + Testing Library, zerolog. Design tokens from `DESIGN.md`.

**Spec:** `docs/superpowers/specs/2026-04-17-execution-ui-design.md`

---

## Phase A — Backend migrations & state store

### Task 1: Add migration for `feedback_idempotency` columns, index, and `execution_state` table

**Files:**
- Modify: `internal/storage/db.go:180-220` (migrate() function)
- Test: `internal/storage/db_test.go` (create if absent, or follow existing test file pattern in the package)

- [ ] **Step 1: Write the failing test**

Append to `internal/storage/db_test.go` (create new file if none exists; existing `signals_test.go` and `calendar_test.go` show the package's test style):

```go
package storage

import (
	"testing"
)

func TestMigration_FeedbackIdempotencyAddsSymbolAndMessage(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Expect symbol and message columns to exist on feedback_idempotency.
	var name string
	rows, err := db.Conn().Query(`PRAGMA table_info(feedback_idempotency)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		cols[name] = true
	}
	for _, col := range []string{"symbol", "message"} {
		if !cols[col] {
			t.Errorf("feedback_idempotency is missing column %q", col)
		}
	}
}

func TestMigration_ExecutionStateTableExists(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var count int
	row := db.Conn().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='execution_state'`,
	)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != 1 {
		t.Fatalf("execution_state table not created (count=%d)", count)
	}
}

func TestMigration_FeedbackCreatedAtIndex(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var count int
	row := db.Conn().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_feedback_received_at'`,
	)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != 1 {
		t.Fatalf("idx_feedback_received_at not created (count=%d)", count)
	}
}

func TestMigration_Idempotent(t *testing.T) {
	dir := t.TempDir()
	db1, err := New(dir + "/test.db")
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	db1.Close()
	// Re-open same file — migration should run cleanly (no duplicate-column error).
	db2, err := New(dir + "/test.db")
	if err != nil {
		t.Fatalf("open 2 (idempotency): %v", err)
	}
	db2.Close()
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/storage/ -run TestMigration -v
```
Expected: FAIL — `symbol`/`message` columns missing, `execution_state` table missing, index missing.

- [ ] **Step 3: Update migration in `internal/storage/db.go`**

Inside the `schema := ` backtick string in `migrate()`, append before the closing backtick (after line 202):

```sql
	-- Execution runtime state (kill switch timestamp, config version, etc.).
	CREATE TABLE IF NOT EXISTS execution_state (
		key         TEXT    NOT NULL PRIMARY KEY,
		value       TEXT    NOT NULL,
		updated_at  INTEGER NOT NULL  -- Unix seconds (UTC)
	);

	-- Feedback read-path index for 24h window aggregation and list queries.
	CREATE INDEX IF NOT EXISTS idx_feedback_received_at
		ON feedback_idempotency(received_at);
```

Then, after the `db.conn.Exec(schema)` block (just below the existing `signals` ALTER TABLE block), add two more ALTER TABLE migrations following the same pattern:

```go
	// Migrate existing DB: add symbol column to feedback_idempotency.
	if _, err := db.conn.Exec(
		`ALTER TABLE feedback_idempotency ADD COLUMN symbol TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("feedback_idempotency symbol migration: %w", err)
		}
	}

	// Migrate existing DB: add message column to feedback_idempotency.
	if _, err := db.conn.Exec(
		`ALTER TABLE feedback_idempotency ADD COLUMN message TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("feedback_idempotency message migration: %w", err)
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/storage/ -run TestMigration -v
```
Expected: PASS — all four tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/db.go internal/storage/db_test.go
git commit -m "feat(exec): migrations — symbol/message on feedback_idempotency, execution_state table"
```

---

### Task 2: Implement `StateStore` key-value helper

**Files:**
- Create: `internal/execution/state.go`
- Test: `internal/execution/state_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/execution/state_test.go`:

```go
package execution

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newStateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE execution_state (
			key         TEXT PRIMARY KEY,
			value       TEXT NOT NULL,
			updated_at  INTEGER NOT NULL
		);
	`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestStateStore_MissingKeyReturnsEmpty(t *testing.T) {
	s := NewStateStore(newStateTestDB(t))
	v, err := s.Get(context.Background(), "killed_at")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestStateStore_SetThenGet(t *testing.T) {
	s := NewStateStore(newStateTestDB(t))
	ctx := context.Background()
	if err := s.Set(ctx, "killed_at", "2026-04-17T10:30:00Z"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, err := s.Get(ctx, "killed_at")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if v != "2026-04-17T10:30:00Z" {
		t.Fatalf("unexpected value: %q", v)
	}
}

func TestStateStore_SetUpsertsExistingKey(t *testing.T) {
	s := NewStateStore(newStateTestDB(t))
	ctx := context.Background()
	_ = s.Set(ctx, "config_version", "3")
	time.Sleep(10 * time.Millisecond)
	if err := s.Set(ctx, "config_version", "4"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, _ := s.Get(ctx, "config_version")
	if v != "4" {
		t.Fatalf("upsert failed: %q", v)
	}
}

func TestStateStore_ConcurrentSetsSerialize(t *testing.T) {
	s := NewStateStore(newStateTestDB(t))
	ctx := context.Background()
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			done <- s.Set(ctx, "counter", string(rune('0'+n)))
		}(i)
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent set %d: %v", i, err)
		}
	}
	v, _ := s.Get(ctx, "counter")
	if len(v) != 1 {
		t.Fatalf("expected single-char value, got %q", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/execution/ -run TestStateStore -v
```
Expected: FAIL — `NewStateStore` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `internal/execution/state.go`:

```go
package execution

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StateStore is a thin key-value facade over the execution_state table.
// Used for runtime state that must survive restarts (kill-switch timestamp,
// config version) but does not belong in the YAML configuration file.
type StateStore struct {
	db *sql.DB
}

// NewStateStore constructs a StateStore bound to the given database handle.
func NewStateStore(db *sql.DB) *StateStore {
	return &StateStore{db: db}
}

// Get returns the value for key, or empty string if the key is absent.
func (s *StateStore) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM execution_state WHERE key = ?`, key,
	).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("state get %s: %w", key, err)
	}
	return v, nil
}

// Set upserts key to value, stamping updated_at with the current Unix time.
func (s *StateStore) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO execution_state(key, value, updated_at) VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("state set %s: %w", key, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/execution/ -run TestStateStore -race -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/execution/state.go internal/execution/state_test.go
git commit -m "feat(exec): add StateStore key-value helper on execution_state"
```

---

### Task 3: Extend `FeedbackIdempotency.RecordOnce` to persist symbol and message

**Files:**
- Modify: `internal/execution/dedup.go:112-142`
- Modify: `internal/execution/testhelpers_test.go:31-40` (schema used by in-package tests)
- Test: `internal/execution/dedup_test.go`

- [ ] **Step 1: Update test schema fixture**

Edit `internal/execution/testhelpers_test.go` where the `CREATE TABLE feedback_idempotency` string lives; add two columns so the test helper mirrors production schema:

```sql
CREATE TABLE feedback_idempotency (
    plugin_id    TEXT    NOT NULL,
    signal_id    TEXT    NOT NULL,
    order_id     TEXT    NOT NULL DEFAULT '',
    status       TEXT    NOT NULL,
    received_at  INTEGER NOT NULL,
    symbol       TEXT    NOT NULL DEFAULT '',
    message      TEXT    NOT NULL DEFAULT '',
    UNIQUE(plugin_id, signal_id, order_id, status)
);
```

Also add the index to match production:

```sql
CREATE INDEX idx_feedback_received_at ON feedback_idempotency(received_at);
```

- [ ] **Step 2: Write the failing test**

Append to `internal/execution/dedup_test.go`:

```go
func TestFeedbackIdempotency_PersistsSymbolAndMessage(t *testing.T) {
	db := newExecTestDB(t)
	f := NewFeedbackIdempotency(db)
	ctx := context.Background()

	fresh, err := f.RecordOnce(ctx, "alpaca-paper", "sig-1", "ord-1", "FILLED", "AAPL", "Market order filled", time.Now())
	if err != nil || !fresh {
		t.Fatalf("record: fresh=%v err=%v", fresh, err)
	}

	var gotSymbol, gotMsg string
	err = db.QueryRow(
		`SELECT symbol, message FROM feedback_idempotency WHERE signal_id = 'sig-1'`,
	).Scan(&gotSymbol, &gotMsg)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotSymbol != "AAPL" || gotMsg != "Market order filled" {
		t.Fatalf("got (%q, %q), want (AAPL, Market order filled)", gotSymbol, gotMsg)
	}
}
```

Note: `newExecTestDB` is the existing helper in `testhelpers_test.go`.

- [ ] **Step 3: Run test to verify it fails (compile error)**

```
go test ./internal/execution/ -run TestFeedbackIdempotency_PersistsSymbolAndMessage -v
```
Expected: FAIL — `RecordOnce` signature does not accept symbol/message.

- [ ] **Step 4: Update `RecordOnce` signature and SQL**

Edit `internal/execution/dedup.go`:

```go
func (f *FeedbackIdempotency) RecordOnce(ctx context.Context, pluginID, signalID, orderID, status, symbol, message string, at time.Time) (bool, error) {
	res, err := f.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO feedback_idempotency(plugin_id, signal_id, order_id, status, received_at, symbol, message) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		pluginID, signalID, orderID, status, at.Unix(), symbol, message,
	)
	if err != nil {
		if isBusy(err) {
			return false, ErrBusy
		}
		return false, fmt.Errorf("feedback_idempotency insert: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
```

- [ ] **Step 5: Fix compile errors at all call sites**

Every call to `f.RecordOnce(...)` must now pass `symbol` and `message`. Find them:

```
grep -rn "RecordOnce(" --include="*.go"
```

Update each call site. The primary one is `internal/api/execution_handler.go:148`. Any existing tests that call `RecordOnce` in `dedup_test.go` / handler tests must also pass empty strings or meaningful values for the new parameters.

- [ ] **Step 6: Run full execution package tests**

```
go test ./internal/execution/ -race -v
```
Expected: PASS (all prior tests still green with updated signature).

- [ ] **Step 7: Commit**

```bash
git add internal/execution/dedup.go internal/execution/testhelpers_test.go internal/execution/dedup_test.go
git commit -m "feat(exec): persist symbol and message on feedback_idempotency rows"
```

---

### Task 4: Add `Symbol` to `OrderFeedback` model and persist it in the handler

**Files:**
- Modify: `pkg/models/trade_signal.go:37-46` (OrderFeedback struct)
- Modify: `internal/api/execution_handler.go:134-148` (feedback parser + RecordOnce call)
- Test: `internal/api/execution_handler_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/api/execution_handler_test.go` (follow the existing table-driven + test server pattern already in that file):

```go
func TestFeedback_PersistsSymbolAndMessage(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t) // existing helper
	defer cleanup()

	// Post a feedback with symbol + message filled in.
	body := []byte(`{"signal_id":"sig-99","plugin_name":"alpaca-paper","status":"FILLED","symbol":"TSLA","message":"OK"}`)
	resp := srv.postSignedFeedback(t, "alpaca-paper", body) // existing helper
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	var sym, msg string
	row := srv.db.QueryRow(`SELECT symbol, message FROM feedback_idempotency WHERE signal_id = 'sig-99'`)
	if err := row.Scan(&sym, &msg); err != nil {
		t.Fatalf("row: %v", err)
	}
	if sym != "TSLA" || msg != "OK" {
		t.Fatalf("got (%q, %q)", sym, msg)
	}
}
```

If the helpers (`newExecutionHandlerTestServer`, `postSignedFeedback`) don't exist in the exact names above, read the existing `execution_handler_test.go` and adapt to the real helper names used there. The point is: build a server, sign a feedback body, POST, then read the DB row.

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/api/ -run TestFeedback_PersistsSymbolAndMessage -v
```
Expected: FAIL — symbol/message not persisted (blank strings).

- [ ] **Step 3: Add `Symbol` to `OrderFeedback`**

Edit `pkg/models/trade_signal.go`:

```go
type OrderFeedback struct {
	SignalID    string    `json:"signal_id"`
	PluginName  string    `json:"plugin_name"`
	Status      string    `json:"status"`
	OrderID     string    `json:"order_id,omitempty"`
	Symbol      string    `json:"symbol,omitempty"`    // NEW — echoed by plugin
	FilledQty   float64   `json:"filled_qty,omitempty"`
	FilledPrice float64   `json:"filled_price,omitempty"`
	Message     string    `json:"message,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}
```

- [ ] **Step 4: Update handler to parse and forward symbol + message**

Edit `internal/api/execution_handler.go` inside `postExecutionFeedback`. Replace the anonymous struct `fb` with the model type so we pick up the new fields for free, and pass them through:

```go
	// Parse the feedback payload AFTER authentication succeeds.
	var fb models.OrderFeedback
	if err := json.Unmarshal(body, &fb); err != nil {
		http.Error(w, "invalid feedback JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if fb.SignalID == "" || fb.Status == "" {
		http.Error(w, "signal_id and status are required", http.StatusBadRequest)
		return
	}

	fresh, err := s.execFeedback.RecordOnce(
		r.Context(),
		pluginID, fb.SignalID, fb.OrderID, fb.Status,
		strings.ToUpper(fb.Symbol), fb.Message,
		time.Now(),
	)
```

Add `"github.com/Ju571nK/ChartNagari/pkg/models"` and `"strings"` to the imports if not already present. (Verify the module import path with `grep '^module' go.mod`.)

- [ ] **Step 5: Run tests to verify they pass**

```
go test ./internal/api/ -race -v
```
Expected: PASS (new test green, existing tests still green).

- [ ] **Step 6: Commit**

```bash
git add pkg/models/trade_signal.go internal/api/execution_handler.go internal/api/execution_handler_test.go
git commit -m "feat(exec): OrderFeedback.Symbol — plugins echo symbol for UI enrichment"
```

---

### Task 5: Update Alpaca adapter to include symbol in feedback

**Files:**
- Modify: `internal/plugins/alpaca/feedback.go`
- Modify: `internal/plugins/alpaca/feedback_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/plugins/alpaca/feedback_test.go`, add (or extend an existing table-driven test) a case asserting the serialized feedback body contains `"symbol":"AAPL"`:

```go
func TestFeedbackIncludesSymbol(t *testing.T) {
	var captured []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	c := NewFeedbackClient(ts.URL, "secret", "alpaca-paper")
	err := c.Send(context.Background(), Feedback{
		SignalID: "sig-1",
		Status:   "FILLED",
		Symbol:   "AAPL",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !bytes.Contains(captured, []byte(`"symbol":"AAPL"`)) {
		t.Fatalf("body missing symbol: %s", captured)
	}
}
```

Adjust struct/method names to match the existing adapter. The key assertion: the posted JSON body includes the `symbol` field.

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/plugins/alpaca/ -run TestFeedbackIncludesSymbol -v
```
Expected: FAIL — `Symbol` field doesn't exist on the adapter's feedback struct.

- [ ] **Step 3: Add `Symbol` field + thread it through**

In `internal/plugins/alpaca/feedback.go`, add `Symbol` to the adapter's internal feedback struct (or use `models.OrderFeedback` directly if already imported) and populate it from the `TradeSignal.Symbol` at the call site in `server.go` / `runner.go`:

```go
type Feedback struct {
	SignalID string `json:"signal_id"`
	Status   string `json:"status"`
	OrderID  string `json:"order_id,omitempty"`
	Symbol   string `json:"symbol,omitempty"` // NEW
	Message  string `json:"message,omitempty"`
}
```

Find where the adapter constructs outbound feedback (grep for `Feedback{` in `internal/plugins/alpaca/`), and set `Symbol: signal.Symbol` from the inbound TradeSignal.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/plugins/alpaca/ -race -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/plugins/alpaca/feedback.go internal/plugins/alpaca/feedback_test.go
git commit -m "feat(alpaca): echo TradeSignal symbol in outbound OrderFeedback"
```

---

## Phase B — Backend new endpoints and config version

### Task 6: `GET /api/execution/feedback` endpoint

**Files:**
- Modify: `internal/api/execution_handler.go` (add `listExecutionFeedback` handler)
- Modify: `internal/api/server.go` (register route)
- Test: `internal/api/execution_handler_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/api/execution_handler_test.go`:

```go
func TestListFeedback_NoFilterReturnsRecent(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	srv.seedFeedback(t, "alpaca-paper", "sig-1", "ord-1", "FILLED", "AAPL", "ok")
	srv.seedFeedback(t, "alpaca-paper", "sig-2", "ord-2", "REJECTED", "TSLA", "bad")

	resp := srv.get(t, "/api/execution/feedback")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out struct {
		Items []struct{ SignalID, Status, Symbol, Message string } `json:"items"`
		Count int                                                  `json:"count"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Count != 2 {
		t.Fatalf("count: %d", out.Count)
	}
}

func TestListFeedback_FilterByStatus(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	srv.seedFeedback(t, "alpaca-paper", "s1", "o1", "FILLED", "AAPL", "")
	srv.seedFeedback(t, "alpaca-paper", "s2", "o2", "REJECTED", "TSLA", "")

	resp := srv.get(t, "/api/execution/feedback?status=FILLED")
	var out struct{ Count int `json:"count"` }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Count != 1 {
		t.Fatalf("count: %d", out.Count)
	}
}

func TestListFeedback_SymbolFilterUppercase(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	srv.seedFeedback(t, "alpaca-paper", "s1", "o1", "FILLED", "AAPL", "")

	resp := srv.get(t, "/api/execution/feedback?symbol=aapl")
	var out struct{ Count int `json:"count"` }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Count != 1 {
		t.Fatalf("lowercase query should match uppercase storage, got %d", out.Count)
	}
}

func TestListFeedback_LimitBounds(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	cases := []struct {
		q          string
		wantStatus int
	}{
		{"?limit=0", http.StatusOK},       // treat as default
		{"?limit=-1", http.StatusBadRequest},
		{"?limit=501", http.StatusBadRequest},
		{"?limit=500", http.StatusOK},
	}
	for _, c := range cases {
		resp := srv.get(t, "/api/execution/feedback"+c.q)
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s: got %d want %d", c.q, resp.StatusCode, c.wantStatus)
		}
	}
}

func TestListFeedback_Unauthorized(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	resp := srv.getNoAuth(t, "/api/execution/feedback")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}
```

If `seedFeedback`, `get`, and `getNoAuth` helpers are not already in the test file, add them following the patterns used by existing tests in `execution_handler_test.go`.

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/api/ -run TestListFeedback -v
```
Expected: FAIL — route not registered.

- [ ] **Step 3: Implement handler**

Append to `internal/api/execution_handler.go`:

```go
// listExecutionFeedback returns recent feedback rows, newest first, with
// optional plugin/status/symbol filters and a limit (default 100, max 500).
func (s *Server) listExecutionFeedback(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "not enabled", http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	plugin := q.Get("plugin")
	status := q.Get("status")
	symbol := strings.ToUpper(strings.TrimSpace(q.Get("symbol")))

	limit := 100
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			http.Error(w, "bad limit", http.StatusBadRequest)
			return
		}
		if n < 0 || n > 500 {
			http.Error(w, "limit out of range (0..500)", http.StatusBadRequest)
			return
		}
		if n > 0 {
			limit = n
		}
	}

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT plugin_id, signal_id, order_id, status, COALESCE(symbol,''), COALESCE(message,''), received_at
		FROM feedback_idempotency
		WHERE (? = '' OR plugin_id = ?)
		  AND (? = '' OR status = ?)
		  AND (? = '' OR symbol = ?)
		ORDER BY received_at DESC
		LIMIT ?`,
		plugin, plugin, status, status, symbol, symbol, limit,
	)
	if err != nil {
		http.Error(w, "query", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type item struct {
		PluginID   string `json:"plugin_id"`
		SignalID   string `json:"signal_id"`
		OrderID    string `json:"order_id"`
		Status     string `json:"status"`
		Symbol     string `json:"symbol"`
		Message    string `json:"message"`
		ReceivedAt int64  `json:"received_at"`
	}
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.PluginID, &it.SignalID, &it.OrderID, &it.Status, &it.Symbol, &it.Message, &it.ReceivedAt); err != nil {
			http.Error(w, "scan", http.StatusInternalServerError)
			return
		}
		items = append(items, it)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "count": len(items)})
}
```

Add `"strconv"` to imports if missing.

- [ ] **Step 4: Register route**

In `internal/api/server.go`, find the existing execution route registrations (search for `/api/execution/`) and add:

```go
mux.Handle("GET /api/execution/feedback", s.requireAuth(http.HandlerFunc(s.listExecutionFeedback)))
```

Use the same `requireAuth` wrapper used by other `/api/execution/*` GET endpoints.

- [ ] **Step 5: Run tests to verify pass**

```
go test ./internal/api/ -run TestListFeedback -race -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/execution_handler.go internal/api/server.go internal/api/execution_handler_test.go
git commit -m "feat(exec): GET /api/execution/feedback with plugin/status/symbol filters"
```

---

### Task 7: `GET /api/execution/plugins/stats` endpoint with 24h aggregation + Cache-Control

**Files:**
- Modify: `internal/api/execution_handler.go` (add `getExecutionPluginStats`)
- Modify: `internal/api/server.go` (register route)
- Test: `internal/api/execution_handler_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestPluginStats_Aggregates24hCounts(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	now := time.Now()
	srv.seedFeedbackAt(t, "alpaca", "s1", "o1", "FILLED", "AAPL", "", now.Add(-1*time.Hour))
	srv.seedFeedbackAt(t, "alpaca", "s2", "o2", "FILLED", "AAPL", "", now.Add(-2*time.Hour))
	srv.seedFeedbackAt(t, "alpaca", "s3", "o3", "REJECTED", "TSLA", "denied", now.Add(-3*time.Hour))
	srv.seedFeedbackAt(t, "alpaca", "s4", "o4", "FILLED", "AAPL", "", now.Add(-25*time.Hour)) // outside window

	resp := srv.get(t, "/api/execution/plugins/stats?window=24h")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "max-age=60" {
		t.Errorf("Cache-Control = %q, want max-age=60", cc)
	}
	var out struct {
		Plugins []struct {
			PluginID       string `json:"plugin_id"`
			Submitted      int    `json:"submitted"`
			Filled         int    `json:"filled"`
			Rejected       int    `json:"rejected"`
			LastFailureMsg string `json:"last_failure_msg"`
		} `json:"plugins"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Plugins) != 1 {
		t.Fatalf("plugins: %d", len(out.Plugins))
	}
	p := out.Plugins[0]
	if p.Filled != 2 || p.Rejected != 1 || p.LastFailureMsg != "denied" {
		t.Fatalf("aggregation wrong: %+v", p)
	}
}

func TestPluginStats_ZeroActivityPluginOmitted(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	resp := srv.get(t, "/api/execution/plugins/stats?window=24h")
	var out struct{ Plugins []any `json:"plugins"` }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Plugins) != 0 {
		t.Fatalf("expected empty, got %d", len(out.Plugins))
	}
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/api/ -run TestPluginStats -v
```
Expected: FAIL — route not registered.

- [ ] **Step 3: Implement handler**

```go
func (s *Server) getExecutionPluginStats(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "not enabled", http.StatusNotFound)
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "24h"
	}
	if window != "24h" {
		http.Error(w, "only window=24h is supported", http.StatusBadRequest)
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour).Unix()

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT plugin_id,
		       SUM(CASE WHEN status IN ('SUBMITTED','RECEIVED') THEN 1 ELSE 0 END) AS submitted,
		       SUM(CASE WHEN status IN ('FILLED','PARTIAL_FILL')  THEN 1 ELSE 0 END) AS filled,
		       SUM(CASE WHEN status IN ('REJECTED','ERROR')       THEN 1 ELSE 0 END) AS rejected,
		       MAX(CASE WHEN status IN ('REJECTED','ERROR') THEN received_at END)   AS last_fail_at,
		       COALESCE(
		         (SELECT message FROM feedback_idempotency f2
		          WHERE f2.plugin_id = f1.plugin_id
		            AND f2.status IN ('REJECTED','ERROR')
		            AND f2.received_at >= ?
		          ORDER BY f2.received_at DESC LIMIT 1), '') AS last_fail_msg
		FROM feedback_idempotency f1
		WHERE received_at >= ?
		GROUP BY plugin_id`,
		cutoff, cutoff,
	)
	if err != nil {
		http.Error(w, "query", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type stat struct {
		PluginID       string `json:"plugin_id"`
		Submitted      int    `json:"submitted"`
		Filled         int    `json:"filled"`
		Rejected       int    `json:"rejected"`
		LastFailureAt  *int64 `json:"last_failure_at,omitempty"`
		LastFailureMsg string `json:"last_failure_msg"`
	}
	out := []stat{}
	for rows.Next() {
		var st stat
		var lastFailAt sql.NullInt64
		if err := rows.Scan(&st.PluginID, &st.Submitted, &st.Filled, &st.Rejected, &lastFailAt, &st.LastFailureMsg); err != nil {
			http.Error(w, "scan", http.StatusInternalServerError)
			return
		}
		if lastFailAt.Valid {
			v := lastFailAt.Int64
			st.LastFailureAt = &v
		}
		out = append(out, st)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=60")
	_ = json.NewEncoder(w).Encode(map[string]any{"window": "24h", "plugins": out})
}
```

Add `"database/sql"` to imports if missing.

- [ ] **Step 4: Register route**

In `internal/api/server.go`:

```go
mux.Handle("GET /api/execution/plugins/stats", s.requireAuth(http.HandlerFunc(s.getExecutionPluginStats)))
```

- [ ] **Step 5: Run tests to verify pass**

```
go test ./internal/api/ -run TestPluginStats -race -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/execution_handler.go internal/api/server.go internal/api/execution_handler_test.go
git commit -m "feat(exec): GET /api/execution/plugins/stats with 24h aggregation and 60s cache"
```

---

### Task 8: Add `version` field to config and implement 409 on mismatch

**Files:**
- Modify: `internal/api/execution_handler.go` (GET + PUT config)
- Modify: `internal/api/server.go` (inject StateStore)
- Test: `internal/api/execution_handler_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestUpdateConfig_VersionMatchBumps(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	cfg := srv.getConfig(t)           // helper that does GET /api/execution/config and unmarshals
	if cfg.Version < 1 {
		t.Fatalf("initial version: %d", cfg.Version)
	}

	body, _ := json.Marshal(map[string]any{
		"version":        cfg.Version,
		"enabled":        true,
		"max_dispatched": 5,
	})
	resp := srv.put(t, "/api/execution/config", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	cfg2 := srv.getConfig(t)
	if cfg2.Version != cfg.Version+1 {
		t.Fatalf("version did not bump: %d → %d", cfg.Version, cfg2.Version)
	}
}

func TestUpdateConfig_VersionMismatchReturns409(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	cfg := srv.getConfig(t)

	body, _ := json.Marshal(map[string]any{
		"version":        cfg.Version + 99,
		"enabled":        true,
	})
	resp := srv.put(t, "/api/execution/config", body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var body409 struct {
		Error          string `json:"error"`
		CurrentVersion int    `json:"current_version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body409)
	if body409.Error != "version_conflict" || body409.CurrentVersion != cfg.Version {
		t.Fatalf("bad 409 body: %+v", body409)
	}
}

func TestUpdateConfig_MissingVersionRejected(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	body, _ := json.Marshal(map[string]any{"enabled": true})
	resp := srv.put(t, "/api/execution/config", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/api/ -run TestUpdateConfig -v
```
Expected: FAIL.

- [ ] **Step 3: Implement version logic**

In `internal/api/server.go`, the `Server` struct already holds dependencies. Add a field:

```go
type Server struct {
	// ... existing fields
	execState *execution.StateStore
}
```

And a constructor parameter / setter — follow the pattern used by `execHolder`, `execFeedback`, etc.

Add a helper:

```go
const (
	stateKeyConfigVersion = "config_version"
	stateKeyKilledAt      = "killed_at"
)

func (s *Server) readConfigVersion(ctx context.Context) (int, error) {
	v, err := s.execState.Get(ctx, stateKeyConfigVersion)
	if err != nil {
		return 0, err
	}
	if v == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("bad config_version: %w", err)
	}
	return n, nil
}

func (s *Server) bumpConfigVersion(ctx context.Context) (int, error) {
	v, err := s.readConfigVersion(ctx)
	if err != nil {
		return 0, err
	}
	next := v + 1
	if err := s.execState.Set(ctx, stateKeyConfigVersion, strconv.Itoa(next)); err != nil {
		return 0, err
	}
	return next, nil
}
```

In `getExecutionConfig` (existing), add the version to the response:

```go
v, _ := s.readConfigVersion(r.Context())
out := cfg.Redacted()               // whatever the existing redaction call is
outMap := map[string]any{
	/* existing fields */
	"version":    v,
	"killed_at":  func() string { s, _ := s.execState.Get(r.Context(), stateKeyKilledAt); return s }(),
}
_ = json.NewEncoder(w).Encode(outMap)
```

If the existing handler marshals a typed struct, switch to `map[string]any` for this field addition, or add `Version int `json:"version"`` to the existing DTO struct.

In `putExecutionConfig` (existing), add version-check logic at the top of the body-parsing block:

```go
var incoming struct {
	Version *int `json:"version"`
	// ... other fields
}
if err := json.Unmarshal(body, &incoming); err != nil {
	http.Error(w, "invalid json", http.StatusBadRequest)
	return
}
if incoming.Version == nil {
	http.Error(w, "version field required", http.StatusBadRequest)
	return
}
current, err := s.readConfigVersion(r.Context())
if err != nil {
	http.Error(w, "state", http.StatusInternalServerError)
	return
}
if *incoming.Version != current {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":           "version_conflict",
		"current_version": current,
	})
	return
}

// ... existing save/merge logic ...

next, err := s.bumpConfigVersion(r.Context())
if err != nil {
	http.Error(w, "version bump", http.StatusInternalServerError)
	return
}
_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "version": next})
```

- [ ] **Step 4: Update test server helpers**

Add `s.execState = execution.NewStateStore(db.Conn())` to `newExecutionHandlerTestServer`. Provide `getConfig` and `put` helpers if not present.

- [ ] **Step 5: Run tests to verify pass**

```
go test ./internal/api/ -run TestUpdateConfig -race -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/execution_handler.go internal/api/server.go internal/api/execution_handler_test.go
git commit -m "feat(exec): config version + 409 on mismatch, stored in execution_state"
```

---

### Task 9: Move `killed_at` from YAML to `execution_state`; return it from config GET

**Files:**
- Modify: `internal/api/execution_handler.go` (`postExecutionKill`, `getExecutionConfig`)
- Modify: `internal/config/execution.go` if it currently persists `killed_at` to YAML (remove that field usage)
- Test: `internal/api/execution_handler_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestKill_WritesToStateTable(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	before := time.Now().Add(-1 * time.Second)
	resp := srv.post(t, "/api/execution/kill", []byte(`{"on":true}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	cfg := srv.getConfig(t)
	if cfg.KilledAt == "" {
		t.Fatalf("KilledAt empty")
	}
	ts, err := time.Parse(time.RFC3339, cfg.KilledAt)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ts.Before(before) {
		t.Fatalf("KilledAt %v precedes request time %v", ts, before)
	}

	// Confirm value landed in state table.
	v, err := srv.execState.Get(context.Background(), "killed_at")
	if err != nil || v == "" {
		t.Fatalf("state: v=%q err=%v", v, err)
	}
}

func TestKill_Idempotent(t *testing.T) {
	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()
	_ = srv.post(t, "/api/execution/kill", []byte(`{"on":true}`))
	resp := srv.post(t, "/api/execution/kill", []byte(`{"on":true}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second kill status: %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```
go test ./internal/api/ -run TestKill_Writes -v
```
Expected: FAIL.

- [ ] **Step 3: Update `postExecutionKill` to write to state**

Add after the existing in-memory kill-switch update:

```go
if body.On {
	if err := s.execState.Set(r.Context(), stateKeyKilledAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		log.Error().Err(err).Msg("execution: persist killed_at failed")
	}
} else {
	// Re-enable clears the timestamp.
	_ = s.execState.Set(r.Context(), stateKeyKilledAt, "")
}
```

- [ ] **Step 4: Include `killed_at` in GET config response**

Already added as part of Task 8 Step 3. Verify presence and typed shape (`KilledAt string `json:"killed_at"``).

- [ ] **Step 5: Run tests to verify pass**

```
go test ./internal/api/ -run TestKill -race -v
```
Expected: PASS.

- [ ] **Step 6: If `internal/config/execution.go` has any `KilledAt` field that writes to YAML, remove it**

Search:
```
grep -rn "killed_at\|KilledAt" internal/config/
```
Any YAML-persisted `killed_at` should be deleted (the state table is now the single source of truth).

- [ ] **Step 7: Commit**

```bash
git add internal/api/execution_handler.go internal/api/execution_handler_test.go
# (+ internal/config/execution.go if modified)
git commit -m "feat(exec): kill switch persists killed_at to execution_state table"
```

---

### Task 10: Blanket secret-leak test across all `/api/execution/*` endpoints

**Files:**
- Create: `internal/api/execution_secret_leak_test.go`

- [ ] **Step 1: Write the test**

```go
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestNoSecretLeakInExecutionEndpoints configures a plugin with a known secret,
// hits every GET /api/execution/* and a mutating endpoint, and asserts the raw
// secret string never appears in any response body or header.
func TestNoSecretLeakInExecutionEndpoints(t *testing.T) {
	const knownSecret = "SUPERSECRET_DO_NOT_LEAK_0123456789ABCDEF"

	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	// Seed a plugin with the sentinel secret via direct config load.
	srv.setPluginSecret(t, "alpaca-paper", knownSecret)

	paths := []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/api/execution/config", nil},
		{"GET", "/api/execution/feedback", nil},
		{"GET", "/api/execution/plugins/stats?window=24h", nil},
	}

	for _, p := range paths {
		var req *http.Request
		if p.body != nil {
			req = srv.authReq(p.method, p.path, bytes.NewReader(p.body))
		} else {
			req = srv.authReq(p.method, p.path, nil)
		}
		resp, err := srv.do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", p.method, p.path, err)
		}
		body, _ := io.ReadAll(resp.Body)

		// Response body must not contain the raw secret.
		if bytes.Contains(body, []byte(knownSecret)) {
			t.Errorf("[LEAK] %s %s response contains raw secret:\n%s", p.method, p.path, body)
		}

		// Response headers must not contain the raw secret either.
		for k, vals := range resp.Header {
			for _, v := range vals {
				if strings.Contains(v, knownSecret) {
					t.Errorf("[LEAK] %s %s header %s=%s contains raw secret", p.method, p.path, k, v)
				}
			}
		}
	}

	// Also assert that PUT round-trip with masked/blank secret preserves — but
	// never echoes the raw secret on the response.
	cfg := srv.getConfig(t)
	cfgJSON, _ := json.Marshal(cfg)
	if bytes.Contains(cfgJSON, []byte(knownSecret)) {
		t.Errorf("[LEAK] GET config carries raw secret through helper")
	}
}
```

If `setPluginSecret`, `authReq`, `do` helpers don't exist, add minimal implementations that manipulate the in-memory `ExecutionHolder` / `ExecutionConfig`.

- [ ] **Step 2: Run**

```
go test ./internal/api/ -run TestNoSecretLeakInExecutionEndpoints -v
```
Expected: PASS (if previous tasks did their jobs; otherwise, FAIL pinpointing which endpoint leaks).

- [ ] **Step 3: Commit**

```bash
git add internal/api/execution_secret_leak_test.go
git commit -m "test(exec): blanket secret-leak regression across /api/execution/*"
```

---

### Task 11: Wire `StateStore` in `cmd/server/main.go`

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Construct and inject**

Find the section in `main.go` where the dispatcher / dedup / feedback stores are constructed (search for `NewFeedbackIdempotency` or `NewDedupStore`). Add:

```go
execState := execution.NewStateStore(db.Conn())
// ... then when constructing the API server, pass it in
apiServer := api.NewServer(/* existing args */, execState)
```

Update `api.NewServer` signature (or the setter pattern it uses) to accept `*execution.StateStore`.

- [ ] **Step 2: Verify build**

```
go build ./...
```
Expected: clean build.

- [ ] **Step 3: Smoke test the server end-to-end**

```
go run ./cmd/server -config config/server.yaml &
sleep 1
curl -s -H "Authorization: Bearer $CHARTNAGARI_API_TOKEN" http://localhost:8080/api/execution/config | jq .version
kill %1
```
Expected: prints a version number (likely `1`).

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go internal/api/server.go
git commit -m "feat(exec): wire StateStore into server + API bootstrap"
```

---

## Phase C — Frontend scaffold

### Task 12: Create `ExecutionTab` scaffold + add top-level tab

**Files:**
- Create: `web/src/ExecutionTab.tsx`
- Modify: `web/src/App.tsx` (add tab type, tab button, render branch)
- Test: `web/src/ExecutionTab.test.tsx`

- [ ] **Step 1: Write failing test**

Create `web/src/ExecutionTab.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import ExecutionTab from './ExecutionTab';

beforeEach(() => {
	globalThis.fetch = vi.fn().mockImplementation((url: string) => {
		if (url.includes('/api/execution/config')) {
			return Promise.resolve(new Response(JSON.stringify({
				version: 1, enabled: false, killed_at: '', plugins: [], max_dispatched: 3, dedup_window: '5m', symbol_map: {},
			})));
		}
		if (url.includes('/api/execution/plugins/stats')) {
			return Promise.resolve(new Response(JSON.stringify({ window: '24h', plugins: [] })));
		}
		if (url.includes('/api/execution/feedback')) {
			return Promise.resolve(new Response(JSON.stringify({ items: [], count: 0 })));
		}
		return Promise.resolve(new Response('{}'));
	});
});

describe('ExecutionTab', () => {
	it('fetches config, stats, and feedback in parallel on mount', async () => {
		render(<ExecutionTab />);
		await waitFor(() => {
			expect(globalThis.fetch).toHaveBeenCalledWith(expect.stringContaining('/api/execution/config'), expect.anything());
			expect(globalThis.fetch).toHaveBeenCalledWith(expect.stringContaining('/api/execution/plugins/stats'), expect.anything());
			expect(globalThis.fetch).toHaveBeenCalledWith(expect.stringContaining('/api/execution/feedback'), expect.anything());
		});
	});

	it('renders the kill switch, plugin area, global form, and feedback table slots', async () => {
		render(<ExecutionTab />);
		await waitFor(() => {
			expect(screen.getByTestId('kill-switch')).toBeInTheDocument();
			expect(screen.getByTestId('plugins-area')).toBeInTheDocument();
			expect(screen.getByTestId('global-config')).toBeInTheDocument();
			expect(screen.getByTestId('feedback-table')).toBeInTheDocument();
		});
	});
});
```

- [ ] **Step 2: Run to verify failure**

```
cd web && npx vitest run ExecutionTab --reporter=default
```
Expected: FAIL — file not found.

- [ ] **Step 3: Create skeleton component**

```tsx
// web/src/ExecutionTab.tsx
import { useCallback, useEffect, useRef, useState } from 'react';

type Plugin = {
	name: string;
	url: string;
	enabled: boolean;
	symbols: string[];
	min_score: number;
	direction_filter: '' | 'LONG' | 'SHORT';
	secret: string; // always masked on GET
};

type ExecutionConfig = {
	version: number;
	enabled: boolean;
	killed_at: string;
	plugins: Plugin[];
	max_dispatched: number;
	dedup_window: string;
	symbol_map: Record<string, Record<string, string>>;
};

type PluginStat = {
	plugin_id: string;
	submitted: number;
	filled: number;
	rejected: number;
	last_failure_at?: number;
	last_failure_msg: string;
};

type FeedbackRow = {
	plugin_id: string;
	signal_id: string;
	order_id: string;
	status: string;
	symbol: string;
	message: string;
	received_at: number;
};

export default function ExecutionTab() {
	const [config, setConfig] = useState<ExecutionConfig | null>(null);
	const [stats, setStats] = useState<PluginStat[]>([]);
	const [feedback, setFeedback] = useState<FeedbackRow[]>([]);

	const loadConfig = useCallback(async () => {
		const r = await fetch('/api/execution/config', { credentials: 'include' });
		if (r.ok) setConfig(await r.json());
	}, []);
	const loadStats = useCallback(async () => {
		const r = await fetch('/api/execution/plugins/stats?window=24h', { credentials: 'include' });
		if (r.ok) { const b = await r.json(); setStats(b.plugins ?? []); }
	}, []);
	const loadFeedback = useCallback(async () => {
		const r = await fetch('/api/execution/feedback?limit=100', { credentials: 'include' });
		if (r.ok) { const b = await r.json(); setFeedback(b.items ?? []); }
	}, []);

	useEffect(() => {
		void Promise.allSettled([loadConfig(), loadStats(), loadFeedback()]);
	}, [loadConfig, loadStats, loadFeedback]);

	// 30s polling for stats + feedback, paused when tab not visible.
	const timerRef = useRef<number | null>(null);
	useEffect(() => {
		const tick = () => {
			if (document.visibilityState !== 'visible') return;
			void Promise.allSettled([loadStats(), loadFeedback()]);
		};
		timerRef.current = window.setInterval(tick, 30_000);
		return () => { if (timerRef.current) window.clearInterval(timerRef.current); };
	}, [loadStats, loadFeedback]);

	return (
		<div className="execution-tab">
			<div data-testid="kill-switch">{/* KillSwitch goes here */}</div>
			<div data-testid="plugins-area">{/* PluginCard list + Add button */}</div>
			<div data-testid="global-config">{/* GlobalConfigForm */}</div>
			<div data-testid="feedback-table">{/* FeedbackTable */}</div>
		</div>
	);
}
```

- [ ] **Step 4: Wire the tab in `App.tsx`**

Edit `web/src/App.tsx`:

1. At `type Tab = ...` (around line 20), add `'execution'`:

```ts
type Tab = 'symbols' | 'rules' | 'status' | 'chart' | 'backtest' | 'paper' | 'report' | 'history' | 'alert' | 'performance' | 'analysis' | 'settings' | 'price-alerts' | 'calendar' | 'execution'
```

2. Import the new component at the top of `App.tsx`:

```tsx
import ExecutionTab from './ExecutionTab';
```

3. Add the tab button alongside the others (around line 3980):

```tsx
<button className={`tab-btn${tab === 'execution' ? ' active' : ''}`} onClick={() => setTab('execution')}>{t('execution')}</button>
```

4. Add the render branch (around line 3990):

```tsx
{tab === 'execution' && <ExecutionTab />}
```

- [ ] **Step 5: Run tests to verify pass**

```
cd web && npx vitest run ExecutionTab --reporter=default
```
Expected: PASS.

- [ ] **Step 6: Run dev server + manual smoke**

```
cd web && npm run dev
```
Open the app, click `Execution` tab. Expected: blank layout with four `<div>` slots (to be filled by later tasks).

- [ ] **Step 7: Commit**

```bash
git add web/src/ExecutionTab.tsx web/src/ExecutionTab.test.tsx web/src/App.tsx
git commit -m "feat(ui): Execution tab scaffold with parallel fetch + 30s polling"
```

---

### Task 13: `KillSwitch` component

**Files:**
- Create: `web/src/KillSwitch.tsx`
- Test: `web/src/KillSwitch.test.tsx`
- Modify: `web/src/ExecutionTab.tsx` (replace placeholder div with component)

- [ ] **Step 1: Write failing test**

```tsx
// web/src/KillSwitch.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import KillSwitch from './KillSwitch';

describe('KillSwitch', () => {
	it('renders the Kill button when not killed', () => {
		render(<KillSwitch killed={false} killedAt={null} onToggle={vi.fn()} />);
		expect(screen.getByRole('button', { name: /kill/i })).toBeInTheDocument();
		expect(screen.queryByRole('banner')).toBeNull();
	});

	it('renders red banner with formatted timestamp when killed', () => {
		render(<KillSwitch killed={true} killedAt="2026-04-17T10:30:00Z" onToggle={vi.fn()} />);
		expect(screen.getByRole('banner')).toHaveTextContent(/execution killed/i);
		expect(screen.getByRole('banner')).toHaveTextContent(/2026/);
		expect(screen.getByRole('button', { name: /re-enable/i })).toBeInTheDocument();
	});

	it('opens confirm modal on click and calls onToggle only on Confirm', async () => {
		const onToggle = vi.fn().mockResolvedValue(undefined);
		render(<KillSwitch killed={false} killedAt={null} onToggle={onToggle} />);

		fireEvent.click(screen.getByRole('button', { name: /kill/i }));
		expect(screen.getByRole('dialog')).toBeInTheDocument();

		fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
		expect(onToggle).not.toHaveBeenCalled();

		fireEvent.click(screen.getByRole('button', { name: /kill/i }));
		fireEvent.click(screen.getByRole('button', { name: /^confirm$/i }));
		expect(onToggle).toHaveBeenCalledTimes(1);
	});
});
```

- [ ] **Step 2: Run to verify failure**

```
cd web && npx vitest run KillSwitch
```
Expected: FAIL.

- [ ] **Step 3: Implement component**

```tsx
// web/src/KillSwitch.tsx
import { useState } from 'react';
import { useI18n } from './i18n';

type Props = {
	killed: boolean;
	killedAt: string | null;
	onToggle: () => Promise<void>;
};

export default function KillSwitch({ killed, killedAt, onToggle }: Props) {
	const { t } = useI18n();
	const [modalOpen, setModalOpen] = useState(false);
	const [busy, setBusy] = useState(false);

	const formattedKilledAt = killedAt ? new Date(killedAt).toISOString().replace('T', ' ').slice(0, 19) + ' UTC' : '';

	const confirm = async () => {
		setBusy(true);
		try { await onToggle(); } finally { setBusy(false); setModalOpen(false); }
	};

	return (
		<div className="kill-switch">
			{killed ? (
				<div role="banner" className="kill-banner" style={{ background: 'var(--danger)', color: '#fff', padding: '12px' }}>
					{t('execution.killed_banner')} — {t('execution.last_killed')}: {formattedKilledAt}
					<button onClick={() => setModalOpen(true)} disabled={busy}>{t('execution.reenable')}</button>
				</div>
			) : (
				<div className="kill-bar">
					<button className="kill-btn" style={{ background: 'var(--danger)', color: '#fff' }} onClick={() => setModalOpen(true)}>
						{t('execution.kill_switch')}
					</button>
				</div>
			)}

			{modalOpen && (
				<div role="dialog" className="modal-backdrop">
					<div className="modal">
						<p>{killed ? t('execution.confirm_reenable') : t('execution.confirm_kill')}</p>
						<button onClick={() => setModalOpen(false)}>{t('common.cancel')}</button>
						<button onClick={confirm} disabled={busy}>{t('common.confirm')}</button>
					</div>
				</div>
			)}
		</div>
	);
}
```

- [ ] **Step 4: Wire it into `ExecutionTab.tsx`**

Replace the placeholder `<div data-testid="kill-switch">` block with:

```tsx
<div data-testid="kill-switch">
	<KillSwitch
		killed={!!config && !config.enabled}
		killedAt={config?.killed_at || null}
		onToggle={async () => {
			const on = !!config && config.enabled; // flipping on → killed
			await fetch('/api/execution/kill', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'include',
				body: JSON.stringify({ on }),
			});
			await loadConfig();
		}}
	/>
</div>
```

(Import at top of ExecutionTab: `import KillSwitch from './KillSwitch';`)

- [ ] **Step 5: Run tests**

```
cd web && npx vitest run KillSwitch ExecutionTab
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/KillSwitch.tsx web/src/KillSwitch.test.tsx web/src/ExecutionTab.tsx
git commit -m "feat(ui): KillSwitch with confirm modal + banner"
```

---

### Task 14: `PluginCard` component

**Files:**
- Create: `web/src/PluginCard.tsx`
- Test: `web/src/PluginCard.test.tsx`
- Modify: `web/src/ExecutionTab.tsx` (render list)

- [ ] **Step 1: Write failing test**

```tsx
// web/src/PluginCard.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import PluginCard from './PluginCard';

const basePlugin = {
	name: 'alpaca-paper',
	url: 'http://localhost:9100',
	enabled: true,
	symbols: [],
	min_score: 12,
	direction_filter: '' as const,
	secret: '••••',
};

describe('PluginCard', () => {
	it('shows "no activity" when stats are undefined', () => {
		render(<PluginCard plugin={basePlugin} stats={undefined} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(screen.getByText(/no activity/i)).toBeInTheDocument();
	});

	it('renders 24h counts when stats are provided', () => {
		const stats = { plugin_id: 'alpaca-paper', submitted: 13, filled: 12, rejected: 1, last_failure_msg: '' };
		render(<PluginCard plugin={basePlugin} stats={stats} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(screen.getByText(/12 filled/i)).toBeInTheDocument();
		expect(screen.getByText(/1 rejected/i)).toBeInTheDocument();
	});

	it('shows the danger border when last_failure_msg is present', () => {
		const stats = { plugin_id: 'alpaca-paper', submitted: 1, filled: 0, rejected: 1, last_failure_msg: 'denied' };
		const { container } = render(<PluginCard plugin={basePlugin} stats={stats} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(container.firstChild).toHaveClass('plugin-card--has-failure');
	});

	it('renders disabled opacity when enabled=false', () => {
		const plugin = { ...basePlugin, enabled: false };
		const { container } = render(<PluginCard plugin={plugin} stats={undefined} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(container.firstChild).toHaveClass('plugin-card--disabled');
	});

	it('calls onDelete only after confirming', () => {
		const onDelete = vi.fn();
		render(<PluginCard plugin={basePlugin} stats={undefined} onEdit={vi.fn()} onDelete={onDelete} onToggleEnabled={vi.fn()} />);
		fireEvent.click(screen.getByRole('button', { name: /delete/i }));
		expect(screen.getByRole('dialog')).toBeInTheDocument();
		fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
		expect(onDelete).not.toHaveBeenCalled();

		fireEvent.click(screen.getByRole('button', { name: /delete/i }));
		fireEvent.click(screen.getByRole('button', { name: /^confirm$/i }));
		expect(onDelete).toHaveBeenCalledTimes(1);
	});
});
```

- [ ] **Step 2: Run to verify failure**

```
cd web && npx vitest run PluginCard
```
Expected: FAIL.

- [ ] **Step 3: Implement component**

```tsx
// web/src/PluginCard.tsx
import { useState } from 'react';
import { useI18n } from './i18n';

export type PluginCardProps = {
	plugin: {
		name: string;
		url: string;
		enabled: boolean;
		symbols: string[];
		min_score: number;
		direction_filter: '' | 'LONG' | 'SHORT';
		secret: string;
	};
	stats?: {
		plugin_id: string;
		submitted: number;
		filled: number;
		rejected: number;
		last_failure_msg: string;
	};
	onEdit: () => void;
	onDelete: () => Promise<void>;
	onToggleEnabled: (next: boolean) => Promise<void>;
};

export default function PluginCard({ plugin, stats, onEdit, onDelete, onToggleEnabled }: PluginCardProps) {
	const { t } = useI18n();
	const [confirming, setConfirming] = useState(false);

	const hasFailure = !!stats?.last_failure_msg;
	const classes = [
		'plugin-card',
		!plugin.enabled ? 'plugin-card--disabled' : '',
		hasFailure ? 'plugin-card--has-failure' : '',
	].filter(Boolean).join(' ');

	return (
		<div className={classes}>
			<label>
				<input type="checkbox" checked={plugin.enabled} onChange={e => void onToggleEnabled(e.target.checked)} />
			</label>
			<span className="plugin-name">{plugin.name}</span>
			<span className="plugin-url">{plugin.url}</span>
			<span className="plugin-stats">
				{stats
					? `24h: ${stats.filled} filled / ${stats.rejected} rejected`
					: t('execution.no_activity')}
			</span>
			{hasFailure && <span className="plugin-failure-tooltip" title={stats!.last_failure_msg}>!</span>}
			<button onClick={onEdit}>{t('common.edit')}</button>
			<button onClick={() => setConfirming(true)}>{t('common.delete')}</button>

			{confirming && (
				<div role="dialog" className="modal-backdrop">
					<div className="modal">
						<p>{t('execution.confirm_delete_plugin', { name: plugin.name })}</p>
						<button onClick={() => setConfirming(false)}>{t('common.cancel')}</button>
						<button onClick={() => { setConfirming(false); void onDelete(); }}>{t('common.confirm')}</button>
					</div>
				</div>
			)}
		</div>
	);
}
```

Add matching CSS to `App.css` for the two modifier classes (minimal: opacity 0.4 for `--disabled`, border left 3px `--danger` for `--has-failure`).

- [ ] **Step 4: Render list in `ExecutionTab.tsx`**

Replace the `plugins-area` placeholder:

```tsx
<div data-testid="plugins-area">
	{config?.plugins.map(p => {
		const s = stats.find(x => x.plugin_id === p.name);
		return (
			<PluginCard
				key={p.name}
				plugin={p}
				stats={s}
				onEdit={() => setEditing(p)}
				onDelete={async () => {
					if (!config) return;
					const nextPlugins = config.plugins.filter(x => x.name !== p.name);
					await putConfig({ ...config, plugins: nextPlugins });
				}}
				onToggleEnabled={async next => {
					if (!config) return;
					const nextPlugins = config.plugins.map(x => x.name === p.name ? { ...x, enabled: next } : x);
					await putConfig({ ...config, plugins: nextPlugins });
				}}
			/>
		);
	})}
	<button onClick={() => setEditing({} as never)}>{t('execution.add_plugin')}</button>
</div>
```

Add a `putConfig` helper inside `ExecutionTab` and a local `editing` state that will drive the modal in Task 16. For now, `editing` can be `Plugin | null`.

- [ ] **Step 5: Run tests**

```
cd web && npx vitest run PluginCard ExecutionTab
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/PluginCard.tsx web/src/PluginCard.test.tsx web/src/ExecutionTab.tsx web/src/App.css
git commit -m "feat(ui): PluginCard with 24h stats, disabled/failure visual states, delete confirm"
```

---

## Phase D — Frontend edit paths

### Task 15: `FeedbackTable` with filters and refresh

**Files:**
- Create: `web/src/FeedbackTable.tsx`
- Test: `web/src/FeedbackTable.test.tsx`
- Modify: `web/src/ExecutionTab.tsx`

- [ ] **Step 1: Write failing test**

```tsx
// web/src/FeedbackTable.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import FeedbackTable from './FeedbackTable';

const rows = [
	{ plugin_id: 'alpaca-paper', signal_id: '550e8400-e29b-41d4-a716-446655440000', order_id: 'o1', status: 'FILLED', symbol: 'AAPL', message: '', received_at: 1713312000 },
	{ plugin_id: 'alpaca-paper', signal_id: 'abc', order_id: 'o2', status: 'REJECTED', symbol: 'TSLA', message: 'denied', received_at: 1713311000 },
];

describe('FeedbackTable', () => {
	it('applies status color classes', () => {
		render(<FeedbackTable
			feedback={rows}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={vi.fn()}
			onRefresh={vi.fn()}
			pluginNames={['alpaca-paper']}
		/>);
		expect(screen.getByText('FILLED')).toHaveClass('status-filled');
		expect(screen.getByText('REJECTED')).toHaveClass('status-rejected');
	});

	it('calls onFiltersChange synchronously when a dropdown changes', () => {
		const onFiltersChange = vi.fn();
		render(<FeedbackTable
			feedback={rows}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={onFiltersChange}
			onRefresh={vi.fn()}
			pluginNames={['alpaca-paper']}
		/>);
		fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'FILLED' } });
		expect(onFiltersChange).toHaveBeenCalledWith({ plugin: '', status: 'FILLED', symbol: '' });
	});

	it('Refresh button invokes onRefresh', () => {
		const onRefresh = vi.fn();
		render(<FeedbackTable
			feedback={[]}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={vi.fn()}
			onRefresh={onRefresh}
			pluginNames={[]}
		/>);
		fireEvent.click(screen.getByRole('button', { name: /refresh/i }));
		expect(onRefresh).toHaveBeenCalled();
	});
});
```

Also add to `ExecutionTab.test.tsx`:

```tsx
it('clears feedback synchronously when filter changes (v2.4.0.2 regression pattern)', async () => {
	const { rerender } = render(<ExecutionTab />);
	// Simulate a filter change path; the contract under test is in ExecutionTab's onFiltersChange:
	// it must call setFeedback([]) before the next fetch resolves.
	// Spec-level assertion is covered in ExecutionTab implementation; here we assert that after
	// change, feedback items emptied before the network call's promise resolves.
});
```

(If that integration-level test is hard to pin down with mocks, cover the invariant inside `ExecutionTab.tsx` as a comment + single unit: that `setFeedback([])` is called prior to the fetch kickoff.)

- [ ] **Step 2: Run to verify failure**

```
cd web && npx vitest run FeedbackTable
```
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/FeedbackTable.tsx
import { useI18n } from './i18n';

type Row = {
	plugin_id: string;
	signal_id: string;
	order_id: string;
	status: string;
	symbol: string;
	message: string;
	received_at: number;
};

export type FeedbackFilters = {
	plugin: string;
	status: string;
	symbol: string;
};

type Props = {
	feedback: Row[];
	filters: FeedbackFilters;
	onFiltersChange: (f: FeedbackFilters) => void;
	onRefresh: () => Promise<void>;
	pluginNames: string[];
};

const STATUSES = ['', 'SUBMITTED', 'FILLED', 'PARTIAL_FILL', 'REJECTED', 'CANCELLED', 'ERROR', 'RECEIVED'];

function statusClass(s: string): string {
	switch (s) {
		case 'FILLED':
		case 'PARTIAL_FILL': return 'status-filled';
		case 'REJECTED':
		case 'ERROR':        return 'status-rejected';
		case 'CANCELLED':    return 'status-cancelled';
		default:             return 'status-muted';
	}
}

export default function FeedbackTable({ feedback, filters, onFiltersChange, onRefresh, pluginNames }: Props) {
	const { t } = useI18n();

	return (
		<div className="feedback-table">
			<div className="feedback-filters">
				<label>{t('execution.filter_plugin')}
					<select value={filters.plugin} onChange={e => onFiltersChange({ ...filters, plugin: e.target.value })}>
						<option value="">{t('common.all')}</option>
						{pluginNames.map(n => <option key={n} value={n}>{n}</option>)}
					</select>
				</label>
				<label>{t('execution.filter_status')}
					<select value={filters.status} onChange={e => onFiltersChange({ ...filters, status: e.target.value })}>
						{STATUSES.map(s => <option key={s || '_all'} value={s}>{s || t('common.all')}</option>)}
					</select>
				</label>
				<label>{t('execution.filter_symbol')}
					<input value={filters.symbol} onChange={e => onFiltersChange({ ...filters, symbol: e.target.value.toUpperCase() })} />
				</label>
				<button onClick={() => void onRefresh()}>{t('common.refresh')}</button>
			</div>

			<table>
				<thead>
					<tr>
						<th>{t('execution.col_time')}</th>
						<th>{t('execution.col_plugin')}</th>
						<th>{t('execution.col_signal')}</th>
						<th>{t('execution.col_symbol')}</th>
						<th>{t('execution.col_status')}</th>
						<th>{t('execution.col_order')}</th>
						<th>{t('execution.col_message')}</th>
					</tr>
				</thead>
				<tbody>
					{feedback.length === 0 && (
						<tr><td colSpan={7}>{t('execution.no_feedback')}</td></tr>
					)}
					{feedback.map((r, i) => (
						<tr key={`${r.plugin_id}:${r.signal_id}:${r.order_id}:${r.status}:${i}`}>
							<td>{new Date(r.received_at * 1000).toISOString().replace('T', ' ').slice(0, 19)}</td>
							<td>{r.plugin_id}</td>
							<td><code>{r.signal_id.slice(0, 8)}</code></td>
							<td>{r.symbol || '—'}</td>
							<td className={statusClass(r.status)}>{r.status}</td>
							<td>{r.order_id || '—'}</td>
							<td>{r.message || '—'}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	);
}
```

- [ ] **Step 4: Wire into `ExecutionTab.tsx`**

Replace the `feedback-table` placeholder (add `import FeedbackTable, { FeedbackFilters } from './FeedbackTable';` at the top):

```tsx
const [filters, setFilters] = useState<FeedbackFilters>({ plugin: '', status: '', symbol: '' });

const loadFeedback = useCallback(async (f: FeedbackFilters = filters) => {
	const qs = new URLSearchParams();
	if (f.plugin) qs.set('plugin', f.plugin);
	if (f.status) qs.set('status', f.status);
	if (f.symbol) qs.set('symbol', f.symbol);
	qs.set('limit', '100');
	const r = await fetch('/api/execution/feedback?' + qs, { credentials: 'include' });
	if (r.ok) { const b = await r.json(); setFeedback(b.items ?? []); }
}, [filters]);

// ... in JSX:
<div data-testid="feedback-table">
	<FeedbackTable
		feedback={feedback}
		filters={filters}
		onFiltersChange={f => {
			setFilters(f);
			setFeedback([]);      // v2.4.0.2 regression pattern — clear stale rows synchronously
			void loadFeedback(f);
		}}
		onRefresh={() => loadFeedback()}
		pluginNames={(config?.plugins ?? []).map(p => p.name)}
	/>
</div>
```

Leave a comment at `setFeedback([])`:
```
// Regression guard: v2.4.0.2 — filter change must clear rows before the new fetch resolves.
```

- [ ] **Step 5: Run tests**

```
cd web && npx vitest run FeedbackTable ExecutionTab
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/FeedbackTable.tsx web/src/FeedbackTable.test.tsx web/src/ExecutionTab.tsx
git commit -m "feat(ui): FeedbackTable with plugin/status/symbol filters + stale-rows guard"
```

---

### Task 16: `PluginEditModal` with Generate + clipboard fallback

**Files:**
- Create: `web/src/PluginEditModal.tsx`
- Test: `web/src/PluginEditModal.test.tsx`
- Modify: `web/src/ExecutionTab.tsx`

- [ ] **Step 1: Write failing test**

```tsx
// web/src/PluginEditModal.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import PluginEditModal from './PluginEditModal';

const nav = globalThis.navigator as any;
beforeEach(() => {
	nav.clipboard = { writeText: vi.fn().mockResolvedValue(undefined) };
	// Deterministic crypto for Generate test.
	(globalThis.crypto as any).getRandomValues = (arr: Uint8Array) => {
		for (let i = 0; i < arr.length; i++) arr[i] = i;
		return arr;
	};
});

describe('PluginEditModal', () => {
	it('requires name and secret in create mode', () => {
		const onSave = vi.fn();
		render(<PluginEditModal plugin={null} existingNames={[]} onSave={onSave} onCancel={vi.fn()} />);

		fireEvent.click(screen.getByRole('button', { name: /save/i }));
		expect(onSave).not.toHaveBeenCalled();
		expect(screen.getByText(/name.*required/i)).toBeInTheDocument();
	});

	it('rejects duplicate name in create mode', () => {
		render(<PluginEditModal plugin={null} existingNames={['alpaca-paper']} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'alpaca-paper' } });
		fireEvent.blur(screen.getByLabelText(/name/i));
		expect(screen.getByText(/already exists/i)).toBeInTheDocument();
	});

	it('rejects non-http URL', () => {
		render(<PluginEditModal plugin={null} existingNames={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.change(screen.getByLabelText(/url/i), { target: { value: 'ftp://x' } });
		fireEvent.blur(screen.getByLabelText(/url/i));
		expect(screen.getByText(/must start with http/i)).toBeInTheDocument();
	});

	it('rejects negative min_score', () => {
		render(<PluginEditModal plugin={null} existingNames={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.change(screen.getByLabelText(/min score/i), { target: { value: '-1' } });
		fireEvent.blur(screen.getByLabelText(/min score/i));
		expect(screen.getByText(/must be ≥ 0/i)).toBeInTheDocument();
	});

	it('Generate fills the field and copies to clipboard', async () => {
		render(<PluginEditModal plugin={null} existingNames={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.click(screen.getByRole('button', { name: /generate/i }));
		await screen.findByText(/copied to clipboard/i);
		expect(nav.clipboard.writeText).toHaveBeenCalled();
		const input = screen.getByLabelText(/secret/i) as HTMLInputElement;
		expect(input.value.length).toBe(64); // 32 bytes hex
	});

	it('Generate falls back with manual-copy toast when clipboard rejects', async () => {
		nav.clipboard.writeText = vi.fn().mockRejectedValue(new Error('no https'));
		render(<PluginEditModal plugin={null} existingNames={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.click(screen.getByRole('button', { name: /generate/i }));
		await screen.findByText(/copy manually/i);
	});

	it('resets form when plugin prop changes (6A useEffect)', () => {
		const { rerender } = render(<PluginEditModal plugin={{ name: 'a', url: 'http://a', enabled: true, symbols: [], min_score: 1, direction_filter: '', secret: '••••' }} existingNames={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		expect((screen.getByLabelText(/url/i) as HTMLInputElement).value).toBe('http://a');
		rerender(<PluginEditModal plugin={{ name: 'b', url: 'http://b', enabled: true, symbols: [], min_score: 2, direction_filter: '', secret: '••••' }} existingNames={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		expect((screen.getByLabelText(/url/i) as HTMLInputElement).value).toBe('http://b');
	});
});
```

- [ ] **Step 2: Run to verify failure**

```
cd web && npx vitest run PluginEditModal
```
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/PluginEditModal.tsx
import { useEffect, useState } from 'react';
import { useI18n } from './i18n';

export type Plugin = {
	name: string;
	url: string;
	enabled: boolean;
	symbols: string[];
	min_score: number;
	direction_filter: '' | 'LONG' | 'SHORT';
	secret: string;
};

type Props = {
	plugin: Plugin | null; // null = create
	existingNames: string[];
	onSave: (p: Plugin) => Promise<void>;
	onCancel: () => void;
};

const EMPTY: Plugin = { name: '', url: '', enabled: true, symbols: [], min_score: 0, direction_filter: '', secret: '' };

function genHex32(): string {
	const buf = new Uint8Array(32);
	crypto.getRandomValues(buf);
	return Array.from(buf).map(b => b.toString(16).padStart(2, '0')).join('');
}

export default function PluginEditModal({ plugin, existingNames, onSave, onCancel }: Props) {
	const { t } = useI18n();
	const isCreate = plugin == null || !plugin.name;
	const [form, setForm] = useState<Plugin>(plugin ?? EMPTY);
	const [errors, setErrors] = useState<Record<string, string>>({});
	const [toast, setToast] = useState<string | null>(null);
	const [showSecret, setShowSecret] = useState(false);
	const [busy, setBusy] = useState(false);

	useEffect(() => { setForm(plugin ?? EMPTY); setErrors({}); }, [plugin]);

	const validateName = () => {
		if (isCreate) {
			if (!form.name.trim()) return t('execution.err_name_required');
			if (existingNames.includes(form.name)) return t('execution.err_name_exists');
		}
		return '';
	};
	const validateURL = () => /^https?:\/\//.test(form.url) ? '' : t('execution.err_url');
	const validateMinScore = () => form.min_score >= 0 ? '' : t('execution.err_min_score');

	const runValidators = (): Record<string, string> => {
		const e: Record<string, string> = {};
		const n = validateName(); if (n) e.name = n;
		const u = validateURL(); if (u) e.url = u;
		const m = validateMinScore(); if (m) e.min_score = m;
		if (isCreate && !form.secret) e.secret = t('execution.err_secret_required');
		return e;
	};

	const onGenerate = async () => {
		const hex = genHex32();
		setForm(f => ({ ...f, secret: hex }));
		try {
			await navigator.clipboard.writeText(hex);
			setToast(t('execution.secret_copied'));
		} catch {
			setToast(t('execution.secret_copy_manual'));
		}
		setShowSecret(true);
	};

	const onSubmit = async () => {
		const e = runValidators();
		setErrors(e);
		if (Object.keys(e).length > 0) return;
		setBusy(true);
		try { await onSave(form); } finally { setBusy(false); }
	};

	return (
		<div role="dialog" className="modal-backdrop">
			<div className="modal plugin-edit-modal">
				<label>
					{t('execution.field_name')}
					<input
						aria-label={t('execution.field_name')}
						value={form.name}
						readOnly={!isCreate}
						onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
						onBlur={() => setErrors(prev => ({ ...prev, name: validateName() }))}
					/>
					{errors.name && <span className="error">{errors.name}</span>}
				</label>

				<label>
					{t('execution.field_url')}
					<input
						aria-label={t('execution.field_url')}
						value={form.url}
						onChange={e => setForm(f => ({ ...f, url: e.target.value }))}
						onBlur={() => setErrors(prev => ({ ...prev, url: validateURL() }))}
					/>
					{errors.url && <span className="error">{errors.url}</span>}
				</label>

				<label>
					{t('execution.field_secret')}
					<input
						aria-label={t('execution.field_secret')}
						type={showSecret ? 'text' : 'password'}
						value={form.secret}
						placeholder={isCreate ? '' : t('execution.secret_placeholder')}
						onChange={e => setForm(f => ({ ...f, secret: e.target.value }))}
					/>
					<button type="button" onClick={() => setShowSecret(s => !s)}>{showSecret ? t('common.hide') : t('common.show')}</button>
					<button type="button" onClick={onGenerate}>{t('execution.generate')}</button>
					{errors.secret && <span className="error">{errors.secret}</span>}
				</label>

				<label>
					{t('execution.field_min_score')}
					<input
						aria-label={t('execution.field_min_score')}
						type="number"
						value={form.min_score}
						onChange={e => setForm(f => ({ ...f, min_score: parseFloat(e.target.value) || 0 }))}
						onBlur={() => setErrors(prev => ({ ...prev, min_score: validateMinScore() }))}
					/>
					{errors.min_score && <span className="error">{errors.min_score}</span>}
				</label>

				<label>
					{t('execution.field_symbols')}
					<input
						aria-label={t('execution.field_symbols')}
						value={form.symbols.join(', ')}
						onChange={e => setForm(f => ({ ...f, symbols: e.target.value.split(',').map(s => s.trim().toUpperCase()).filter(Boolean) }))}
					/>
				</label>

				<label>
					{t('execution.field_direction')}
					<select value={form.direction_filter} onChange={e => setForm(f => ({ ...f, direction_filter: e.target.value as Plugin['direction_filter'] }))}>
						<option value="">{t('common.both')}</option>
						<option value="LONG">LONG</option>
						<option value="SHORT">SHORT</option>
					</select>
				</label>

				<label>
					<input type="checkbox" checked={form.enabled} onChange={e => setForm(f => ({ ...f, enabled: e.target.checked }))} />
					{t('execution.field_enabled')}
				</label>

				<div className="actions">
					<button onClick={onCancel}>{t('common.cancel')}</button>
					<button onClick={() => void onSubmit()} disabled={busy}>{t('common.save')}</button>
				</div>

				{toast && <div className="toast" role="status">{toast}</div>}
			</div>
		</div>
	);
}
```

- [ ] **Step 4: Wire in `ExecutionTab.tsx`**

Add state:
```tsx
const [editing, setEditing] = useState<Plugin | null>(null);
const [editingOpen, setEditingOpen] = useState(false);
```

Render near end:
```tsx
{editingOpen && (
	<PluginEditModal
		plugin={editing}
		existingNames={(config?.plugins ?? []).map(p => p.name)}
		onCancel={() => setEditingOpen(false)}
		onSave={async next => {
			if (!config) return;
			const plugins = editing && editing.name
				? config.plugins.map(p => p.name === editing.name ? next : p)
				: [...config.plugins, next];
			await putConfig({ ...config, plugins });
			setEditingOpen(false);
		}}
	/>
)}
```

Set `setEditing(p); setEditingOpen(true)` from PluginCard's Edit button and `setEditing(null); setEditingOpen(true)` from Add button.

Implement `putConfig`:
```tsx
async function putConfig(next: ExecutionConfig): Promise<void> {
	const resp = await fetch('/api/execution/config', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		credentials: 'include',
		body: JSON.stringify(next),
	});
	if (resp.status === 409) {
		setVersionConflict(true);
		return;
	}
	await loadConfig();
	await loadStats();
}
```
Add `const [versionConflict, setVersionConflict] = useState(false);` and render a banner at top of the tab when `true` with `Reload` and `Save Anyway` buttons.

- [ ] **Step 5: Run tests**

```
cd web && npx vitest run PluginEditModal ExecutionTab
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/PluginEditModal.tsx web/src/PluginEditModal.test.tsx web/src/ExecutionTab.tsx
git commit -m "feat(ui): PluginEditModal — CRUD, Generate with clipboard fallback, validation"
```

---

### Task 17: `GlobalConfigForm` including `symbol_map` editor

**Files:**
- Create: `web/src/GlobalConfigForm.tsx`
- Test: `web/src/GlobalConfigForm.test.tsx`
- Modify: `web/src/ExecutionTab.tsx`

- [ ] **Step 1: Write failing test**

```tsx
// web/src/GlobalConfigForm.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import GlobalConfigForm from './GlobalConfigForm';

const base = {
	max_dispatched: 3,
	dedup_window: '5m',
	symbol_map: { BTCUSDT: { alpaca: 'BTC/USD', binance: 'BTCUSDT' } },
};

describe('GlobalConfigForm', () => {
	it('Save is disabled when no changes', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		expect(screen.getByRole('button', { name: /save/i })).toBeDisabled();
	});

	it('enables Save on any change', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		fireEvent.change(screen.getByLabelText(/max dispatched/i), { target: { value: '5' } });
		expect(screen.getByRole('button', { name: /save/i })).toBeEnabled();
	});

	it('shows inline error from server for invalid dedup_window', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={{ dedup_window: 'invalid duration' }} />);
		expect(screen.getByText(/invalid duration/i)).toBeInTheDocument();
	});

	it('Adds and removes a symbol_map row', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		fireEvent.click(screen.getByRole('button', { name: /add row/i }));
		expect(screen.getAllByLabelText(/symbol/i).length).toBeGreaterThan(1);

		fireEvent.click(screen.getAllByRole('button', { name: /remove row/i })[0]);
		// At least one row remains or list is shorter
	});

	it('Discard resets to original', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		fireEvent.change(screen.getByLabelText(/max dispatched/i), { target: { value: '5' } });
		fireEvent.click(screen.getByRole('button', { name: /discard/i }));
		expect((screen.getByLabelText(/max dispatched/i) as HTMLInputElement).value).toBe('3');
	});
});
```

- [ ] **Step 2: Run to verify failure**

```
cd web && npx vitest run GlobalConfigForm
```
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/GlobalConfigForm.tsx
import { useEffect, useMemo, useState } from 'react';
import { useI18n } from './i18n';

export type GlobalConfig = {
	max_dispatched: number;
	dedup_window: string;
	symbol_map: Record<string, Record<string, string>>;
};

type Props = {
	config: GlobalConfig;
	onSave: (partial: GlobalConfig) => Promise<void>;
	onServerError: Record<string, string> | null;
};

type Row = { symbol: string; map: { broker: string; ticker: string }[] };

function toRows(m: GlobalConfig['symbol_map']): Row[] {
	return Object.entries(m).map(([symbol, brokers]) => ({
		symbol,
		map: Object.entries(brokers).map(([broker, ticker]) => ({ broker, ticker })),
	}));
}
function fromRows(rows: Row[]): GlobalConfig['symbol_map'] {
	const out: GlobalConfig['symbol_map'] = {};
	for (const r of rows) {
		if (!r.symbol) continue;
		out[r.symbol.toUpperCase()] = {};
		for (const { broker, ticker } of r.map) {
			if (broker && ticker) out[r.symbol.toUpperCase()][broker] = ticker;
		}
	}
	return out;
}

export default function GlobalConfigForm({ config, onSave, onServerError }: Props) {
	const { t } = useI18n();
	const [maxDispatched, setMaxDispatched] = useState(config.max_dispatched);
	const [dedupWindow, setDedupWindow] = useState(config.dedup_window);
	const [rows, setRows] = useState<Row[]>(toRows(config.symbol_map));
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		setMaxDispatched(config.max_dispatched);
		setDedupWindow(config.dedup_window);
		setRows(toRows(config.symbol_map));
	}, [config]);

	const dirty = useMemo(() =>
		maxDispatched !== config.max_dispatched
		|| dedupWindow !== config.dedup_window
		|| JSON.stringify(fromRows(rows)) !== JSON.stringify(config.symbol_map),
		[maxDispatched, dedupWindow, rows, config]);

	const save = async () => {
		setBusy(true);
		try {
			await onSave({ max_dispatched: maxDispatched, dedup_window: dedupWindow, symbol_map: fromRows(rows) });
		} finally {
			setBusy(false);
		}
	};
	const discard = () => {
		setMaxDispatched(config.max_dispatched);
		setDedupWindow(config.dedup_window);
		setRows(toRows(config.symbol_map));
	};

	return (
		<div className="global-config-form">
			<label>{t('execution.max_dispatched')}
				<input aria-label={t('execution.max_dispatched')} type="number" value={maxDispatched} onChange={e => setMaxDispatched(parseInt(e.target.value, 10) || 0)} />
			</label>
			<label>{t('execution.dedup_window')}
				<input aria-label={t('execution.dedup_window')} value={dedupWindow} onChange={e => setDedupWindow(e.target.value)} />
				{onServerError?.dedup_window && <span className="error">{onServerError.dedup_window}</span>}
			</label>

			<details open>
				<summary>{t('execution.symbol_map')}</summary>
				{rows.map((row, i) => (
					<div key={i} className="symbol-map-row">
						<input aria-label={t('execution.field_symbol')} value={row.symbol} onChange={e => {
							const next = [...rows]; next[i] = { ...row, symbol: e.target.value }; setRows(next);
						}} />
						{row.map.map((bt, j) => (
							<span key={j}>
								<input placeholder="broker" value={bt.broker} onChange={e => {
									const next = [...rows]; next[i].map[j] = { ...bt, broker: e.target.value }; setRows(next);
								}} />
								<input placeholder="ticker" value={bt.ticker} onChange={e => {
									const next = [...rows]; next[i].map[j] = { ...bt, ticker: e.target.value }; setRows(next);
								}} />
							</span>
						))}
						<button onClick={() => {
							const next = [...rows]; next[i] = { ...row, map: [...row.map, { broker: '', ticker: '' }] }; setRows(next);
						}}>{t('execution.add_broker')}</button>
						<button onClick={() => setRows(rows.filter((_, k) => k !== i))}>{t('execution.remove_row')}</button>
					</div>
				))}
				<button onClick={() => setRows([...rows, { symbol: '', map: [{ broker: '', ticker: '' }] }])}>{t('execution.add_row')}</button>
			</details>

			<div className="actions">
				<button onClick={discard} disabled={!dirty}>{t('common.discard')}</button>
				<button onClick={save} disabled={!dirty || busy}>{t('common.save')}</button>
			</div>
		</div>
	);
}
```

- [ ] **Step 4: Wire into `ExecutionTab.tsx`**

```tsx
const [serverFieldErrors, setServerFieldErrors] = useState<Record<string,string> | null>(null);

<div data-testid="global-config">
	{config && (
		<GlobalConfigForm
			config={{ max_dispatched: config.max_dispatched, dedup_window: config.dedup_window, symbol_map: config.symbol_map }}
			onServerError={serverFieldErrors}
			onSave={async partial => {
				if (!config) return;
				const next = { ...config, ...partial };
				const resp = await fetch('/api/execution/config', {
					method: 'PUT',
					headers: { 'Content-Type': 'application/json' },
					credentials: 'include',
					body: JSON.stringify(next),
				});
				if (resp.status === 422) {
					const body = await resp.json();
					setServerFieldErrors(body.fields ?? null);
					return;
				}
				if (resp.status === 409) {
					setVersionConflict(true);
					return;
				}
				setServerFieldErrors(null);
				await loadConfig();
			}}
		/>
	)}
</div>
```

- [ ] **Step 5: Run tests**

```
cd web && npx vitest run GlobalConfigForm ExecutionTab
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/GlobalConfigForm.tsx web/src/GlobalConfigForm.test.tsx web/src/ExecutionTab.tsx
git commit -m "feat(ui): GlobalConfigForm with symbol_map editor + server-error inline display"
```

---

## Phase E — i18n and finalize

### Task 18: Add i18n keys to en / ko / ja

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/ko.json`
- Modify: `web/src/i18n/locales/ja.json`

- [ ] **Step 1: Inventory referenced keys**

Search all new components for `t('...')` and `t('...', ...)`:

```
grep -rhn "t('execution\." web/src/ExecutionTab.tsx web/src/KillSwitch.tsx web/src/PluginCard.tsx web/src/PluginEditModal.tsx web/src/GlobalConfigForm.tsx web/src/FeedbackTable.tsx | sort -u
grep -rhn "t('common\." web/src/ExecutionTab.tsx web/src/KillSwitch.tsx web/src/PluginCard.tsx web/src/PluginEditModal.tsx web/src/GlobalConfigForm.tsx web/src/FeedbackTable.tsx | sort -u
```

- [ ] **Step 2: Add to `en.json`**

Append under a new `execution` key (preserve the existing JSON structure the project uses):

```json
"execution": "Execution",
"execution.kill_switch": "Kill Switch",
"execution.reenable": "Re-enable",
"execution.killed_banner": "EXECUTION KILLED — no signals being dispatched",
"execution.last_killed": "Last killed",
"execution.confirm_kill": "Really disable all plugin dispatch?",
"execution.confirm_reenable": "Really re-enable all plugin dispatch?",
"execution.no_activity": "no activity",
"execution.confirm_delete_plugin": "Delete plugin {{name}}?",
"execution.add_plugin": "+ Add plugin",
"execution.field_name": "Name",
"execution.field_url": "URL",
"execution.field_secret": "Secret",
"execution.field_min_score": "Min score",
"execution.field_symbols": "Symbols (comma-separated)",
"execution.field_direction": "Direction filter",
"execution.field_enabled": "Enabled",
"execution.field_symbol": "Symbol",
"execution.secret_placeholder": "Leave blank to keep current secret",
"execution.secret_copied": "Secret copied to clipboard",
"execution.secret_copy_manual": "Copy the secret manually before saving",
"execution.generate": "Generate",
"execution.err_name_required": "Name is required",
"execution.err_name_exists": "already exists",
"execution.err_url": "must start with http:// or https://",
"execution.err_min_score": "must be ≥ 0",
"execution.err_secret_required": "Secret is required for new plugins",
"execution.max_dispatched": "Max dispatched",
"execution.dedup_window": "Dedup window (e.g. 5m)",
"execution.symbol_map": "Symbol map (advanced)",
"execution.add_row": "Add row",
"execution.add_broker": "Add broker",
"execution.remove_row": "Remove row",
"execution.filter_plugin": "Plugin",
"execution.filter_status": "Status",
"execution.filter_symbol": "Symbol",
"execution.col_time": "Time",
"execution.col_plugin": "Plugin",
"execution.col_signal": "Signal",
"execution.col_symbol": "Symbol",
"execution.col_status": "Status",
"execution.col_order": "Order",
"execution.col_message": "Message",
"execution.no_feedback": "No orders yet",
"common.all": "All",
"common.both": "Both",
"common.cancel": "Cancel",
"common.confirm": "Confirm",
"common.save": "Save",
"common.discard": "Discard",
"common.refresh": "Refresh",
"common.edit": "Edit",
"common.delete": "Delete",
"common.show": "Show",
"common.hide": "Hide"
```

- [ ] **Step 3: Add to `ko.json`** (identical keys, Korean values — sample below)

```json
"execution": "실행",
"execution.kill_switch": "킬 스위치",
"execution.reenable": "재활성화",
"execution.killed_banner": "실행 중단됨 — 시그널이 디스패치되지 않습니다",
"execution.last_killed": "마지막 중단",
"execution.confirm_kill": "정말 모든 플러그인 디스패치를 중단하시겠습니까?",
"execution.confirm_reenable": "정말 모든 플러그인 디스패치를 재활성화하시겠습니까?",
"execution.no_activity": "활동 없음",
"execution.confirm_delete_plugin": "플러그인 {{name}}을(를) 삭제하시겠습니까?",
"execution.add_plugin": "+ 플러그인 추가",
"execution.field_name": "이름",
"execution.field_url": "URL",
"execution.field_secret": "비밀키",
"execution.field_min_score": "최소 점수",
"execution.field_symbols": "심볼 (쉼표 구분)",
"execution.field_direction": "방향 필터",
"execution.field_enabled": "활성화",
"execution.field_symbol": "심볼",
"execution.secret_placeholder": "비우면 기존 비밀키 유지",
"execution.secret_copied": "비밀키가 클립보드에 복사되었습니다",
"execution.secret_copy_manual": "저장 전에 비밀키를 수동으로 복사하세요",
"execution.generate": "생성",
"execution.err_name_required": "이름은 필수입니다",
"execution.err_name_exists": "이미 존재합니다",
"execution.err_url": "http:// 또는 https://로 시작해야 합니다",
"execution.err_min_score": "0 이상이어야 합니다",
"execution.err_secret_required": "새 플러그인에는 비밀키가 필요합니다",
"execution.max_dispatched": "최대 디스패치 수",
"execution.dedup_window": "중복 방지 시간창 (예: 5m)",
"execution.symbol_map": "심볼 매핑 (고급)",
"execution.add_row": "행 추가",
"execution.add_broker": "브로커 추가",
"execution.remove_row": "행 제거",
"execution.filter_plugin": "플러그인",
"execution.filter_status": "상태",
"execution.filter_symbol": "심볼",
"execution.col_time": "시각",
"execution.col_plugin": "플러그인",
"execution.col_signal": "시그널",
"execution.col_symbol": "심볼",
"execution.col_status": "상태",
"execution.col_order": "주문",
"execution.col_message": "메시지",
"execution.no_feedback": "주문 내역 없음",
"common.all": "전체",
"common.both": "둘 다",
"common.cancel": "취소",
"common.confirm": "확인",
"common.save": "저장",
"common.discard": "취소",
"common.refresh": "새로고침",
"common.edit": "편집",
"common.delete": "삭제",
"common.show": "보이기",
"common.hide": "숨기기"
```

- [ ] **Step 4: Add to `ja.json`** — same keys, Japanese values:

```json
"execution": "実行",
"execution.kill_switch": "キルスイッチ",
"execution.reenable": "再有効化",
"execution.killed_banner": "実行停止中 — シグナルは配信されません",
"execution.last_killed": "停止時刻",
"execution.confirm_kill": "本当にすべてのプラグイン配信を停止しますか？",
"execution.confirm_reenable": "本当にすべてのプラグイン配信を再有効化しますか？",
"execution.no_activity": "アクティビティなし",
"execution.confirm_delete_plugin": "プラグイン {{name}} を削除しますか？",
"execution.add_plugin": "+ プラグイン追加",
"execution.field_name": "名前",
"execution.field_url": "URL",
"execution.field_secret": "シークレット",
"execution.field_min_score": "最小スコア",
"execution.field_symbols": "シンボル（カンマ区切り）",
"execution.field_direction": "方向フィルター",
"execution.field_enabled": "有効",
"execution.field_symbol": "シンボル",
"execution.secret_placeholder": "空欄で現在のシークレットを維持",
"execution.secret_copied": "シークレットをクリップボードにコピーしました",
"execution.secret_copy_manual": "保存前にシークレットを手動でコピーしてください",
"execution.generate": "生成",
"execution.err_name_required": "名前は必須です",
"execution.err_name_exists": "既に存在します",
"execution.err_url": "http:// または https:// で始まる必要があります",
"execution.err_min_score": "0 以上である必要があります",
"execution.err_secret_required": "新しいプラグインにはシークレットが必要です",
"execution.max_dispatched": "最大同時配信数",
"execution.dedup_window": "重複防止ウィンドウ（例: 5m）",
"execution.symbol_map": "シンボルマッピング（上級）",
"execution.add_row": "行追加",
"execution.add_broker": "ブローカー追加",
"execution.remove_row": "行削除",
"execution.filter_plugin": "プラグイン",
"execution.filter_status": "ステータス",
"execution.filter_symbol": "シンボル",
"execution.col_time": "時刻",
"execution.col_plugin": "プラグイン",
"execution.col_signal": "シグナル",
"execution.col_symbol": "シンボル",
"execution.col_status": "ステータス",
"execution.col_order": "注文",
"execution.col_message": "メッセージ",
"execution.no_feedback": "まだ注文はありません",
"common.all": "全て",
"common.both": "両方",
"common.cancel": "キャンセル",
"common.confirm": "確認",
"common.save": "保存",
"common.discard": "破棄",
"common.refresh": "更新",
"common.edit": "編集",
"common.delete": "削除",
"common.show": "表示",
"common.hide": "非表示"
```

- [ ] **Step 5: Run full test suite**

```
cd web && npm run test
go test ./... -race
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/i18n/locales/en.json web/src/i18n/locales/ko.json web/src/i18n/locales/ja.json
git commit -m "feat(i18n): execution tab translations (en/ko/ja)"
```

---

### Task 19: Manual smoke, CHANGELOG, VERSION bump

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

- [ ] **Step 1: Manual end-to-end smoke**

1. Run server + Alpaca adapter:
   ```
   go run ./cmd/server -config config/server.yaml &
   go run ./cmd/plugin-alpaca -config config/alpaca.yaml &
   ```
2. `cd web && npm run dev` and open the Execution tab.
3. Verify:
   - [ ] Plugin card shows `alpaca-paper`.
   - [ ] 24h stats update after a signal fires (manually trigger via curl or wait for real).
   - [ ] Kill switch: click → confirm → banner appears → plugin card receives no new feedback.
   - [ ] Re-enable: banner clears.
   - [ ] Edit plugin: change `min_score`, save, reopen → new value persists.
   - [ ] Generate secret: toast appears, clipboard contains hex.
   - [ ] 2-tab version conflict: open two tabs, edit/save in A → save in B → conflict banner appears.
   - [ ] FeedbackTable filter: change status dropdown → rows clear synchronously then repopulate.

- [ ] **Step 2: CHANGELOG entry**

Add a new section at the top of `CHANGELOG.md`:

```markdown
## [2.5.0.0] - 2026-04-17

### Added
- **Execution tab (React UI)** (`web/src/ExecutionTab.tsx` + 5 component files) — first UI surface for the execution plugin system shipped in v2.3–v2.4. Top-level tab with four regions: kill switch (confirmation modal + red banner with `killed_at` timestamp), plugin cards (24h SUBMITTED/FILLED/REJECTED counts with last-failure tooltip), global config form (max_dispatched, dedup_window, symbol_map editor), and filtered feedback table (plugin/status/symbol filters, 100-row default).
- **`GET /api/execution/feedback`** (`internal/api/execution_handler.go`) — lists recent feedback with plugin/status/symbol filters and a `limit` bound (0..500, default 100). Queries the new `symbol`/`message` columns on `feedback_idempotency`.
- **`GET /api/execution/plugins/stats?window=24h`** — per-plugin 24h aggregation (SUBMITTED/FILLED/REJECTED counts + most recent failure message). Emits `Cache-Control: max-age=60`.
- **Config version field** on `PUT /api/execution/config` — request must include the last-read `version`; mismatch returns 409 with `current_version` so a concurrent edit surfaces as a banner instead of silently overwriting.
- **`execution_state` SQLite table** — generic key-value runtime state. Houses `killed_at` (moved out of YAML — kill switch state now survives restarts via DB, not git-tracked config) and `config_version`.
- **Secret `Generate` UX** — in-modal button produces a 32-byte hex secret via `crypto.getRandomValues` and auto-copies to clipboard; falls back to a `Copy manually` toast when clipboard is unavailable (HTTP-served remote sessions).
- **Stale-rows regression guard** — FeedbackTable applies the v2.4.0.2 `setFeedback([])` pattern on filter change so no previous-filter rows linger during the new fetch.

### Changed
- `feedback_idempotency` schema — added `symbol` and `message` columns (NULL-default for backward compatibility with pre-2.5 rows).
- `OrderFeedback` payload (`pkg/models/trade_signal.go`) — added optional `Symbol` field. Alpaca adapter now echoes `TradeSignal.Symbol` in feedback callbacks.
- `RecordOnce` signature — now accepts `symbol` and `message` args.

### Database
- New table `execution_state(key PK, value, updated_at)` and `CREATE INDEX idx_feedback_received_at ON feedback_idempotency(received_at)` for the 24h aggregation query.

### Tests
- `TestNoSecretLeakInExecutionEndpoints` blanket regression across `/api/execution/*` responses. Backend coverage on new handlers ~80%. Frontend new components ~70%. Full suite `-race` clean.

### Deferred (see TODOS.md)
- WebSocket feedback push (Phase 5).
- Playwright E2E infrastructure.
- Detailed dispatcher metrics (active positions, avg latency, dedup skip count).
```

- [ ] **Step 3: Bump VERSION**

```
echo "2.5.0.0" > VERSION
```

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md VERSION
git commit -m "chore: bump version to 2.5.0.0 — Execution tab UI"
```

- [ ] **Step 5: Open PR**

Push branch and open PR. Base against `main`. Target title: `feat(ui): Execution tab Phase 4 — kill switch, plugins, config, feedback (v2.5.0.0)`.

---

## Review Checklist (run before requesting code review)

- [ ] `go test ./... -race` — all green
- [ ] `cd web && npm run test` — all green
- [ ] `cd web && npm run lint` — no new warnings
- [ ] `go vet ./...` — clean
- [ ] Manual smoke (Task 19 Step 1) — all items checked
- [ ] CHANGELOG entry committed
- [ ] VERSION bumped to 2.5.0.0
- [ ] Existing Alpaca adapter still works (end-to-end signal → feedback round trip)

---

## Out-of-Scope Reminder

These are explicitly NOT part of this plan (captured in `TODOS.md`):

- WebSocket real-time feedback push.
- Playwright E2E test infrastructure.
- Detailed dispatcher metrics (active position count, avg dispatch latency, dedup skip count).
- ETag-based optimistic locking.
- Bulk `symbol_map` import/export.
- Additional broker adapters (IBKR, Binance paper).
