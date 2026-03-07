package indicator

// obv computes On-Balance Volume.
// OBV starts at 0 and adds volume when close > prev close, subtracts when close < prev close.
// Returns 0 when fewer than 2 bars are provided (no change can be measured).
func obv(closes, volumes []float64) float64 {
	n := len(closes)
	if n < 2 || len(volumes) < n {
		return 0
	}
	var result float64
	for i := 1; i < n; i++ {
		switch {
		case closes[i] > closes[i-1]:
			result += volumes[i]
		case closes[i] < closes[i-1]:
			result -= volumes[i]
		// equal: no change
		}
	}
	return result
}
