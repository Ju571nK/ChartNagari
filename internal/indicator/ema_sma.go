package indicator

// ema computes the Exponential Moving Average for the last value in prices.
// Uses the standard multiplier: 2/(period+1).
// Returns the EMA value and true if there is sufficient data.
func ema(prices []float64, period int) (float64, bool) {
	if period <= 0 || len(prices) < period {
		return 0, false
	}

	// Seed with SMA of the first `period` values.
	var sum float64
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	result := sum / float64(period)

	k := 2.0 / float64(period+1)
	for i := period; i < len(prices); i++ {
		result = prices[i]*k + result*(1-k)
	}
	return result, true
}

// sma computes the Simple Moving Average over the last `period` values.
// Returns the SMA value and true if there is sufficient data.
func sma(prices []float64, period int) (float64, bool) {
	if period <= 0 || len(prices) < period {
		return 0, false
	}
	var sum float64
	start := len(prices) - period
	for i := start; i < len(prices); i++ {
		sum += prices[i]
	}
	return sum / float64(period), true
}

// volumeMA computes the Simple Moving Average of volumes over the last `period` values.
func volumeMA(volumes []float64, period int) (float64, bool) {
	return sma(volumes, period)
}
