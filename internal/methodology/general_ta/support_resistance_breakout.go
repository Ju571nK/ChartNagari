package general_ta

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SupportResistanceBreakoutRule signals price breakouts through swing high/low.
//
// Break above SWING_HIGH → LONG
// Break below SWING_LOW  → SHORT
// Requires SWING_HIGH and SWING_LOW in indicators, and ≥2 bars in TF.
type SupportResistanceBreakoutRule struct{}

func (r *SupportResistanceBreakoutRule) Name() string                 { return "support_resistance_breakout" }
func (r *SupportResistanceBreakoutRule) RequiredIndicators() []string { return nil }

func (r *SupportResistanceBreakoutRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 2 {
			continue
		}

		swingHigh, hasHigh := ctx.Indicators[tf+":SWING_HIGH"]
		swingLow, hasLow := ctx.Indicators[tf+":SWING_LOW"]
		if !hasHigh || !hasLow {
			continue
		}

		prev := bars[len(bars)-2]
		curr := bars[len(bars)-1]

		var dir string
		var level float64
		var levelLabel string

		if prev.Close <= swingHigh && curr.Close > swingHigh {
			dir = "LONG"
			level = swingHigh
			levelLabel = "저항"
		} else if prev.Close >= swingLow && curr.Close < swingLow {
			dir = "SHORT"
			level = swingLow
			levelLabel = "지지"
		} else {
			continue
		}

		breakoutPct := math.Abs(curr.Close-level) / level
		rawScore := breakoutPct * 50
		if rawScore < 0.1 {
			rawScore = 0.1
		} else if rawScore > 1.0 {
			rawScore = 1.0
		}

		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: dir,
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] %s 돌파 → %s (%.4f)", tf, levelLabel, dir, curr.Close),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
