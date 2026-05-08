package notifier

import (
	"fmt"
	"sync"
	"time"
)

// DailyLimit caps how many alerts a single symbol can produce per UTC day.
// A limit of 0 means "no limit" (unlimited). The bucket resets when the UTC
// day changes — implemented lazily on each Allow call.
type DailyLimit struct {
	mu     sync.Mutex
	counts map[string]int // key: "{symbol}|{utc_date}"
	now    func() time.Time
}

// NewDailyLimit creates an empty limiter with time.Now as the clock.
func NewDailyLimit() *DailyLimit {
	return &DailyLimit{
		counts: make(map[string]int),
		now:    time.Now,
	}
}

// Allow returns true and increments the day's count when the symbol has
// fewer than `limit` alerts on the current UTC day. Returns true when
// `limit <= 0` (no limit configured).
func (l *DailyLimit) Allow(symbol string, limit int) bool {
	if limit <= 0 {
		return true
	}
	day := l.now().UTC().Format("2006-01-02")
	key := fmt.Sprintf("%s|%s", symbol, day)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[key] >= limit {
		return false
	}
	l.counts[key]++
	return true
}

// setClock overrides the time source. Used only in tests.
func (l *DailyLimit) setClock(f func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = f
}
