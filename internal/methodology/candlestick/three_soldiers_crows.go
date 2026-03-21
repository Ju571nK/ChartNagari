package candlestick

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ThreeWhiteSoldiersRule detects three consecutive bullish candles with
// higher closes and each opening within the previous body → LONG.
type ThreeWhiteSoldiersRule struct{}

func (r *ThreeWhiteSoldiersRule) Name() string                 { return "three_white_soldiers" }
func (r *ThreeWhiteSoldiersRule) RequiredIndicators() []string { return nil }

func (r *ThreeWhiteSoldiersRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
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
		b1 := bars[n-3]
		b2 := bars[n-2]
		b3 := bars[n-1]

		// All three must be bullish with substantial bodies
		if !isBullish(b1) || !isBullish(b2) || !isBullish(b3) {
			continue
		}
		if bodyRatio(b1) <= 0.50 || bodyRatio(b2) <= 0.50 || bodyRatio(b3) <= 0.50 {
			continue
		}

		// Each closes higher
		if b2.Close <= b1.Close || b3.Close <= b2.Close {
			continue
		}

		// Each opens within the previous candle's body
		if b2.Open <= math.Min(b1.Open, b1.Close) || b2.Open >= b1.Close {
			continue
		}
		if b3.Open <= math.Min(b2.Open, b2.Close) || b3.Open >= b2.Close {
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
				Message:   fmt.Sprintf("[%s] 세 백색 병사 — 강한 상승 모멘텀 LONG", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}

// ThreeBlackCrowsRule detects three consecutive bearish candles with
// lower closes and each opening within the previous body → SHORT.
type ThreeBlackCrowsRule struct{}

func (r *ThreeBlackCrowsRule) Name() string                 { return "three_black_crows" }
func (r *ThreeBlackCrowsRule) RequiredIndicators() []string { return nil }

func (r *ThreeBlackCrowsRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
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
		b1 := bars[n-3]
		b2 := bars[n-2]
		b3 := bars[n-1]

		// All three must be bearish with substantial bodies
		if !isBearish(b1) || !isBearish(b2) || !isBearish(b3) {
			continue
		}
		if bodyRatio(b1) <= 0.50 || bodyRatio(b2) <= 0.50 || bodyRatio(b3) <= 0.50 {
			continue
		}

		// Each closes lower
		if b2.Close >= b1.Close || b3.Close >= b2.Close {
			continue
		}

		// Each opens within the previous candle's body (for bearish: Open is top of body)
		// b2.Open should be within b1's body: between b1.Close (bottom) and b1.Open (top)
		if b2.Open >= math.Max(b1.Open, b1.Close) || b2.Open <= b1.Close {
			continue
		}
		if b3.Open >= math.Max(b2.Open, b2.Close) || b3.Open <= b2.Close {
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
				Message:   fmt.Sprintf("[%s] 세 흑색 까마귀 — 강한 하락 모멘텀 SHORT", tf),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
