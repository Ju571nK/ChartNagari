package wyckoff

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// WyckoffSpringRule detects the Wyckoff Spring pattern.
//
// A Spring occurs when price temporarily breaks BELOW the support (SWING_LOW)
// with a wick, then closes BACK ABOVE it — often accompanied by high volume.
//
// Detection (per TF):
//   - Look at last 5 bars (including current)
//   - Any bar in bars[len-5..len-2] has Low < SWING_LOW (the spring dip)
//   - Current bar (bars[len-1]) Close > SWING_LOW (recovery)
//   - Volume on recovery ≥ 1.5× VOLUME_MA_20 (institutional buying)
//
// Score = 1.0 for confirmed spring.
// Requires SWING_LOW, VOLUME_MA_20, and ≥ 5 bars.
type WyckoffSpringRule struct{}

func (r *WyckoffSpringRule) Name() string                 { return "wyckoff_spring" }
func (r *WyckoffSpringRule) RequiredIndicators() []string { return nil }

func (r *WyckoffSpringRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	const lookback = 5
	const volMultiplier = 1.5

	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	bestScore := 0.0
	bestTF := ""
	bestSwingLow := 0.0

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < lookback {
			continue
		}

		swingLowKey := tf + ":SWING_LOW"
		volMAKey := tf + ":VOLUME_MA_20"

		swingLow, hasSwingLow := ctx.Indicators[swingLowKey]
		volMA, hasVolMA := ctx.Indicators[volMAKey]
		if !hasSwingLow || !hasVolMA {
			continue
		}

		curr := bars[len(bars)-1]
		prev := bars[len(bars)-lookback : len(bars)-1]

		// Check if any prior bar dipped below swing low
		dipped := false
		for _, b := range prev {
			if b.Low < swingLow {
				dipped = true
				break
			}
		}
		if !dipped {
			continue
		}

		// Current bar must close above swing low
		if curr.Close <= swingLow {
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
			bestSwingLow = swingLow
		}
	}

	if bestTF == "" {
		return nil, nil
	}

	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: bestTF,
		Rule:      r.Name(),
		Direction: "LONG",
		Score:     1.0,
		Message:   fmt.Sprintf("[%s] Wyckoff 스프링 패턴 → LONG (스윙저점: %.4f)", bestTF, bestSwingLow),
		CreatedAt: time.Now(),
	}, nil
}
