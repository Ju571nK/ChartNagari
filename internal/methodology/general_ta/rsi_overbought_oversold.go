package general_ta

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// RSIOverboughtOversoldRule signals overbought (SHORT) or oversold (LONG) based on RSI(14).
// Checks all TFs, returns the signal with highest (rawScore × tfWeight).
//
// Thresholds: RSI >= 70 → SHORT, RSI <= 30 → LONG
// Score: (distance from threshold) / 30, clamped to [0.1, 1.0]
type RSIOverboughtOversoldRule struct{}

func (r *RSIOverboughtOversoldRule) Name() string                 { return "rsi_overbought_oversold" }
func (r *RSIOverboughtOversoldRule) RequiredIndicators() []string { return nil }

func (r *RSIOverboughtOversoldRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		rsi, ok := ctx.Indicators[tf+":RSI_14"]
		if !ok {
			continue
		}

		var dir string
		var rawScore float64

		if rsi >= 70 {
			dir = "SHORT"
			rawScore = (rsi - 70) / 30
		} else if rsi <= 30 {
			dir = "LONG"
			rawScore = (30 - rsi) / 30
		} else {
			continue
		}

		// Clamp to [0.1, 1.0]
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
				Message:   fmt.Sprintf("[%s] RSI(14)=%.1f → %s", tf, rsi, dir),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
