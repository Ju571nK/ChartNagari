package general_ta

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// VSAEffortCandleRule detects Volume Spread Analysis effort-vs-result patterns.
//
// Three patterns:
//
// 1. Stopping Volume: high volume (>= 1.5x volume MA) + narrow body (<= 0.3x ATR)
//    + close in bottom 40% of range + prior 5-bar downtrend → LONG
//    (Institutional absorption of selling pressure.)
//
// 2. No Demand: low volume (< 0.8x volume MA) + narrow body (<= 0.3x ATR)
//    + bullish candle (close > open) + prior uptrend → SHORT
//    (No buyers stepping in during uptrend — reversal imminent.)
//
// 3. No Supply: low volume (< 0.8x volume MA) + narrow body (<= 0.3x ATR)
//    + bearish candle (close < open) + prior downtrend → LONG
//    (No sellers stepping in during downtrend — reversal imminent.)
type VSAEffortCandleRule struct{}

func (r *VSAEffortCandleRule) Name() string                 { return "vsa_effort_candle" }
func (r *VSAEffortCandleRule) RequiredIndicators() []string { return []string{"ATR_14", "VOLUME_MA_20"} }

// isDowntrend checks if the last `n` bars show a downtrend (close[last] < close[first]).
func isDowntrend(bars []models.OHLCV, end int, lookback int) bool {
	start := end - lookback
	if start < 0 {
		start = 0
	}
	if start >= end {
		return false
	}
	return bars[end].Close < bars[start].Close
}

// isUptrend checks if the last `n` bars show an uptrend (close[last] > close[first]).
func isUptrend(bars []models.OHLCV, end int, lookback int) bool {
	start := end - lookback
	if start < 0 {
		start = 0
	}
	if start >= end {
		return false
	}
	return bars[end].Close > bars[start].Close
}

func (r *VSAEffortCandleRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 6 {
			continue
		}

		atr, hasATR := ctx.Indicators[tf+":ATR_14"]
		volMA, hasVol := ctx.Indicators[tf+":VOLUME_MA_20"]
		if !hasATR || !hasVol || atr <= 0 || volMA <= 0 {
			continue
		}

		n := len(bars)
		curr := bars[n-1]
		body := math.Abs(curr.Close - curr.Open)
		candleRange := curr.High - curr.Low
		volRatio := curr.Volume / volMA

		narrowBody := body <= 0.3*atr

		var dir string
		var rawScore float64
		var pattern string

		// 1. Stopping Volume
		if volRatio >= 1.5 && narrowBody && candleRange > 0 {
			closePos := (curr.Close - curr.Low) / candleRange
			if closePos <= 0.4 && isDowntrend(bars, n-2, 5) {
				dir = "LONG"
				rawScore = volRatio / 3.0
				if rawScore < 0.1 {
					rawScore = 0.1
				}
				if rawScore > 1.0 {
					rawScore = 1.0
				}
				pattern = "Stopping Volume"
			}
		}

		// 2. No Demand (only if no stopping volume detected)
		if dir == "" && volRatio < 0.8 && narrowBody && curr.Close > curr.Open {
			if isUptrend(bars, n-2, 5) {
				dir = "SHORT"
				rawScore = 0.5
				pattern = "No Demand"
			}
		}

		// 3. No Supply (only if nothing else detected)
		if dir == "" && volRatio < 0.8 && narrowBody && curr.Close < curr.Open {
			if isDowntrend(bars, n-2, 5) {
				dir = "LONG"
				rawScore = 0.5
				pattern = "No Supply"
			}
		}

		if dir == "" {
			continue
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
				Message:   fmt.Sprintf("[%s] VSA %s (볼륨비: %.1fx) → %s", tf, pattern, volRatio, dir),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
