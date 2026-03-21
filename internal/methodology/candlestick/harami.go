package candlestick

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// BullishHaramiRule detects bullish harami patterns → LONG.
type BullishHaramiRule struct{}

func (r *BullishHaramiRule) Name() string                 { return "bullish_harami" }
func (r *BullishHaramiRule) RequiredIndicators() []string { return nil }

func (r *BullishHaramiRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
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

		if !isBearish(prev) || bodyRatio(prev) <= 0.50 {
			continue
		}
		if !isBullish(curr) {
			continue
		}
		if curr.Open <= math.Min(prev.Open, prev.Close) || curr.Close >= math.Max(prev.Open, prev.Close) {
			continue
		}
		if candleBody(curr) >= candleBody(prev)*0.5 {
			continue
		}

		rawScore := 2.5
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "LONG",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 강세 하라미 — 약세 소진 LONG", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}

// BearishHaramiRule detects bearish harami patterns → SHORT.
type BearishHaramiRule struct{}

func (r *BearishHaramiRule) Name() string                 { return "bearish_harami" }
func (r *BearishHaramiRule) RequiredIndicators() []string { return nil }

func (r *BearishHaramiRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
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

		if !isBullish(prev) || bodyRatio(prev) <= 0.50 {
			continue
		}
		if !isBearish(curr) {
			continue
		}
		if curr.Open >= math.Max(prev.Open, prev.Close) || curr.Close <= math.Min(prev.Open, prev.Close) {
			continue
		}
		if candleBody(curr) >= candleBody(prev)*0.5 {
			continue
		}

		rawScore := 2.5
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "SHORT",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 약세 하라미 — 강세 소진 SHORT", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
