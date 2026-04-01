// Package sequence tracks multi-signal patterns across time for a single symbol.
//
// Trading signals are more reliable when they occur in specific sequences.
// For example, a liquidity sweep followed by displacement (strong impulse)
// followed by a retest of the sweep level is a high-conviction entry.
//
// The Tracker maintains a rolling window of recent signals per symbol and
// checks for known sequences. When a sequence completes, it returns a
// score bonus that the pipeline can apply to the triggering signal.
package sequence

import (
	"sync"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// maxHistory is the maximum number of signals kept per symbol.
const maxHistory = 20

// maxAge is the maximum age of a signal to be considered part of a sequence.
const maxAge = 48 * time.Hour

// SequenceMatch describes a completed signal sequence.
type SequenceMatch struct {
	Name       string  // e.g. "sweep_displacement_retest"
	Bonus      float64 // score multiplier bonus (e.g. 0.3 = +30%)
	Signals    []models.Signal // the signals that form the sequence
}

// pattern defines a signal sequence pattern to detect.
type pattern struct {
	name  string
	bonus float64
	// match returns a SequenceMatch if the history (oldest-first) contains this pattern.
	// The latest signal is always history[len-1].
	match func(history []models.Signal) *SequenceMatch
}

// Tracker maintains per-symbol signal history and checks for sequence patterns.
type Tracker struct {
	mu       sync.Mutex
	history  map[string][]models.Signal // key: symbol
	patterns []pattern
}

// New creates a Tracker with the built-in sequence patterns.
func New() *Tracker {
	t := &Tracker{
		history: make(map[string][]models.Signal),
	}
	t.patterns = []pattern{
		{
			name:  "sweep_displacement",
			bonus: 0.2,
			match: matchSweepDisplacement,
		},
		{
			name:  "fvg_retest",
			bonus: 0.15,
			match: matchFVGRetest,
		},
		{
			name:  "ob_retest",
			bonus: 0.15,
			match: matchOBRetest,
		},
	}
	return t
}

// Record adds a signal to the symbol's history and returns any completed sequences.
func (t *Tracker) Record(sig models.Signal) []SequenceMatch {
	t.mu.Lock()
	defer t.mu.Unlock()

	sym := sig.Symbol
	t.history[sym] = append(t.history[sym], sig)

	// Trim old entries
	t.trim(sym)

	// Check all patterns
	var matches []SequenceMatch
	for _, p := range t.patterns {
		if m := p.match(t.history[sym]); m != nil {
			m.Name = p.name
			m.Bonus = p.bonus
			matches = append(matches, *m)
		}
	}
	return matches
}

// trim removes old and excess signals from a symbol's history.
func (t *Tracker) trim(sym string) {
	h := t.history[sym]
	cutoff := time.Now().Add(-maxAge)

	// Remove signals older than maxAge
	start := 0
	for start < len(h) && h[start].CreatedAt.Before(cutoff) {
		start++
	}
	if start > 0 {
		h = h[start:]
	}

	// Cap at maxHistory
	if len(h) > maxHistory {
		h = h[len(h)-maxHistory:]
	}
	t.history[sym] = h
}

// matchSweepDisplacement detects: liquidity sweep → strong impulse in sweep direction.
// The sweep grabs liquidity, then price moves decisively in the expected direction.
// Pattern: ict_liquidity_sweep (LONG/SHORT) → any signal with same direction within 5 signals.
func matchSweepDisplacement(history []models.Signal) *SequenceMatch {
	n := len(history)
	if n < 2 {
		return nil
	}

	latest := history[n-1]
	// Look backwards for a sweep that matches the latest signal's direction
	lookback := 5
	if lookback > n-1 {
		lookback = n - 1
	}

	for i := n - 2; i >= n-1-lookback; i-- {
		prev := history[i]
		if prev.Rule == "ict_liquidity_sweep" && prev.Direction == latest.Direction {
			// Sweep followed by same-direction signal = displacement confirmed
			return &SequenceMatch{
				Signals: []models.Signal{prev, latest},
			}
		}
	}
	return nil
}

// matchFVGRetest detects: FVG formation → later signal at FVG level (same direction).
// When price returns to fill a Fair Value Gap and a new signal fires there.
func matchFVGRetest(history []models.Signal) *SequenceMatch {
	n := len(history)
	if n < 2 {
		return nil
	}

	latest := history[n-1]
	lookback := 10
	if lookback > n-1 {
		lookback = n - 1
	}

	for i := n - 2; i >= n-1-lookback; i-- {
		prev := history[i]
		if prev.Rule == "ict_fair_value_gap" && prev.Direction == latest.Direction {
			return &SequenceMatch{
				Signals: []models.Signal{prev, latest},
			}
		}
	}
	return nil
}

// matchOBRetest detects: order block formation → later signal at OB level (same direction).
func matchOBRetest(history []models.Signal) *SequenceMatch {
	n := len(history)
	if n < 2 {
		return nil
	}

	latest := history[n-1]
	lookback := 10
	if lookback > n-1 {
		lookback = n - 1
	}

	for i := n - 2; i >= n-1-lookback; i-- {
		prev := history[i]
		if prev.Rule == "ict_order_block" && prev.Direction == latest.Direction {
			return &SequenceMatch{
				Signals: []models.Signal{prev, latest},
			}
		}
	}
	return nil
}
