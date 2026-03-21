package candlestick

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// DojiRule detects doji candles (body < 5% of range) and interprets
// direction based on the preceding trend.
type DojiRule struct{}

func (r *DojiRule) Name() string                 { return "doji" }
func (r *DojiRule) RequiredIndicators() []string { return nil }

func (r *DojiRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 5 {
			continue
		}

		last := bars[len(bars)-1]

		if candleRange(last) == 0 {
			continue
		}
		if bodyRatio(last) >= 0.05 {
			continue
		}

		rawScore := 1.5
		var dir, msg string

		switch {
		case isDowntrend(bars, 5):
			dir = "LONG"
			msg = fmt.Sprintf("[%s] 도지 — 하락 추세 후 반전 가능", tf)
		case isUptrend(bars, 5):
			dir = "SHORT"
			msg = fmt.Sprintf("[%s] 도지 — 상승 추세 후 반전 가능", tf)
		default:
			dir = "NEUTRAL"
			msg = fmt.Sprintf("[%s] 도지 — 추세 전환 신호", tf)
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
				Message:   msg,
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
