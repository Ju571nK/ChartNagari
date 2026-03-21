package candlestick

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// MarubozuRule detects marubozu candles (body > 90% of range) indicating
// strong momentum.
type MarubozuRule struct{}

func (r *MarubozuRule) Name() string                 { return "marubozu" }
func (r *MarubozuRule) RequiredIndicators() []string { return nil }

func (r *MarubozuRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 1 {
			continue
		}

		last := bars[len(bars)-1]

		if candleRange(last) == 0 {
			continue
		}
		if bodyRatio(last) <= 0.90 {
			continue
		}

		rawScore := 2.5
		var dir, label string

		if isBullish(last) {
			dir = "LONG"
			label = "상승"
		} else if isBearish(last) {
			dir = "SHORT"
			label = "하락"
		} else {
			continue
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
				Message:   fmt.Sprintf("[%s] 마루보주 — 강한 %s 모멘텀", tf, label),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
