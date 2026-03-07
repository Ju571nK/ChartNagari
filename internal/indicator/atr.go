package indicator

import "math"

// atr computes Average True Range using Wilder's smoothing.
// Requires at least period+1 bars (one extra bar to compute the first true range).
// Returns (value, true) on success, (0, false) on insufficient data.
func atr(highs, lows, closes []float64, period int) (float64, bool) {
	n := len(closes)
	if period <= 0 || n < period+1 || len(highs) < n || len(lows) < n {
		return 0, false
	}

	// True Range for bar i: max(high-low, |high-prevClose|, |low-prevClose|)
	tr := func(i int) float64 {
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		return math.Max(hl, math.Max(hc, lc))
	}

	// Seed: simple average of first `period` true ranges (bars 1..period).
	var sum float64
	for i := 1; i <= period; i++ {
		sum += tr(i)
	}
	atrVal := sum / float64(period)

	// Wilder's smoothing for the rest.
	for i := period + 1; i < n; i++ {
		atrVal = (atrVal*float64(period-1) + tr(i)) / float64(period)
	}
	return atrVal, true
}
