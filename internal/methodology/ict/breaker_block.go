package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTBreakerBlockRule detects breaker blocks — failed order blocks that become opposing S/R.
//
// Bullish breaker: price previously broke below SWING_LOW (bars closed below),
//   then reversed back above SWING_LOW -> the zone below SWING_LOW is now a bullish breaker.
//   If current close crossed back above SWING_LOW from below -> LONG
//
// Bearish breaker: price previously broke above SWING_HIGH (bars closed above),
//   then reversed back below SWING_HIGH -> SHORT
//
// Detection: scan last 10 bars (excluding current).
//   Bullish: any of bars[len-6..len-2] had Close < SWING_LOW, AND current Close > SWING_LOW -> LONG
//   Bearish: any of bars[len-6..len-2] had Close > SWING_HIGH, AND current Close < SWING_HIGH -> SHORT
//
// Score = 1.0. Requires SWING_HIGH, SWING_LOW, and >= 6 bars.
type ICTBreakerBlockRule struct{}

func (r *ICTBreakerBlockRule) Name() string                 { return "ict_breaker_block" }
func (r *ICTBreakerBlockRule) RequiredIndicators() []string { return nil }

func (r *ICTBreakerBlockRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 6 {
			continue
		}

		swingHigh, hasHigh := ctx.Indicators[tf+":SWING_HIGH"]
		swingLow, hasLow := ctx.Indicators[tf+":SWING_LOW"]

		if !hasHigh && !hasLow {
			continue
		}

		n := len(bars)
		current := bars[n-1]

		// Scan bars[len-6..len-2] (5 bars before current)
		scanStart := n - 6
		if scanStart < 0 {
			scanStart = 0
		}
		scanEnd := n - 2 // inclusive

		var dir string
		var level float64

		var prevBelowSwingLow, prevAboveSwingHigh bool
		for i := scanStart; i <= scanEnd; i++ {
			if hasLow && bars[i].Close < swingLow {
				prevBelowSwingLow = true
			}
			if hasHigh && bars[i].Close > swingHigh {
				prevAboveSwingHigh = true
			}
		}

		// Bullish breaker: previous bars were below SWING_LOW, current is back above
		if hasLow && prevBelowSwingLow && current.Close > swingLow {
			dir = "LONG"
			level = swingLow
		}

		// Bearish breaker: previous bars were above SWING_HIGH, current is back below
		// (bearish takes priority if both trigger simultaneously)
		if hasHigh && prevAboveSwingHigh && current.Close < swingHigh {
			dir = "SHORT"
			level = swingHigh
		}

		if dir == "" {
			continue
		}

		rawScore := 1.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: dir,
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] ICT 브레이커 블록 → %s (레벨: %.4f)", tf, dir, level),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
