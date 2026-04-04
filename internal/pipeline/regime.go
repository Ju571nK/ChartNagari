package pipeline

import (
	"math"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// volatilityRegime classifies the current ATR environment.
type volatilityRegime int

const (
	regimeNormal volatilityRegime = iota
	regimeLowVol
	regimeHighVol
)

// atrPercentile computes the percentile rank (0–100) of currentATR among the
// rolling ATR values derived from the last historyLen bars.
//
// The function:
//  1. Takes the most recent historyLen bars (DESC order: index 0 = newest).
//  2. Reverses them to ASC order for sequential True Range computation.
//  3. Computes a rolling Wilder ATR with the given period, producing
//     (historyLen - period) ATR values.
//  4. Returns the percentile rank of currentATR among those values.
//
// Returns -1 when there is insufficient data (fewer than historyLen bars or
// fewer than period+1 bars to seed the first ATR).
func atrPercentile(bars []models.OHLCV, currentATR float64, period, historyLen int) float64 {
	if len(bars) < historyLen {
		return -1
	}

	// Use the most recent historyLen bars. bars is DESC (index 0 = newest),
	// so take the first historyLen elements and then reverse to ASC order.
	window := bars[:historyLen]
	asc := make([]models.OHLCV, historyLen)
	for i, b := range window {
		asc[historyLen-1-i] = b
	}

	// We need at least period+1 bars to produce the first ATR value.
	if historyLen < period+1 {
		return -1
	}

	// True Range helper (ASC indexed, i >= 1).
	tr := func(i int) float64 {
		hl := asc[i].High - asc[i].Low
		hc := math.Abs(asc[i].High - asc[i-1].Close)
		lc := math.Abs(asc[i].Low - asc[i-1].Close)
		return math.Max(hl, math.Max(hc, lc))
	}

	// Seed: simple average of the first `period` true ranges.
	var seed float64
	for i := 1; i <= period; i++ {
		seed += tr(i)
	}
	seed /= float64(period)

	// Collect rolling ATR values starting from bar index `period`.
	// That gives historyLen - period values total.
	numATR := historyLen - period
	if numATR <= 0 {
		return -1
	}
	atrHistory := make([]float64, 0, numATR)
	cur := seed
	atrHistory = append(atrHistory, cur)
	for i := period + 1; i < historyLen; i++ {
		cur = (cur*float64(period-1) + tr(i)) / float64(period)
		atrHistory = append(atrHistory, cur)
	}

	// Percentile rank: count how many historical ATR values are <= currentATR.
	count := 0
	for _, v := range atrHistory {
		if v <= currentATR {
			count++
		}
	}
	return float64(count) / float64(len(atrHistory)) * 100.0
}

// applyVolatilityRegime adjusts signal scores based on the ATR percentile regime.
//
// For each signal, the function looks up ATR_14 from indicators (keyed by
// the signal's timeframe), then calls atrPercentile against the bars for that
// timeframe. Score multipliers:
//
//	HIGH_VOL  (percentile >= cfg.HighVolPercentile): Score *= 1 + bonus/100
//	LOW_VOL   (percentile <= cfg.LowVolPercentile) : Score *= 1 - penalty/100
//	NORMAL                                          : unchanged
func applyVolatilityRegime(
	signals []models.Signal,
	allBars map[string][]models.OHLCV,
	indicators map[string]float64,
	cfg appconfig.VolatilityRegimeConfig,
) {
	const (
		period     = 14
		historyLen = 90
	)

	for i := range signals {
		sig := &signals[i]
		tf := sig.Timeframe

		currentATR, hasATR := indicators[tf+":ATR_14"]
		if !hasATR || currentATR <= 0 {
			continue
		}

		bars, hasBars := allBars[tf]
		if !hasBars {
			continue
		}

		pct := atrPercentile(bars, currentATR, period, historyLen)
		if pct < 0 {
			continue // insufficient data — skip regime classification
		}

		regime := regimeNormal
		if pct >= float64(cfg.HighVolPercentile) {
			regime = regimeHighVol
		} else if pct <= float64(cfg.LowVolPercentile) {
			regime = regimeLowVol
		}

		switch regime {
		case regimeHighVol:
			sig.Score *= 1.0 + float64(cfg.HighVolBonusPct)/100.0
		case regimeLowVol:
			sig.Score *= 1.0 - float64(cfg.LowVolPenaltyPct)/100.0
		}
	}
}

// atrSlopeRising returns true when the EMA of the provided ATR history values
// is trending upward — i.e., the final EMA value is greater than the
// second-to-last EMA value.
//
// emaPeriod controls the smoothing window. Returns false when the slice has
// fewer than emaPeriod+1 elements (not enough to compare two EMA steps).
func atrSlopeRising(atrHistory []float64, emaPeriod int) bool {
	if emaPeriod <= 0 || len(atrHistory) < emaPeriod+1 {
		return false
	}

	// Seed: simple average of the first emaPeriod values.
	var seed float64
	for i := 0; i < emaPeriod; i++ {
		seed += atrHistory[i]
	}
	seed /= float64(emaPeriod)

	k := 2.0 / float64(emaPeriod+1)
	prev := seed
	var last float64
	for i := emaPeriod; i < len(atrHistory); i++ {
		last = atrHistory[i]*k + prev*(1-k)
		prev = last
	}

	// Compare final EMA to the EMA one step back (prev before last iteration).
	// Re-compute the second-to-last EMA value.
	prev2 := seed
	end := len(atrHistory) - 1 // index of last element
	for i := emaPeriod; i < end; i++ {
		prev2 = atrHistory[i]*k + prev2*(1-k)
	}

	return last > prev2
}

// applyATRSlopeBonus boosts signal scores when the ATR EMA is rising,
// indicating expanding volatility which tends to produce stronger moves.
//
// The function recomputes the rolling ATR history (same 90-bar window as
// applyVolatilityRegime) and delegates to atrSlopeRising.
func applyATRSlopeBonus(
	signals []models.Signal,
	allBars map[string][]models.OHLCV,
	indicators map[string]float64,
	cfg appconfig.ATRSlopeConfig,
) {
	const (
		period     = 14
		historyLen = 90
	)

	for i := range signals {
		sig := &signals[i]
		tf := sig.Timeframe

		currentATR, hasATR := indicators[tf+":ATR_14"]
		if !hasATR || currentATR <= 0 {
			continue
		}

		bars, hasBars := allBars[tf]
		if !hasBars || len(bars) < historyLen {
			continue
		}

		// Build the same rolling ATR history as atrPercentile.
		window := bars[:historyLen]
		asc := make([]models.OHLCV, historyLen)
		for j, b := range window {
			asc[historyLen-1-j] = b
		}

		tr := func(idx int) float64 {
			hl := asc[idx].High - asc[idx].Low
			hc := math.Abs(asc[idx].High - asc[idx-1].Close)
			lc := math.Abs(asc[idx].Low - asc[idx-1].Close)
			return math.Max(hl, math.Max(hc, lc))
		}

		var seed float64
		for j := 1; j <= period; j++ {
			seed += tr(j)
		}
		seed /= float64(period)

		numATR := historyLen - period
		if numATR <= 0 {
			continue
		}
		atrHistory := make([]float64, 0, numATR)
		cur := seed
		atrHistory = append(atrHistory, cur)
		for j := period + 1; j < historyLen; j++ {
			cur = (cur*float64(period-1) + tr(j)) / float64(period)
			atrHistory = append(atrHistory, cur)
		}

		emaPeriod := cfg.EMAPeriod
		if emaPeriod <= 0 {
			emaPeriod = 20
		}

		if atrSlopeRising(atrHistory, emaPeriod) {
			sig.Score *= 1.0 + float64(cfg.RisingBonusPct)/100.0
		}
	}
}
