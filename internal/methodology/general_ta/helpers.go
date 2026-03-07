package general_ta

// rollingRSI computes RSI(period) for all bars using Wilder's smoothing.
// Returns values from index `period` onward (len = len(closes)-period).
// Returns nil if len(closes) <= period.
func rollingRSI(closes []float64, period int) []float64 {
	if len(closes) <= period {
		return nil
	}

	// Compute initial average gain/loss over first `period` bars
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			avgGain += diff
		} else {
			avgLoss += -diff
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	result := make([]float64, len(closes)-period)

	// First RSI value
	if avgLoss == 0 {
		result[0] = 100
	} else {
		rs := avgGain / avgLoss
		result[0] = 100 - 100/(1+rs)
	}

	// Wilder's smoothing for remaining bars
	for i := period + 1; i < len(closes); i++ {
		diff := closes[i] - closes[i-1]
		var gain, loss float64
		if diff > 0 {
			gain = diff
		} else {
			loss = -diff
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)

		if avgLoss == 0 {
			result[i-period] = 100
		} else {
			rs := avgGain / avgLoss
			result[i-period] = 100 - 100/(1+rs)
		}
	}

	return result
}

// rollingEMA computes EMA(period) starting from the seed SMA.
// Returns len(closes)-period+1 values (last = current EMA).
// Returns nil if len(closes) < period.
func rollingEMA(closes []float64, period int) []float64 {
	if len(closes) < period {
		return nil
	}

	k := 2.0 / float64(period+1)

	// Seed with SMA of first `period` bars
	var seed float64
	for i := 0; i < period; i++ {
		seed += closes[i]
	}
	seed /= float64(period)

	result := make([]float64, len(closes)-period+1)
	result[0] = seed

	for i := period; i < len(closes); i++ {
		result[i-period+1] = closes[i]*k + result[i-period]*(1-k)
	}

	return result
}

// swingLowPair finds the indices of the two most recent swing lows in closes.
// A swing low: closes[i] < closes[i-1] && closes[i] < closes[i+1].
// Returns (idx1, idx2) where idx1 < idx2 (earlier swing, later swing).
// Returns (-1, -1) if fewer than 2 swing lows found.
func swingLowPair(closes []float64) (idx1, idx2 int) {
	var lows []int
	for i := 1; i < len(closes)-1; i++ {
		if closes[i] < closes[i-1] && closes[i] < closes[i+1] {
			lows = append(lows, i)
		}
	}
	if len(lows) < 2 {
		return -1, -1
	}
	// Return the two most recent
	n := len(lows)
	return lows[n-2], lows[n-1]
}

// swingHighPair finds the indices of the two most recent swing highs.
func swingHighPair(closes []float64) (idx1, idx2 int) {
	var highs []int
	for i := 1; i < len(closes)-1; i++ {
		if closes[i] > closes[i-1] && closes[i] > closes[i+1] {
			highs = append(highs, i)
		}
	}
	if len(highs) < 2 {
		return -1, -1
	}
	n := len(highs)
	return highs[n-2], highs[n-1]
}
