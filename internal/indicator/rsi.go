package indicator

// rsi computes the Relative Strength Index using Wilder's smoothing method.
// Returns the RSI value and true if there is sufficient data, otherwise false.
// Requires at least period+1 closes.
func rsi(closes []float64, period int) (float64, bool) {
	if period <= 0 || len(closes) < period+1 {
		return 0, false
	}

	// Compute initial average gain and loss over the first `period` changes.
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			avgGain += change
		} else {
			avgLoss += -change
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	// Apply Wilder's smoothing for the remaining bars.
	for i := period + 1; i < len(closes); i++ {
		change := closes[i] - closes[i-1]
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}

	if avgLoss == 0 {
		return 100, true
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs)), true
}
