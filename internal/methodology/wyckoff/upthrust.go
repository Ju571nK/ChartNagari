package wyckoff

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// WyckoffUpthrustRule detects the Wyckoff Upthrust pattern.
//
// An Upthrust occurs when price temporarily breaks ABOVE resistance (SWING_HIGH)
// with a wick, then closes BACK BELOW it — the opposite of a Spring.
//
// Detection (per TF):
//   - Any bar in bars[len-5..len-2] has High > SWING_HIGH (the upthrust pierce)
//   - Current bar Close < SWING_HIGH (reversal)
//   - Volume ≥ 1.5× VOLUME_MA_20 on the reversal
//
// Score = 1.0 for confirmed upthrust.
// Requires SWING_HIGH, VOLUME_MA_20, and ≥ 5 bars.
type WyckoffUpthrustRule struct{}

func (r *WyckoffUpthrustRule) Name() string                 { return "wyckoff_upthrust" }
func (r *WyckoffUpthrustRule) RequiredIndicators() []string { return nil }

func (r *WyckoffUpthrustRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	const lookback = 5
	const volMultiplier = 1.5

	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	bestScore := 0.0
	bestTF := ""
	bestSwingHigh := 0.0

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < lookback {
			continue
		}

		swingHighKey := tf + ":SWING_HIGH"
		volMAKey := tf + ":VOLUME_MA_20"

		swingHigh, hasSwingHigh := ctx.Indicators[swingHighKey]
		volMA, hasVolMA := ctx.Indicators[volMAKey]
		if !hasSwingHigh || !hasVolMA {
			continue
		}

		curr := bars[len(bars)-1]
		prev := bars[len(bars)-lookback : len(bars)-1]

		// Check if any prior bar pierced above swing high
		pierced := false
		for _, b := range prev {
			if b.High > swingHigh {
				pierced = true
				break
			}
		}
		if !pierced {
			continue
		}

		// Current bar must close below swing high
		if curr.Close >= swingHigh {
			continue
		}

		// Volume confirmation
		if curr.Volume < volMultiplier*volMA {
			continue
		}

		rawScore := 1.0
		weighted := rawScore * tfW[tf]
		if weighted > bestScore {
			bestScore = weighted
			bestTF = tf
			bestSwingHigh = swingHigh
		}
	}

	if bestTF == "" {
		return nil, nil
	}

	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: bestTF,
		Rule:      r.Name(),
		Direction: "SHORT",
		Score:     1.0,
		Message:   fmt.Sprintf("[%s] Wyckoff 업스러스트 패턴 → SHORT (스윙고점: %.4f)", bestTF, bestSwingHigh),
		CreatedAt: time.Now(),
	}, nil
}
