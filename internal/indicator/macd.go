package indicator

// macd computes the MACD line, signal line, and histogram using standard parameters:
// fast=12, slow=26, signal=9.
// Returns (line, signal, histogram, ok).
// ok is false when there is insufficient data (need at least 26+8=34 bars for signal).
func macd(closes []float64) (line, signal, hist float64, ok bool) {
	const (
		fastPeriod   = 12
		slowPeriod   = 26
		signalPeriod = 9
	)
	// Minimum bars needed: slowPeriod to seed slow EMA, then signalPeriod-1 more bars
	// to compute the signal EMA (first MACD value seeds the signal EMA).
	minBars := slowPeriod + signalPeriod - 1
	if len(closes) < minBars {
		return 0, 0, 0, false
	}

	// Compute MACD line values for all bars from index slowPeriod-1 onward.
	// We need signalPeriod MACD values to seed the signal EMA.
	macdValues := make([]float64, 0, len(closes)-slowPeriod+1)

	// Seed fast EMA with first fastPeriod bars.
	var fastSum float64
	for i := 0; i < fastPeriod; i++ {
		fastSum += closes[i]
	}
	fastEMA := fastSum / float64(fastPeriod)
	kFast := 2.0 / float64(fastPeriod+1)

	// Seed slow EMA with first slowPeriod bars.
	var slowSum float64
	for i := 0; i < slowPeriod; i++ {
		slowSum += closes[i]
	}
	slowEMA := slowSum / float64(slowPeriod)
	kSlow := 2.0 / float64(slowPeriod+1)

	// First MACD value (at index slowPeriod-1) using the seeded EMAs.
	macdValues = append(macdValues, fastEMA-slowEMA)

	// Advance both EMAs from index slowPeriod onward.
	for i := slowPeriod; i < len(closes); i++ {
		fastEMA = closes[i]*kFast + fastEMA*(1-kFast)
		slowEMA = closes[i]*kSlow + slowEMA*(1-kSlow)
		macdValues = append(macdValues, fastEMA-slowEMA)
	}

	if len(macdValues) < signalPeriod {
		return 0, 0, 0, false
	}

	// Seed signal EMA with first signalPeriod MACD values.
	var sigSum float64
	for i := 0; i < signalPeriod; i++ {
		sigSum += macdValues[i]
	}
	sigEMA := sigSum / float64(signalPeriod)
	kSig := 2.0 / float64(signalPeriod+1)

	for i := signalPeriod; i < len(macdValues); i++ {
		sigEMA = macdValues[i]*kSig + sigEMA*(1-kSig)
	}

	line = macdValues[len(macdValues)-1]
	signal = sigEMA
	hist = line - signal
	ok = true
	return
}
