package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTFairValueGapRule detects Fair Value Gaps (3-candle imbalance pattern).
//
// Bullish FVG: bars[i].high < bars[i+2].low (gap between candle i high and candle i+2 low)
//   -> Price entering the gap -> LONG
// Bearish FVG: bars[i].low > bars[i+2].high (gap between candle i low and candle i+2 high)
//   -> Price entering the gap -> SHORT
//
// Scan last 15 bars for FVG patterns. Check if current bar's close is within any gap.
// Score = 1.0 for confirmed FVG entry.
// Requires >= 4 bars.
type ICTFairValueGapRule struct{}

func (r *ICTFairValueGapRule) Name() string                 { return "ict_fair_value_gap" }
func (r *ICTFairValueGapRule) RequiredIndicators() []string { return nil }

func (r *ICTFairValueGapRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 4 {
			continue
		}

		n := len(bars)
		current := bars[n-1]

		// Scan from bars[len-15] to bars[len-3], checking pattern with bars[i], bars[i+1], bars[i+2]
		// i+2 must be <= n-2 (excluding current bar index n-1)
		start := n - 15
		if start < 0 {
			start = 0
		}

		var gapLow, gapHigh float64
		var dir string

		// Scan from most recent to oldest to find latest valid FVG
		for i := n - 2 - 2; i >= start; i-- {
			b0 := bars[i]
			b2 := bars[i+2]

			// Bullish FVG: b0.high < b2.low, gap = [b0.high, b2.low]
			gl := b0.High
			gh := b2.Low
			if gh > gl && current.Close >= gl && current.Close <= gh {
				gapLow = gl
				gapHigh = gh
				dir = "LONG"
				break
			}

			// Bearish FVG: b0.low > b2.high, gap = [b2.high, b0.low]
			gl2 := b2.High
			gh2 := b0.Low
			if gh2 > gl2 && current.Close >= gl2 && current.Close <= gh2 {
				gapLow = gl2
				gapHigh = gh2
				dir = "SHORT"
				break
			}
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
				Message:   fmt.Sprintf("[%s] ICT FVG 진입 → %s (Gap: %.4f-%.4f)", tf, dir, gapLow, gapHigh),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
