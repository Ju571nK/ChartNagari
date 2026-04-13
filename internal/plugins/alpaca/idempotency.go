package alpaca

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // sqlite driver
)

// IdempotencyStore persists one row per (signal_id) so the adapter never
// submits two Alpaca orders for the same TradeSignal. The schema uses a UNIQUE
// index and a single-statement INSERT OR IGNORE — same fail-closed pattern the
// ChartNagari dispatcher uses for its dedup store.
type IdempotencyStore struct {
	db *sql.DB
}

// OpenIdempotencyStore opens (or creates) the SQLite file at path, enables WAL,
// and applies the idempotency schema.
func OpenIdempotencyStore(path string) (*IdempotencyStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open idempotency db: %w", err)
	}
	// Single connection is sufficient — the adapter is low-QPS.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS alpaca_idempotency (
			signal_id   TEXT PRIMARY KEY,
			order_id    TEXT,
			status      TEXT NOT NULL,
			created_at  INTEGER NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}
	return &IdempotencyStore{db: db}, nil
}

// Close releases the underlying DB handle.
func (s *IdempotencyStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ErrDuplicate is returned by Reserve when a signal_id has already been seen.
var ErrDuplicate = errors.New("alpaca: duplicate signal_id")

// Reserve attempts to claim signal_id. Returns nil on fresh reservation; returns
// ErrDuplicate when the signal was already processed.
func (s *IdempotencyStore) Reserve(ctx context.Context, signalID string, now time.Time) error {
	if signalID == "" {
		return errors.New("alpaca: empty signal_id")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO alpaca_idempotency(signal_id, status, created_at) VALUES(?, ?, ?)`,
		signalID, "RESERVED", now.Unix(),
	)
	if err != nil {
		return fmt.Errorf("reserve: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reserve rows: %w", err)
	}
	if n == 0 {
		return ErrDuplicate
	}
	return nil
}

// MarkSubmitted updates the row for signal_id with the Alpaca order id and a
// terminal-ish status. Called after a successful Alpaca submission so we can
// reconstruct adapter state on restart.
func (s *IdempotencyStore) MarkSubmitted(ctx context.Context, signalID, orderID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alpaca_idempotency SET order_id = ?, status = ? WHERE signal_id = ?`,
		orderID, status, signalID,
	)
	if err != nil {
		return fmt.Errorf("mark submitted: %w", err)
	}
	return nil
}

// Release removes the reservation for signal_id. Used when a pre-submission
// step fails (e.g. Alpaca returned 4xx for a mapping error) so retries with a
// fresh signal_id can still work — the failed attempt is not reported as
// "duplicate" later. Idempotent: missing rows are fine.
func (s *IdempotencyStore) Release(ctx context.Context, signalID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM alpaca_idempotency WHERE signal_id = ? AND status = 'RESERVED'`,
		signalID,
	)
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	return nil
}
