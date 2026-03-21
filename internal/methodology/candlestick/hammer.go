package candlestick

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// HammerRule detects hammer patterns after a downtrend → LONG reversal.
type HammerRule struct{}

func (r *HammerRule) Name() string                 { return "hammer" }
func (r *HammerRule) RequiredIndicators() []string { return nil }

func (r *HammerRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
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
		if lowerShadow(last) <= 2.0*body {
			continue
		}
		if upperShadow(last) >= body {
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
				Direction: "LONG",
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] 해머 패턴 — 하락 후 강세 반전 신호 LONG", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}

// HangingManRule detects hanging man patterns after an uptrend → SHORT reversal.
type HangingManRule struct{}

func (r *HangingManRule) Name() string                 { return "hanging_man" }
func (r *HangingManRule) RequiredIndicators() []string { return nil }

func (r *HangingManRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
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
		if lowerShadow(last) <= 2.0*body {
			continue
		}
		if upperShadow(last) >= body {
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
				Message:   fmt.Sprintf("[%s] 행잉맨 패턴 — 상승 후 약세 반전 신호 SHORT", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
