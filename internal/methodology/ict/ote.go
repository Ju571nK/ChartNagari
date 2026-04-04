package ict

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTOTERule detects ICT Optimal Trade Entry setups.
//
// Bullish OTE: After a swing-low → swing-high impulse, price pulls back into the
// 0.618–0.786 Fibonacci retracement zone. If the current bar's close lands inside
// that zone, a LONG signal fires. A close below the 0.786 level invalidates.
//
// Bearish OTE: After a swing-high → swing-low impulse, price pulls back up into
// the 0.618–0.786 retracement zone. If the current bar's close lands inside that
// zone, a SHORT signal fires. A close above the 0.786 level invalidates.
//
// The rule scans the last 20 bars for swing-high/low pairs to identify impulses.
type ICTOTERule struct{}

func (r *ICTOTERule) Name() string                 { return "ict_ote" }
func (r *ICTOTERule) RequiredIndicators() []string { return []string{"SWING_HIGH", "SWING_LOW"} }

// findSwingLow returns the index and price of the lowest low in bars[start:end].
func findSwingLow(bars []models.OHLCV, start, end int) (int, float64) {
	bestIdx := start
	bestVal := bars[start].Low
	for i := start + 1; i < end; i++ {
		if bars[i].Low < bestVal {
			bestVal = bars[i].Low
			bestIdx = i
		}
	}
	return bestIdx, bestVal
}

// findSwingHigh returns the index and price of the highest high in bars[start:end].
func findSwingHigh(bars []models.OHLCV, start, end int) (int, float64) {
	bestIdx := start
	bestVal := bars[start].High
	for i := start + 1; i < end; i++ {
		if bars[i].High > bestVal {
			bestVal = bars[i].High
			bestIdx = i
		}
	}
	return bestIdx, bestVal
}

func (r *ICTOTERule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 5 {
			continue
		}

		n := len(bars)
		curr := bars[n-1]

		// Scan window: last 20 bars (or all if fewer)
		scanStart := n - 20
		if scanStart < 0 {
			scanStart = 0
		}
		scanEnd := n - 1 // exclude current bar from impulse search

		if scanEnd-scanStart < 3 {
			continue
		}

		// Find swing low and swing high in the scan window
		swLowIdx, swLow := findSwingLow(bars, scanStart, scanEnd)
		swHighIdx, swHigh := findSwingHigh(bars, scanStart, scanEnd)

		if math.Abs(swHigh-swLow) < 1e-10 {
			continue
		}

		// Bullish OTE: swing low comes BEFORE swing high (impulse up), then pullback
		if swLowIdx < swHighIdx {
			// Fibonacci retracement levels (measured from swing high back toward swing low)
			fib618 := swHigh - 0.618*(swHigh-swLow)
			fib786 := swHigh - 0.786*(swHigh-swLow)
			// OTE zone: fib786 (lower) to fib618 (upper)
			zoneLow := fib786
			zoneHigh := fib618

			if curr.Close >= zoneLow && curr.Close <= zoneHigh {
				rawScore := 1.0
				weighted := rawScore * tfW[tf]
				if weighted > bestWeighted {
					bestWeighted = weighted
					bestSig = &models.Signal{
						Symbol:    ctx.Symbol,
						Timeframe: tf,
						Rule:      r.Name(),
						Direction: "LONG",
						Score:     rawScore,
						ZoneLow:   zoneLow,
						ZoneHigh:  zoneHigh,
						Message:   fmt.Sprintf("[%s] ICT OTE Bullish — 피보 0.618~0.786 리트레이스먼트 구간 진입 → LONG (%.4f–%.4f)", tf, zoneLow, zoneHigh),
						CreatedAt: time.Now(),
					}
				}
			}
		}

		// Bearish OTE: swing high comes BEFORE swing low (impulse down), then pullback
		if swHighIdx < swLowIdx {
			// Fibonacci retracement levels (measured from swing low back toward swing high)
			fib618 := swLow + 0.618*(swHigh-swLow)
			fib786 := swLow + 0.786*(swHigh-swLow)
			// OTE zone: fib618 (lower) to fib786 (upper)
			zoneLow := fib618
			zoneHigh := fib786

			if curr.Close >= zoneLow && curr.Close <= zoneHigh {
				rawScore := 1.0
				weighted := rawScore * tfW[tf]
				if weighted > bestWeighted {
					bestWeighted = weighted
					bestSig = &models.Signal{
						Symbol:    ctx.Symbol,
						Timeframe: tf,
						Rule:      r.Name(),
						Direction: "SHORT",
						Score:     rawScore,
						ZoneLow:   zoneLow,
						ZoneHigh:  zoneHigh,
						Message:   fmt.Sprintf("[%s] ICT OTE Bearish — 피보 0.618~0.786 리트레이스먼트 구간 진입 → SHORT (%.4f–%.4f)", tf, zoneLow, zoneHigh),
						CreatedAt: time.Now(),
					}
				}
			}
		}
	}

	return bestSig, nil
}
