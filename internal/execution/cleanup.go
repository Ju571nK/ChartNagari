package execution

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// DedupCleaner runs a 1-minute ticker that deletes expired dedup rows. Per
// the eng review P2 rewrite: this is **space reclamation only** — dedup
// correctness is guaranteed by the UNIQUE(key, bucket) index, so a slow or
// crashed cleanup cannot create a race window.
type DedupCleaner struct {
	store  *DedupStore
	log    zerolog.Logger
	period time.Duration
	ttl    time.Duration
}

// NewDedupCleaner constructs a cleaner with the default 1-minute period and a
// TTL of 2*windowSec so dedup rows outlive their effective bucket boundary.
func NewDedupCleaner(store *DedupStore, log zerolog.Logger) *DedupCleaner {
	period := 1 * time.Minute
	ttl := time.Duration(store.windowSec*2) * time.Second
	if ttl < 2*time.Minute {
		ttl = 2 * time.Minute
	}
	return &DedupCleaner{store: store, log: log, period: period, ttl: ttl}
}

// Run ticks every period until ctx is cancelled. Errors are logged and
// swallowed so a transient DB issue does not tear down the goroutine.
func (c *DedupCleaner) Run(ctx context.Context) {
	t := time.NewTicker(c.period)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			cutoff := now.Add(-c.ttl)
			n, err := c.store.Cleanup(ctx, cutoff)
			if err != nil {
				c.log.Warn().Err(err).Msg("exec: dedup cleanup failed")
				continue
			}
			if n > 0 {
				c.log.Debug().Int64("rows", n).Time("cutoff", cutoff).
					Msg("exec: dedup cleanup")
			}
		}
	}
}
