// Package notifier handles signal filtering and dispatch to notification backends.
package notifier

import (
	"fmt"
	"sync"
	"time"
)

// Cooldown prevents duplicate alerts by tracking the last send time per signal key.
// The key is "{symbol}|{rule}" — direction is intentionally excluded so LONG and SHORT
// for the same rule share the cooldown window.
type Cooldown struct {
	mu       sync.Mutex
	last     map[string]time.Time
	duration time.Duration
	now      func() time.Time // injectable for testing
}

// NewCooldown creates a Cooldown with the given duration and time.Now as clock.
func NewCooldown(duration time.Duration) *Cooldown {
	return &Cooldown{
		last:     make(map[string]time.Time),
		duration: duration,
		now:      time.Now,
	}
}

// Allow returns true and records the current time when the cooldown has expired
// (or when this key has never been seen before). Returns false if the last alert
// for this key was sent within the cooldown window.
func (c *Cooldown) Allow(symbol, rule string) bool {
	key := fmt.Sprintf("%s|%s", symbol, rule)
	c.mu.Lock()
	defer c.mu.Unlock()

	last, exists := c.last[key]
	if !exists || c.now().Sub(last) >= c.duration {
		c.last[key] = c.now()
		return true
	}
	return false
}

// setClock overrides the time source. Used only in tests.
func (c *Cooldown) setClock(f func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = f
}
