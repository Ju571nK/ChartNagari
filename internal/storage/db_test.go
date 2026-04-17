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
