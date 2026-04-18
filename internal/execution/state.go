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
