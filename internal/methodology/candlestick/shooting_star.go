package candlestick

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ShootingStarRule detects shooting star patterns after an uptrend → SHORT.
type ShootingStarRule struct{}

func (r *ShootingStarRule) Name() string                 { return "shooting_star" }
func (r *ShootingStarRule) RequiredIndicators() []string { return nil }

func (r *ShootingStarRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 6 {
			continue
		}

		if !isUptrend(bars, 5) {
			continue
		}

		last := bars[len(bars)-1]
		body := candleBody(last)
		if body == 0 {
			continue
		}

		br := bodyRatio(last)
		if br < 0.05 || br > 0.40 {
			continue
		}
		if upperShadow(last) <= 2.0*body {
			continue
		}
		if lowerShadow(last) >= body {
			continue
		}

		rawScore := 2.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "SHORT",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 슈팅스타 — 상승 후 약세 반전 SHORT", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}

// InvertedHammerRule detects inverted hammer patterns after a downtrend → LONG.
type InvertedHammerRule struct{}

func (r *InvertedHammerRule) Name() string                 { return "inverted_hammer" }
func (r *InvertedHammerRule) RequiredIndicators() []string { return nil }

func (r *InvertedHammerRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 6 {
			continue
		}

		if !isDowntrend(bars, 5) {
			continue
		}

		last := bars[len(bars)-1]
		body := candleBody(last)
		if body == 0 {
			continue
		}

		br := bodyRatio(last)
		if br < 0.05 || br > 0.40 {
			continue
		}
		if upperShadow(last) <= 2.0*body {
			continue
		}
		if lowerShadow(last) >= body {
			continue
		}

		rawScore := 1.5
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: "LONG",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 역해머 — 하락 후 강세 반전 가능 LONG", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
