package indicator

import "math"

// bollingerBands computes Bollinger Bands over the last `period` closes.
// k is the standard deviation multiplier (typically 2.0).
// Returns (upper, middle, lower, width, %B, ok).
// width = (upper - lower) / middle
// %B   = (close - lower) / (upper - lower)  [NaN-safe: returns 0.5 when upper==lower]
func bollingerBands(closes []float64, period int, k float64) (upper, middle, lower, width, pct float64, ok bool) {
	if period <= 0 || len(closes) < period {
		return 0, 0, 0, 0, 0, false
	}

	// Use the last `period` bars.
	slice := closes[len(closes)-period:]

	var sum float64
	for _, v := range slice {
		sum += v
	}
	mean := sum / float64(period)

	var variance float64
	for _, v := range slice {
		diff := v - mean
		variance += diff * diff
	}
	// Population standard deviation (matches most charting platforms).
	stddev := math.Sqrt(variance / float64(period))

	upper = mean + k*stddev
	middle = mean
	lower = mean - k*stddev

	if middle != 0 {
		width = (upper - lower) / middle
	}

	band := upper - lower
	if band == 0 {
		pct = 0.5
	} else {
		lastClose := closes[len(closes)-1]
		pct = (lastClose - lower) / band
	}

	return upper, middle, lower, width, pct, true
}
