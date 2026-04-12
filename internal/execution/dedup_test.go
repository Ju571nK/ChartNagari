package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestIsBusy_Strings detects SQLITE_BUSY via message match for the fallback
// path (when the typed *sqlite.Error is unavailable).
func TestIsBusy_Strings(t *testing.T) {
	if !isBusy(errors.New("SQLITE_BUSY: database is locked")) {
		t.Error("SQLITE_BUSY string should be detected")
	}
	if !isBusy(errors.New("database is locked")) {
		t.Error("database is locked should be detected")
	}
	if isBusy(errors.New("some other error")) {
		t.Error("unrelated error must not be busy")
	}
	if isBusy(nil) {
		t.Error("nil is not busy")
	}
}

// TestReserveDispatch_FirstInsertWins — first call for a (symbol, rule,
// direction) in a window returns (true, nil). Subsequent calls return
// (false, nil) without error. This is the UNIQUE(key, bucket) + INSERT OR
// IGNORE contract Codex #2 requires.
func TestReserveDispatch_FirstInsertWins(t *testing.T) {
	db := newTestDB(t)
	store := NewDedupStore(db, 300)
	now := time.Unix(1_700_000_000, 0)

	ok1, err := store.ReserveDispatch(context.Background(), "BTCUSDT", "wyckoff_spring", "LONG", now)
	if err != nil {
		t.Fatalf("first reserve err: %v", err)
	}
	if !ok1 {
		t.Fatal("first reserve should return fresh=true")
	}

	ok2, err := store.ReserveDispatch(context.Background(), "BTCUSDT", "wyckoff_spring", "LONG", now)
	if err != nil {
		t.Fatalf("second reserve err: %v", err)
	}
	if ok2 {
		t.Fatal("duplicate reserve should return fresh=false")
	}
}

// TestReserveDispatch_DifferentDirections — LONG and SHORT are independent
// dedup keys so a reversal is never swallowed (T3/T4).
func TestReserveDispatch_DifferentDirections(t *testing.T) {
	db := newTestDB(t)
	store := NewDedupStore(db, 300)
	now := time.Unix(1_700_000_000, 0)

	ok, err := store.ReserveDispatch(context.Background(), "BTCUSDT", "rsi_div", "LONG", now)
	if err != nil || !ok {
		t.Fatalf("LONG reserve: ok=%v err=%v", ok, err)
	}
	ok, err = store.ReserveDispatch(context.Background(), "BTCUSDT", "rsi_div", "SHORT", now)
	if err != nil || !ok {
		t.Fatalf("SHORT reserve must succeed independently: ok=%v err=%v", ok, err)
	}
}

// TestReserveDispatch_DifferentRules — distinct rules on the same symbol/dir
// are independent (T3/T4).
func TestReserveDispatch_DifferentRules(t *testing.T) {
	db := newTestDB(t)
	store := NewDedupStore(db, 300)
	now := time.Unix(1_700_000_000, 0)

	ok, err := store.ReserveDispatch(context.Background(), "AAPL", "ict_ob", "LONG", now)
	if err != nil || !ok {
		t.Fatalf("rule-A reserve: ok=%v err=%v", ok, err)
	}
	ok, err = store.ReserveDispatch(context.Background(), "AAPL", "smc_bos", "LONG", now)
	if err != nil || !ok {
		t.Fatalf("rule-B reserve must succeed: ok=%v err=%v", ok, err)
	}
}

// TestReserveDispatch_CrossWindow — after the bucket boundary advances, the
// same key can be reserved again (fresh dispatch in a new window).
func TestReserveDispatch_CrossWindow(t *testing.T) {
	db := newTestDB(t)
	store := NewDedupStore(db, 300) // 5-minute window

	now := time.Unix(1_700_000_000, 0)
	ok, err := store.ReserveDispatch(context.Background(), "ETHUSDT", "ict_fvg", "LONG", now)
	if err != nil || !ok {
		t.Fatalf("initial: ok=%v err=%v", ok, err)
	}

	// Advance past one full window + a few seconds so the bucket integer changes.
	later := now.Add(301 * time.Second)
	ok, err = store.ReserveDispatch(context.Background(), "ETHUSDT", "ict_fvg", "LONG", later)
	if err != nil || !ok {
		t.Fatalf("next window should be fresh: ok=%v err=%v", ok, err)
	}
}

// TestCleanup_RemovesOldRows — cleanup deletes rows whose dispatched_at is
// strictly less than the cutoff. Space reclamation only; correctness is
// preserved by the UNIQUE index.
func TestCleanup_RemovesOldRows(t *testing.T) {
	db := newTestDB(t)
	store := NewDedupStore(db, 300)

	t1 := time.Unix(1_700_000_000, 0)
	t2 := time.Unix(1_700_000_600, 0) // +10 min
	_, _ = store.ReserveDispatch(context.Background(), "A", "r", "LONG", t1)
	_, _ = store.ReserveDispatch(context.Background(), "B", "r", "LONG", t2)

	n, err := store.Cleanup(context.Background(), time.Unix(1_700_000_300, 0))
	if err != nil {
		t.Fatalf("Cleanup err: %v", err)
	}
	if n != 1 {
		t.Errorf("Cleanup removed %d rows, want 1 (t1 only)", n)
	}
}

// TestBucketKey_Normalization — dedup key normalizes symbol and direction to
// upper case and trims whitespace so "btcusdt" and "BTCUSDT " are treated as
// the same key.
func TestBucketKey_Normalization(t *testing.T) {
	store := NewDedupStore(nil, 300)
	now := time.Unix(1_700_000_000, 0)
	k1, _ := store.BucketKey(" btcusdt ", "r", "long", now)
	k2, _ := store.BucketKey("BTCUSDT", "r", "LONG", now)
	if k1 != k2 {
		t.Errorf("expected keys equal after normalization, got %q vs %q", k1, k2)
	}
}

// TestFeedbackIdempotency_RecordOnce — UNIQUE(plugin_id, signal_id, order_id,
// status) means the first insert returns (true, nil) and any duplicate returns
// (false, nil). Different statuses for the same signal are distinct rows so
// ACK → FILLED is not swallowed (Codex #4).
func TestFeedbackIdempotency_RecordOnce(t *testing.T) {
	db := newTestDB(t)
	idem := NewFeedbackIdempotency(db)
	now := time.Unix(1_700_000_000, 0)

	ok, err := idem.RecordOnce(context.Background(), "p1", "sig-1", "ord-1", "FILLED", now)
	if err != nil || !ok {
		t.Fatalf("first insert: ok=%v err=%v", ok, err)
	}
	// Exact replay → duplicate.
	ok, err = idem.RecordOnce(context.Background(), "p1", "sig-1", "ord-1", "FILLED", now)
	if err != nil {
		t.Fatalf("replay err: %v", err)
	}
	if ok {
		t.Fatal("replay must return fresh=false")
	}
	// Different status for same signal → separate row.
	ok, err = idem.RecordOnce(context.Background(), "p1", "sig-1", "ord-1", "CANCELLED", now)
	if err != nil || !ok {
		t.Fatalf("different status: ok=%v err=%v", ok, err)
	}
}
