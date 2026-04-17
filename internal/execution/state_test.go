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
