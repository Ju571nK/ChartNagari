package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTLiquiditySweepRule detects liquidity sweeps — wicks through swing points with quick reversal.
//
// Bullish sweep (bear trap): current bar's Low < SWING_LOW AND current Close > SWING_LOW -> LONG
//   (price dipped below swing low to grab liquidity, then reversed)
// Bearish sweep (bull trap): current bar's High > SWING_HIGH AND current Close < SWING_HIGH -> SHORT
//
// Score = 1.0 for confirmed sweep.
// Requires SWING_HIGH and SWING_LOW in indicators, and >= 1 bar.
type ICTLiquiditySweepRule struct{}

func (r *ICTLiquiditySweepRule) Name() string                 { return "ict_liquidity_sweep" }
func (r *ICTLiquiditySweepRule) RequiredIndicators() []string { return nil }

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

		current := bars[len(bars)-1]

		var dir string
		var level float64

		// Bullish sweep: low dipped below SWING_LOW, close recovered above it
		if hasLow && current.Low < swingLow && current.Close > swingLow {
			dir = "LONG"
			level = swingLow
		}

		// Bearish sweep: high pierced SWING_HIGH, close fell back below it
		// (bearish takes priority if both trigger simultaneously — rare edge case)
		if hasHigh && current.High > swingHigh && current.Close < swingHigh {
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
				Message:   fmt.Sprintf("[%s] ICT 유동성 스윕 → %s (스윕 레벨: %.4f)", tf, dir, level),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
