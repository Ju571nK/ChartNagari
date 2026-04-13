package alpaca

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
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

// TestIdempotency_ConcurrentReserveRace is the load-bearing safety test: N
// goroutines hit Reserve with the same signal_id at the same time. Exactly one
// must win; the rest must see ErrDuplicate. Anything else means a double-submit
// would be possible under contention — which is the whole bug idempotency is
// meant to prevent.
func TestIdempotency_ConcurrentReserveRace(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()
	const N = 32
	var wg sync.WaitGroup
	var ok, dup atomic.Int32
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := store.Reserve(ctx, "race-sig", time.Now())
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrDuplicate):
				dup.Add(1)
			default:
				t.Errorf("unexpected err: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 1 || dup.Load() != N-1 {
		t.Fatalf("race: ok=%d dup=%d want 1/%d", ok.Load(), dup.Load(), N-1)
	}
}

// TestIdempotency_CloseThenReserve verifies that Close releases the DB handle
// and that subsequent operations surface an error rather than panicking or
// silently succeeding.
func TestIdempotency_CloseThenReserve(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := OpenIdempotencyStore(filepath.Join(dir, "idem.db"))
	if err != nil {
		t.Fatalf("OpenIdempotencyStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Reserve(context.Background(), "s-after-close", time.Now()); err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}
