package general_ta

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

const (
	emaFast = 9
	emaSlow = 20
)

// EMACrossRule signals EMA(9) golden/death cross over EMA(20).
//
// Golden cross (EMA9 crosses above EMA20) → LONG
// Death cross (EMA9 crosses below EMA20) → SHORT
// Requires at least (slow+2) = 22 bars per TF.
type EMACrossRule struct{}

func (r *EMACrossRule) Name() string                 { return "ema_cross" }
func (r *EMACrossRule) RequiredIndicators() []string { return nil }

func (r *EMACrossRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < emaSlow+2 {
			continue
		}

		closes := make([]float64, len(bars))
		for i, b := range bars {
			closes[i] = b.Close
		}

		fastEMA := rollingEMA(closes, emaFast)
		slowEMA := rollingEMA(closes, emaSlow)

		if fastEMA == nil || slowEMA == nil || len(fastEMA) < 2 || len(slowEMA) < 2 {
			continue
		}

		// Both arrays end at the same bar. Take last 2 elements.
		currFast := fastEMA[len(fastEMA)-1]
		prevFast := fastEMA[len(fastEMA)-2]
		currSlow := slowEMA[len(slowEMA)-1]
		prevSlow := slowEMA[len(slowEMA)-2]

		var dir string
		var crossLabel string

		if prevFast < prevSlow && currFast >= currSlow {
			dir = "LONG"
			crossLabel = "골든크로스"
		} else if prevFast > prevSlow && currFast <= currSlow {
			dir = "SHORT"
			crossLabel = "데드크로스"
		} else {
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
				Message:   fmt.Sprintf("[%s] EMA(9) %s EMA(20) → %s", tf, crossLabel, dir),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
