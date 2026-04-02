package indicator

import "math"

// adx computes the Average Directional Index (trend strength, 0-100).
// Uses Wilder's smoothing with the given period (typically 14).
// Returns (value, true) on success, (0, false) on insufficient data.
//
// ADX > 25 → trending market
// ADX < 20 → ranging/weak market
// ADX 20-25 → ambiguous
func adx(highs, lows, closes []float64, period int) (float64, bool) {
	n := len(closes)
	// Need at least 2*period+1 bars for meaningful ADX
	if period <= 0 || n < 2*period+1 || len(highs) < n || len(lows) < n {
		return 0, false
	}

	// Directional Movement: +DM and -DM
	plusDM := make([]float64, n)
	minusDM := make([]float64, n)
	trueRange := make([]float64, n)

	for i := 1; i < n; i++ {
		upMove := highs[i] - highs[i-1]
		downMove := lows[i-1] - lows[i]

		if upMove > downMove && upMove > 0 {
			plusDM[i] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDM[i] = downMove
		}

		// True Range
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		trueRange[i] = math.Max(hl, math.Max(hc, lc))
	}

	// Wilder's smoothed averages (seed with sum of first period values)
	var smoothPlusDM, smoothMinusDM, smoothTR float64
	for i := 1; i <= period; i++ {
		smoothPlusDM += plusDM[i]
		smoothMinusDM += minusDM[i]
		smoothTR += trueRange[i]
	}

	// DX values for ADX smoothing
	dxValues := make([]float64, 0, n-period)

	for i := period + 1; i < n; i++ {
		smoothPlusDM = smoothPlusDM - smoothPlusDM/float64(period) + plusDM[i]
		smoothMinusDM = smoothMinusDM - smoothMinusDM/float64(period) + minusDM[i]
		smoothTR = smoothTR - smoothTR/float64(period) + trueRange[i]

		if smoothTR == 0 {
			dxValues = append(dxValues, 0)
			continue
		}

		plusDI := 100.0 * smoothPlusDM / smoothTR
		minusDI := 100.0 * smoothMinusDM / smoothTR

		diSum := plusDI + minusDI
		if diSum == 0 {
			dxValues = append(dxValues, 0)
			continue
		}

		dx := 100.0 * math.Abs(plusDI-minusDI) / diSum
		dxValues = append(dxValues, dx)
	}

	if len(dxValues) < period {
		return 0, false
	}

	// ADX = Wilder's smoothed average of DX values
	var adxVal float64
	for i := 0; i < period; i++ {
		adxVal += dxValues[i]
	}
	adxVal /= float64(period)

	for i := period; i < len(dxValues); i++ {
		adxVal = (adxVal*float64(period-1) + dxValues[i]) / float64(period)
	}

	return math.Round(adxVal*100) / 100, true
}
