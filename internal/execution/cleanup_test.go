package execution

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestDedupCleaner_TTLFloor — cleaner TTL is max(2*windowSec, 2m).
func TestDedupCleaner_TTLFloor(t *testing.T) {
	db := newTestDB(t)
	// small window → ttl should floor to 2 minutes
	store := NewDedupStore(db, 10)
	c := NewDedupCleaner(store, zerolog.Nop())
	if c.ttl < 2*time.Minute {
		t.Errorf("ttl %v should be at least 2m", c.ttl)
	}
}

// TestDedupCleaner_TTLFromWindow — larger windowSec doubles the ttl.
func TestDedupCleaner_TTLFromWindow(t *testing.T) {
	db := newTestDB(t)
	store := NewDedupStore(db, 600) // 10 minutes
	c := NewDedupCleaner(store, zerolog.Nop())
	if c.ttl != 20*time.Minute {
		t.Errorf("ttl = %v, want 20m (2 * 600s)", c.ttl)
	}
}

// TestDedupCleaner_RunStopsOnCancel — cancelling the context stops the loop.
func TestDedupCleaner_RunStopsOnCancel(t *testing.T) {
	db := newTestDB(t)
	store := NewDedupStore(db, 10)
	c := NewDedupCleaner(store, zerolog.Nop())
	// Speed up the ticker so we don't waste the 1-minute period.
	c.period = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	// Let one tick elapse, then cancel.
	time.Sleep(120 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("cleaner did not stop within 1s of context cancel")
	}
}
