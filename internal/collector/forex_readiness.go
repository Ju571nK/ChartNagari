package collector

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/pkg/models"
)

type forexReadiness struct {
	mu     sync.RWMutex
	issues map[string]string
}

func (r *forexReadiness) set(issues map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.issues = issues
}

func (r *forexReadiness) Ready(symbol string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.issues == nil {
		return false
	}
	for key := range r.issues {
		if strings.HasPrefix(key, symbol+"/") {
			return false
		}
	}
	return true
}

func (r *forexReadiness) Issues() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.issues == nil {
		return nil
	}
	out := make(map[string]string, len(r.issues))
	for k, v := range r.issues {
		out[k] = v
	}
	return out
}

func seriesFresh(bars []models.OHLCV, tf string, now time.Time) bool {
	if len(bars) == 0 {
		return false
	}
	latest := bars[0].OpenTime
	for _, bar := range bars[1:] {
		if bar.OpenTime.After(latest) {
			latest = bar.OpenTime
		}
	}
	maxAge := map[string]time.Duration{"1H": 4 * time.Hour, "4H": 12 * time.Hour, "1D": 96 * time.Hour, "1W": 12 * 24 * time.Hour}[tf]
	if maxAge == 0 {
		return false
	}
	if latest.After(now) {
		return false
	}
	if tf == "1H" || tf == "4H" {
		expected, ok := latestExpectedFXOpen(tf, now)
		return ok && !latest.Before(expected)
	}
	// A Friday close remains the last expected candle until the next FX
	// session. Count only time in which another candle could have formed;
	// IsForexOpen also excludes the NY Christmas and New Year closures.
	remaining := maxAge
	for cursor := latest; cursor.Before(now); {
		next := cursor.Truncate(time.Hour).Add(time.Hour)
		if next.After(now) {
			next = now
		}
		if market.IsForexOpen(cursor.Add(next.Sub(cursor) / 2)) {
			remaining -= next.Sub(cursor)
			if remaining < 0 {
				return false
			}
		}
		cursor = next
	}
	return true
}

// latestExpectedFXOpen finds the most recent completed intraday slot in which
// FX traded. A Friday candle is expected through the weekend, but becomes
// stale as soon as the first Sunday candle completes.
func latestExpectedFXOpen(tf string, now time.Time) (time.Time, bool) {
	duration := tfDuration[tf]
	if duration != time.Hour && duration != 4*time.Hour {
		return time.Time{}, false
	}
	candidate := now.Truncate(time.Hour)
	if tf == "4H" {
		candidate = NY4HBucket(now)
	}
	for i := 0; i < 14*24; i++ {
		if !candidate.Add(duration).After(now) && fxIntervalTraded(candidate, duration) {
			return candidate, true
		}
		if tf == "4H" {
			candidate = NY4HBucket(candidate.Add(-time.Nanosecond))
		} else {
			candidate = candidate.Add(-time.Hour)
		}
	}
	return time.Time{}, false
}

func fxIntervalTraded(start time.Time, duration time.Duration) bool {
	for step := time.Duration(0); step < duration; step += time.Hour {
		if !market.IsForexOpen(start.Add(step + time.Hour/2)) {
			return false
		}
	}
	return true
}

// completeFXBars is the final guard before persistence. Provider completion
// flags and Yahoo's timestamps are useful, but neither can authorize a
// future, malformed, or still-open candle to enter the shared OHLCV table.
func completeFXBars(bars []models.OHLCV, tf string, now time.Time) []models.OHLCV {
	duration := tfDuration[tf]
	if duration == 0 {
		return nil
	}
	out := make([]models.OHLCV, 0, len(bars))
	for _, bar := range bars {
		if bar.OpenTime.After(now) || bar.OpenTime.Add(duration).After(now) ||
			(tf == "1H" || tf == "4H") && !fxIntervalTraded(bar.OpenTime, duration) ||
			bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 ||
			bar.Volume < 0 ||
			math.IsNaN(bar.Open) || math.IsNaN(bar.High) || math.IsNaN(bar.Low) || math.IsNaN(bar.Close) ||
			math.IsNaN(bar.Volume) ||
			math.IsInf(bar.Open, 0) || math.IsInf(bar.High, 0) || math.IsInf(bar.Low, 0) || math.IsInf(bar.Close, 0) ||
			math.IsInf(bar.Volume, 0) ||
			bar.High < bar.Open || bar.High < bar.Close || bar.Low > bar.Open || bar.Low > bar.Close {
			continue
		}
		out = append(out, bar)
	}
	return out
}

func issuesError(issues map[string]string) error {
	if len(issues) == 0 {
		return nil
	}
	keys := make([]string, 0, len(issues))
	for key := range issues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+": "+issues[key])
	}
	return fmt.Errorf("FX series unavailable: %s", strings.Join(parts, "; "))
}
