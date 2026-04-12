package execution

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB opens a fresh in-memory SQLite with only the tables the execution
// package needs. Kept isolated from internal/storage to avoid import cycles and
// so tests run fast without disk I/O.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
	CREATE TABLE execution_dedup (
		key           TEXT    NOT NULL,
		bucket        INTEGER NOT NULL,
		dispatched_at INTEGER NOT NULL,
		UNIQUE(key, bucket)
	);
	CREATE INDEX idx_execution_dedup_cleanup
		ON execution_dedup(dispatched_at);

	CREATE TABLE feedback_idempotency (
		plugin_id   TEXT    NOT NULL,
		signal_id   TEXT    NOT NULL,
		order_id    TEXT    NOT NULL DEFAULT '',
		status      TEXT    NOT NULL,
		received_at INTEGER NOT NULL,
		UNIQUE(plugin_id, signal_id, order_id, status)
	);
	CREATE INDEX idx_feedback_idempotency_lookup
		ON feedback_idempotency(plugin_id, signal_id);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema create: %v", err)
	}
	return db
}
