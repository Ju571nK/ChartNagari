package ict

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTLiquiditySweepRule detects liquidity sweeps — wicks through swing points with quick reversal.
//
// Bullish sweep (bear trap): bar's Low < SWING_LOW AND Close > SWING_LOW -> LONG
//   (price dipped below swing low to grab liquidity, then reversed)
// Bearish sweep (bull trap): bar's High > SWING_HIGH AND Close < SWING_HIGH -> SHORT
//
// The rule scans the last 6 bars for sweep candidates and applies a breakout filter:
// if 3+ confirmation bars after the sweep show sustained price beyond the level with
// strong bodies in the breakout direction, the sweep is reclassified as a breakout
// and suppressed (returns nil).
//
// Quality Score (0.1–1.0) is computed from three factors:
//   1. Volume ratio: sweep candle volume / VOLUME_MA_20 (higher = institutional activity)
//   2. Wick ratio:   wick beyond level / total candle range (longer wick = stronger rejection)
//   3. Reversal strength: close distance from level / total range (farther = stronger reversal)
//
// Requires SWING_HIGH and SWING_LOW in indicators, and >= 1 bar.
type ICTLiquiditySweepRule struct{}

func (r *ICTLiquiditySweepRule) Name() string                 { return "ict_liquidity_sweep" }
func (r *ICTLiquiditySweepRule) RequiredIndicators() []string { return nil }

// sweepQuality computes a 0.1–1.0 quality score for a liquidity sweep candle.
//
//	volRatio: sweep candle volume / VOLUME_MA_20 (0 if unavailable → neutral 0.5)
//	wickBeyond: how far price went past the swing level (absolute)
//	reversalDist: how far close is from the level on the reversal side (absolute)
//	candleRange: high - low of the sweep candle
func sweepQuality(volRatio, wickBeyond, reversalDist, candleRange float64) float64 {
	if candleRange <= 0 {
		return 0.1
	}

	// Factor 1: volume (weight 0.4)
	// volRatio=1.0 → 0.3, volRatio=2.0 → 0.6, volRatio=3.0+ → 1.0
	var volScore float64
	if volRatio <= 0 {
		volScore = 0.5 // no volume data → neutral
	} else {
		volScore = math.Min(1.0, (volRatio-0.5)/2.5)
		if volScore < 0 {
			volScore = 0
		}
	}

	// Factor 2: wick ratio (weight 0.3)
	// How much of the candle is wick beyond the level. Higher = stronger rejection.
	wickRatio := wickBeyond / candleRange
	wickScore := math.Min(1.0, wickRatio*2.0) // 50% wick → 1.0

	// Factor 3: reversal strength (weight 0.3)
	// How far did close recover past the level. Higher = stronger reversal.
	revScore := math.Min(1.0, reversalDist/candleRange*2.0)

	raw := volScore*0.4 + wickScore*0.3 + revScore*0.3

	// Clamp to [0.1, 1.0]
	if raw < 0.1 {
		return 0.1
	}
	if raw > 1.0 {
		return 1.0
	}
	return math.Round(raw*100) / 100
}

// isBreakout checks if a sweep at sweepIdx is actually a breakout by examining
// the confirmation bars that follow. Returns true if 3+ bars after the sweep
// have their close beyond the level with strong bodies (body > 50% of range)
// in the breakout direction.
//
//   - For LONG sweeps (bullish sweep = bear trap): breakout means price stays BELOW
//     the level, indicating a genuine downward breakout rather than a liquidity grab.
//   - For SHORT sweeps (bearish sweep = bull trap): breakout means price stays ABOVE
//     the level, indicating a genuine upward breakout.
func isBreakout(bars []models.OHLCV, sweepIdx int, level float64, dir string) bool {
	n := len(bars)
	// Need at least 3 confirmation bars after the sweep
	if sweepIdx+3 >= n {
		return false
	}

	confirmCount := 0
	checkEnd := sweepIdx + 5 // check up to 5 bars
	if checkEnd >= n {
		checkEnd = n - 1
	}

	for j := sweepIdx + 1; j <= checkEnd; j++ {
		b := bars[j]
		bodySize := math.Abs(b.Close - b.Open)
		candleRange := b.High - b.Low
		if candleRange <= 0 {
			continue
		}

		strongBody := bodySize/candleRange >= 0.5

		if dir == "LONG" {
			// Bullish sweep reclassified as breakout if bars stay below swing low
			if b.Close < level && strongBody {
				confirmCount++
			}
		} else {
			// Bearish sweep reclassified as breakout if bars stay above swing high
			if b.Close > level && strongBody {
				confirmCount++
			}
		}
	}

	return confirmCount >= 3
}

