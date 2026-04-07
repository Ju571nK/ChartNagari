package indicator

import "math"

// realizedVol computes annualized realized volatility from close prices.
// Uses log returns over N periods, annualized by sqrt(252).
// Returns (value, true) on success, (0, false) if insufficient data.
// The result is expressed as a percentage (e.g., 15.2 means 15.2%).
func realizedVol(closes []float64, period int) (float64, bool) {
	if period <= 0 || len(closes) < period+1 {
		return 0, false
	}

	// Compute log returns for the last `period` intervals.
	// closes[len-period-1] .. closes[len-1] gives `period` returns.
	start := len(closes) - period - 1
	returns := make([]float64, period)
	for i := 0; i < period; i++ {
		prev := closes[start+i]
		cur := closes[start+i+1]
		if prev <= 0 {
			return 0, false
		}
		returns[i] = math.Log(cur / prev)
	}

	// Mean of log returns.
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(period)

	// Standard deviation of log returns.
	var variance float64
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(period - 1) // sample variance
	stddev := math.Sqrt(variance)

	// Annualize: stddev * sqrt(252), convert to percentage.
	annualized := stddev * math.Sqrt(252) * 100.0

	return annualized, true
}

// ComputeRealizedVol is an exported wrapper around realizedVol for use by the API layer.
// Returns the annualized realized volatility as a percentage, or 0 if insufficient data.
func ComputeRealizedVol(closes []float64, period int) float64 {
	v, ok := realizedVol(closes, period)
	if !ok {
		return 0
	}
	return v
}
