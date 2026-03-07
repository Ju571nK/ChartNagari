package general_ta

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

const (
	divLookback  = 20
	divRSIPeriod = 14
)

// RSIDivergenceRule detects price/RSI divergence over a lookback window.
//
// Bullish divergence: price makes lower low, RSI makes higher low → LONG
// Bearish divergence: price makes higher high, RSI makes lower high → SHORT
//
// Requires at least (lookback + rsiPeriod) bars per TF.
// Uses rollingRSI internally.
type RSIDivergenceRule struct{}

func (r *RSIDivergenceRule) Name() string                 { return "rsi_divergence" }
func (r *RSIDivergenceRule) RequiredIndicators() []string { return nil }

func (r *RSIDivergenceRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < divLookback+divRSIPeriod {
			continue
		}

		// Use last (lookback + rsiPeriod) bars for RSI warmup
		allBars := bars[len(bars)-(divLookback+divRSIPeriod):]

		closes := make([]float64, len(allBars))
		for i, b := range allBars {
			closes[i] = b.Close
		}

		rsiAll := rollingRSI(closes, divRSIPeriod)
		if rsiAll == nil || len(rsiAll) < divLookback {
			continue
		}

		// Trim RSI to last lookback bars
		rsiVals := rsiAll[len(rsiAll)-divLookback:]

		// Recent closes for swing detection
		recent := bars[len(bars)-divLookback:]
		recentCloses := make([]float64, len(recent))
		for i, b := range recent {
			recentCloses[i] = b.Close
		}

		// Check bullish divergence first
		lo1, lo2 := swingLowPair(recentCloses)
		if lo1 >= 0 && lo2 >= 0 {
			priceLowerLow := recentCloses[lo2] < recentCloses[lo1]
			rsiHigherLow := rsiVals[lo2] > rsiVals[lo1]
			if priceLowerLow && rsiHigherLow {
				priceDiff := math.Abs(recentCloses[lo1]-recentCloses[lo2]) / recentCloses[lo1]
				rsiDiff := math.Abs(rsiVals[lo2]-rsiVals[lo1]) / 100.0
				rawScore := (priceDiff + rsiDiff) / 2
				if rawScore < 0.1 {
					rawScore = 0.1
				} else if rawScore > 1.0 {
					rawScore = 1.0
				}

				weighted := rawScore * tfW[tf]
				if weighted > bestWeighted {
					bestWeighted = weighted
					bestSig = &models.Signal{
						Symbol:    ctx.Symbol,
						Timeframe: tf,
						Rule:      r.Name(),
						Direction: "LONG",
						Score:     rawScore,
						Message:   fmt.Sprintf("[%s] RSI 다이버전스(LONG) → 가격:저저, RSI:고저", tf),
						CreatedAt: time.Now(),
					}
				}
				continue
			}
		}

		// Check bearish divergence
		hi1, hi2 := swingHighPair(recentCloses)
		if hi1 >= 0 && hi2 >= 0 {
			priceHigherHigh := recentCloses[hi2] > recentCloses[hi1]
			rsiLowerHigh := rsiVals[hi2] < rsiVals[hi1]
			if priceHigherHigh && rsiLowerHigh {
				priceDiff := math.Abs(recentCloses[hi2]-recentCloses[hi1]) / recentCloses[hi1]
				rsiDiff := math.Abs(rsiVals[hi1]-rsiVals[hi2]) / 100.0
				rawScore := (priceDiff + rsiDiff) / 2
				if rawScore < 0.1 {
					rawScore = 0.1
				} else if rawScore > 1.0 {
					rawScore = 1.0
				}

				weighted := rawScore * tfW[tf]
				if weighted > bestWeighted {
					bestWeighted = weighted
					bestSig = &models.Signal{
						Symbol:    ctx.Symbol,
						Timeframe: tf,
						Rule:      r.Name(),
						Direction: "SHORT",
						Score:     rawScore,
						Message:   fmt.Sprintf("[%s] RSI 다이버전스(SHORT) → 가격:고고, RSI:저고", tf),
						CreatedAt: time.Now(),
					}
				}
			}
		}
	}

	return bestSig, nil
}
