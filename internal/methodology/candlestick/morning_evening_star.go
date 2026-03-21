package candlestick

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// MorningStarRule detects three-bar morning star patterns → LONG.
type MorningStarRule struct{}

func (r *MorningStarRule) Name() string                 { return "morning_star" }
func (r *MorningStarRule) RequiredIndicators() []string { return nil }

func (r *MorningStarRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 3 {
			continue
		}

		n := len(bars)
		first := bars[n-3]
		star := bars[n-2]
		last := bars[n-1]

		// first: large bearish candle
		if !isBearish(first) || bodyRatio(first) <= 0.50 {
			continue
		}
		// star: small body
		if bodyRatio(star) >= 0.25 {
			continue
		}
		// last: bullish candle closing above first's midpoint
		if !isBullish(last) || bodyRatio(last) <= 0.30 {
			continue
		}
		if last.Close <= midpoint(first) {
			continue
		}

		rawScore := 4.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "LONG",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 모닝스타 — 강력한 강세 반전 LONG", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}

// EveningStarRule detects three-bar evening star patterns → SHORT.
type EveningStarRule struct{}

func (r *EveningStarRule) Name() string                 { return "evening_star" }
func (r *EveningStarRule) RequiredIndicators() []string { return nil }

func (r *EveningStarRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 3 {
			continue
		}

		n := len(bars)
		first := bars[n-3]
		star := bars[n-2]
		last := bars[n-1]

		// first: large bullish candle
		if !isBullish(first) || bodyRatio(first) <= 0.50 {
			continue
		}
		// star: small body
		if bodyRatio(star) >= 0.25 {
			continue
		}
		// last: bearish candle closing below first's midpoint
		if !isBearish(last) || bodyRatio(last) <= 0.30 {
			continue
		}
		if last.Close >= midpoint(first) {
			continue
		}

		rawScore := 4.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "SHORT",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 이브닝스타 — 강력한 약세 반전 SHORT", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
