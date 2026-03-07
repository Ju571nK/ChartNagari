package wyckoff

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// WyckoffDistributionRule detects potential Wyckoff distribution phase.
//
// Characteristics:
//   - Price is in upper range (Close > EMA_50)
//   - Range is tight over last N bars
//   - Volume is low (current volume < VOLUME_MA_20)
//
// If all conditions met → SHORT signal (anticipating markdown)
// Score based on range tightness.
// Requires ≥ 20 bars and EMA_50, VOLUME_MA_20.
type WyckoffDistributionRule struct{}

func (r *WyckoffDistributionRule) Name() string                 { return "wyckoff_distribution" }
func (r *WyckoffDistributionRule) RequiredIndicators() []string { return nil }

func (r *WyckoffDistributionRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	const lookback = 20
	const rangePct = 0.08

	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	bestScore := 0.0
	bestTF := ""

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < lookback {
			continue
		}

		ema50Key := tf + ":EMA_50"
		volMAKey := tf + ":VOLUME_MA_20"

		ema50, hasEMA := ctx.Indicators[ema50Key]
		volMA, hasVolMA := ctx.Indicators[volMAKey]
		if !hasEMA || !hasVolMA {
			continue
		}

		recent := bars[len(bars)-lookback:]
		rangeHigh := math.Inf(-1)
		rangeLow := math.Inf(1)
		for _, b := range recent {
			if b.High > rangeHigh {
				rangeHigh = b.High
			}
			if b.Low < rangeLow {
				rangeLow = b.Low
			}
		}

		avgPrice := (rangeHigh + rangeLow) / 2
		if avgPrice == 0 {
			continue
		}
		rangeWidth := (rangeHigh - rangeLow) / avgPrice

		curr := bars[len(bars)-1]

		if rangeWidth >= rangePct {
			continue
		}
		if curr.Close <= ema50 {
			continue
		}
		if curr.Volume >= volMA {
			continue
		}

		rawScore := math.Max(0.1, math.Min(1.0, 1.0-rangeWidth/rangePct))
		weighted := rawScore * tfW[tf]

		if weighted > bestScore {
			bestScore = weighted
			bestTF = tf
		}
	}

	if bestTF == "" {
		return nil, nil
	}

	bars := ctx.Timeframes[bestTF]
	recent := bars[len(bars)-lookback:]
	rangeHigh := math.Inf(-1)
	rangeLow := math.Inf(1)
	for _, b := range recent {
		if b.High > rangeHigh {
			rangeHigh = b.High
		}
		if b.Low < rangeLow {
			rangeLow = b.Low
		}
	}
	avgPrice := (rangeHigh + rangeLow) / 2
	rangeWidth := (rangeHigh - rangeLow) / avgPrice
	rawScore := math.Max(0.1, math.Min(1.0, 1.0-rangeWidth/rangePct))

	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: bestTF,
		Rule:      r.Name(),
		Direction: "SHORT",
		Score:     rawScore,
		Message:   fmt.Sprintf("[%s] Wyckoff 분산 국면 감지 → SHORT (레인지 %.1f%%)", bestTF, rangeWidth*100),
		CreatedAt: time.Now(),
	}, nil
}
