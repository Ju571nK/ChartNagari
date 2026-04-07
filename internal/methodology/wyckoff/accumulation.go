package wyckoff

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// WyckoffAccumulationRule detects potential Wyckoff accumulation phase.
//
// Characteristics:
//   - Price is in lower range (Close < EMA_50 — in a potential base after downtrend)
//   - Range is tight over last N bars (high-low range < X% of average price)
//   - Volume is low (current volume < VOLUME_MA_20)
//
// If all conditions met → LONG signal (anticipating markup)
// Score based on range tightness: narrower range → higher score
// Requires ≥ 20 bars and EMA_50, VOLUME_MA_20 in indicators.
type WyckoffAccumulationRule struct{}

func (r *WyckoffAccumulationRule) Name() string                 { return "wyckoff_accumulation" }
func (r *WyckoffAccumulationRule) RequiredIndicators() []string { return nil }

func (r *WyckoffAccumulationRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	const lookback = 20
	const rangePct = 0.08

	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	bestScore := 0.0
	bestTF := ""
	bestMsg := ""

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
		if curr.Close >= ema50 {
			continue
		}
		if curr.Volume >= volMA {
			continue
		}

		rawScore := math.Max(0.1, math.Min(1.0, 1.0-rangeWidth/rangePct))

		// Relative Strength factor: compare symbol return vs benchmark (e.g., SPY).
		// If benchmark return is available in indicators, apply RS bonus/penalty.
		benchReturnKey := tf + ":BENCHMARK_RETURN_20"
		if benchReturn, hasBench := ctx.Indicators[benchReturnKey]; hasBench {
			// Calculate symbol's 20-bar return
			if len(bars) >= lookback {
				symbolReturn := (bars[len(bars)-1].Close - bars[len(bars)-lookback].Close) / bars[len(bars)-lookback].Close
				rs := symbolReturn - benchReturn
				if rs > 0 {
					rawScore += 0.2 // outperforming market → higher confidence
				} else if rs < -0.05 {
					rawScore -= 0.1 // underperforming significantly → lower confidence
				}
				rawScore = math.Max(0.1, math.Min(1.5, rawScore)) // clamp after RS adjustment
			}
		}

		weighted := rawScore * tfW[tf]

		if weighted > bestScore {
			bestScore = weighted
			bestTF = tf
			bestMsg = fmt.Sprintf("[%s] Wyckoff 누적 국면 감지 → LONG (레인지 %.1f%%)", tf, rangeWidth*100)
			_ = bestMsg
		}
	}

	if bestTF == "" {
		return nil, nil
	}

	bars := ctx.Timeframes[bestTF]
	rangeHigh := math.Inf(-1)
	rangeLow := math.Inf(1)
	recent := bars[len(bars)-lookback:]
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

	// Apply RS bonus to the final score (mirrors the loop logic above).
	benchReturnKey := bestTF + ":BENCHMARK_RETURN_20"
	if benchReturn, hasBench := ctx.Indicators[benchReturnKey]; hasBench {
		if len(bars) >= lookback {
			symbolReturn := (bars[len(bars)-1].Close - bars[len(bars)-lookback].Close) / bars[len(bars)-lookback].Close
			rs := symbolReturn - benchReturn
			if rs > 0 {
				rawScore += 0.2
			} else if rs < -0.05 {
				rawScore -= 0.1
			}
			rawScore = math.Max(0.1, math.Min(1.5, rawScore))
		}
	}

	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: bestTF,
		Rule:      r.Name(),
		Direction: "LONG",
		Score:     rawScore,
		Message:   fmt.Sprintf("[%s] Wyckoff 누적 국면 감지 → LONG (레인지 %.1f%%)", bestTF, rangeWidth*100),
		CreatedAt: time.Now(),
	}, nil
}
