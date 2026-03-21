package candlestick

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// BullishEngulfingRule detects bullish engulfing patterns → LONG.
type BullishEngulfingRule struct{}

func (r *BullishEngulfingRule) Name() string                 { return "bullish_engulfing" }
func (r *BullishEngulfingRule) RequiredIndicators() []string { return nil }

func (r *BullishEngulfingRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 2 {
			continue
		}

		prev := bars[len(bars)-2]
		curr := bars[len(bars)-1]

		if !isBearish(prev) || !isBullish(curr) {
			continue
		}
		if curr.Open > prev.Close || curr.Close < prev.Open {
			continue
		}
		if candleBody(curr) <= candleBody(prev) {
			continue
		}

		rawScore := 3.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "LONG",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 상승 장악형 — 강세 반전 LONG", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}

// BearishEngulfingRule detects bearish engulfing patterns → SHORT.
type BearishEngulfingRule struct{}

func (r *BearishEngulfingRule) Name() string                 { return "bearish_engulfing" }
func (r *BearishEngulfingRule) RequiredIndicators() []string { return nil }

func (r *BearishEngulfingRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 2 {
			continue
		}

		prev := bars[len(bars)-2]
		curr := bars[len(bars)-1]

		if !isBullish(prev) || !isBearish(curr) {
			continue
		}
		if curr.Open < prev.Close || curr.Close > prev.Open {
			continue
		}
		if candleBody(curr) <= candleBody(prev) {
			continue
		}

		rawScore := 3.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "SHORT",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 하락 장악형 — 약세 반전 SHORT", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
