package alpaca

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *IdempotencyStore {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenIdempotencyStore(filepath.Join(dir, "idem.db"))
	if err != nil {
		t.Fatalf("OpenIdempotencyStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestIdempotency_ReserveOnce(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()
	now := time.Now()
	if err := store.Reserve(ctx, "s1", now); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	err := store.Reserve(ctx, "s1", now)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second reserve: got %v want ErrDuplicate", err)
	}
}

func TestIdempotency_Release(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()
	if err := store.Reserve(ctx, "s2", time.Now()); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.Release(ctx, "s2"); err != nil {
		t.Fatalf("release: %v", err)
	}
	// After release, a fresh reserve should succeed.
	if err := store.Reserve(ctx, "s2", time.Now()); err != nil {
		t.Fatalf("re-reserve after release: %v", err)
	}
}

func TestIdempotency_MarkSubmitted(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()
	if err := store.Reserve(ctx, "s3", time.Now()); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.MarkSubmitted(ctx, "s3", "ord-123", "accepted"); err != nil {
		t.Fatalf("mark submitted: %v", err)
	}
	// Release should NOT delete a submitted row (only status = RESERVED).
	if err := store.Release(ctx, "s3"); err != nil {
		t.Fatalf("release after submit: %v", err)
	}
	err := store.Reserve(ctx, "s3", time.Now())
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("reserve after submit+release: got %v want ErrDuplicate", err)
	}
}

func TestIdempotency_EmptySignalID(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	if err := store.Reserve(context.Background(), "", time.Now()); err == nil {
		t.Fatal("expected error for empty signal_id")
	}
}
