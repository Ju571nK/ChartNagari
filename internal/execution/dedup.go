package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"modernc.org/sqlite"
)

// DedupStore is a tiny wrapper over *sql.DB that implements the
// single-statement INSERT OR IGNORE pattern required by Codex #2.
//
// Dedup key = symbol+":"+rule+":"+direction.
// Dedup bucket = floor(unix_seconds / windowSec) — collapses all timestamps
// inside one window to a single integer so the UNIQUE(key, bucket) index is
// the source of truth for "already dispatched".
//
// This intentionally replaces any SELECT-then-INSERT two-step pattern: the
// INSERT atomically succeeds (new dispatch) or fails with 0 rows affected
// (duplicate). There is no TOCTOU window.
type DedupStore struct {
	db        *sql.DB
	windowSec int
}

// NewDedupStore constructs a store around the given database handle.
// windowSec must be > 0 (caller should use ExecutionConfig.DedupWindow()).
func NewDedupStore(db *sql.DB, windowSec int) *DedupStore {
	if windowSec <= 0 {
		windowSec = 300
	}
	return &DedupStore{db: db, windowSec: windowSec}
}

// ErrBusy is surfaced when SQLite returns SQLITE_BUSY. Codex #6: callers MUST
// treat this as fail-closed (skip dispatch) — missing a dispatch is
// preferable to double-ordering.
var ErrBusy = errors.New("execution_dedup: database busy")

// BucketKey builds the dedup composite key and bucket.
func (s *DedupStore) BucketKey(symbol, rule, direction string, at time.Time) (key string, bucket int64) {
	key = strings.ToUpper(strings.TrimSpace(symbol)) + ":" +
		strings.TrimSpace(rule) + ":" +
		strings.ToUpper(strings.TrimSpace(direction))
	bucket = at.Unix() / int64(s.windowSec)
	return key, bucket
}

// ReserveDispatch attempts to claim a dedup slot for (symbol, rule, direction)
// at the given wall-clock time. Returns (true, nil) on fresh insert — caller
// proceeds to dispatch. Returns (false, nil) when the slot is already taken
// (duplicate within the window). Returns (false, ErrBusy) on SQLITE_BUSY so
// the caller can fail-closed.
func (s *DedupStore) ReserveDispatch(ctx context.Context, symbol, rule, direction string, at time.Time) (bool, error) {
	key, bucket := s.BucketKey(symbol, rule, direction, at)
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO execution_dedup(key, bucket, dispatched_at) VALUES(?, ?, ?)`,
		key, bucket, at.Unix(),
	)
	if err != nil {
		if isBusy(err) {
			return false, ErrBusy
		}
		return false, fmt.Errorf("execution_dedup insert: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("execution_dedup rows affected: %w", err)
	}
	return n == 1, nil
}

// Cleanup deletes dedup rows older than cutoff. Space reclamation only —
// correctness is preserved by the UNIQUE index on (key, bucket), so a racing
// cleanup can never open a dispatch window (Codex #2, P2 rewrite).
func (s *DedupStore) Cleanup(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM execution_dedup WHERE dispatched_at < ?`, cutoff.Unix(),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// isBusy detects SQLITE_BUSY from modernc.org/sqlite, which does not expose
// the classic mattn-style sqlite3.ErrBusy sentinel. We match by error code.
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	var sErr *sqlite.Error
	if errors.As(err, &sErr) {
		// SQLITE_BUSY = 5; primary result codes are the low byte.
		code := sErr.Code()
		if code == 5 || code&0xff == 5 {
			return true
		}
	}
	msg := err.Error()
	if strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked") {
		return true
	}
	return false
}

// FeedbackIdempotency tracks inbound feedback UNIQUE (plugin_id, signal_id,
// order_id, status) — Codex #4. RecordOnce returns (true, nil) if the row was
// newly inserted and (false, nil) on duplicate (handler should respond 409).
type FeedbackIdempotency struct {
	db *sql.DB
}

// NewFeedbackIdempotency constructs the helper.
func NewFeedbackIdempotency(db *sql.DB) *FeedbackIdempotency {
	return &FeedbackIdempotency{db: db}
}

// RecordOnce attempts to mark (pluginID, signalID, orderID, status) as
// processed. Returns true if this is the first time we've seen the combo.
func (f *FeedbackIdempotency) RecordOnce(ctx context.Context, pluginID, signalID, orderID, status string, at time.Time) (bool, error) {
	res, err := f.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO feedback_idempotency(plugin_id, signal_id, order_id, status, received_at) VALUES(?, ?, ?, ?, ?)`,
		pluginID, signalID, orderID, status, at.Unix(),
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