// detectSweep checks whether a single bar qualifies as a sweep candidate.
// Returns direction ("LONG" or "SHORT"), the level, wickBeyond, and reversalDist.
// Returns dir="" if no sweep detected.
func detectSweep(bar models.OHLCV, swingHigh, swingLow float64, hasHigh, hasLow bool) (dir string, level, wickBeyond, reversalDist float64) {
	// Bullish sweep: low dipped below SWING_LOW, close recovered above it
	if hasLow && bar.Low < swingLow && bar.Close > swingLow {
		dir = "LONG"
		level = swingLow
		wickBeyond = swingLow - bar.Low
		reversalDist = bar.Close - swingLow
	}

	// Bearish sweep: high pierced SWING_HIGH, close fell back below it
	// (bearish takes priority if both trigger simultaneously)
	if hasHigh && bar.High > swingHigh && bar.Close < swingHigh {
		dir = "SHORT"
		level = swingHigh
		wickBeyond = bar.High - swingHigh
		reversalDist = swingHigh - bar.Close
	}

	return dir, level, wickBeyond, reversalDist
}

func (r *ICTLiquiditySweepRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 1 {
			continue
		}

		swingHigh, hasHigh := ctx.Indicators[tf+":SWING_HIGH"]
		swingLow, hasLow := ctx.Indicators[tf+":SWING_LOW"]

		if !hasHigh && !hasLow {
			continue
		}

		n := len(bars)

		// Scan from the most recent bar backwards (up to 6 bars back) for sweep candidates.
		// For each candidate, check if it was confirmed as a breakout by subsequent bars.
		scanStart := n - 6
		if scanStart < 0 {
			scanStart = 0
		}

		for idx := n - 1; idx >= scanStart; idx-- {
			bar := bars[idx]
			candleRange := bar.High - bar.Low

			dir, level, wickBeyond, reversalDist := detectSweep(bar, swingHigh, swingLow, hasHigh, hasLow)
			if dir == "" {
				continue
			}

			// Check if this sweep is actually a breakout (only if we have enough confirmation bars)
			if isBreakout(bars, idx, level, dir) {
				continue // suppress — this is a breakout, not a sweep
			}

			// Volume ratio: sweep candle volume / 20-period MA volume
			volMA, hasVol := ctx.Indicators[tf+":VOLUME_MA_20"]
			var volRatio float64
			if hasVol && volMA > 0 {
				volRatio = bar.Volume / volMA
			}

			rawScore := sweepQuality(volRatio, wickBeyond, reversalDist, candleRange)
			weighted := rawScore * tfW[tf]
			if weighted > bestWeighted {
				bestWeighted = weighted
				bestSig = &models.Signal{
					Symbol:    ctx.Symbol,
					Timeframe: tf,
					Rule:      r.Name(),
					Direction: dir,
					Score:     rawScore,
					Message:   fmt.Sprintf("[%s] ICT 유동성 스윕 → %s (레벨: %.4f, 품질: %.0f%%)", tf, dir, level, rawScore*100),
					CreatedAt: time.Now(),
				}
			}

			// Found a valid (non-breakout) sweep in this timeframe — use the most recent one
			break
		}
	}

	return bestSig, nil
}
