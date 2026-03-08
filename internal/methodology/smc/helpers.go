package smc

import "github.com/Ju571nK/Chatter/pkg/models"

// trendDir returns the trend direction of the last n bars.
// Splits bars into first half / second half and compares average close prices.
// Returns "UP"   if secondAvg > firstAvg * 1.01,
//         "DOWN" if secondAvg < firstAvg * 0.99,
//         "NONE" otherwise (sideways or insufficient data).
func trendDir(bars []models.OHLCV, n int) string {
	if len(bars) < n {
		n = len(bars)
	}
	if n < 4 {
		return "NONE"
	}
	sample := bars[len(bars)-n:]
	half := len(sample) / 2

	var firstSum, secondSum float64
	for _, b := range sample[:half] {
		firstSum += b.Close
	}
	for _, b := range sample[half:] {
		secondSum += b.Close
	}

	firstAvg := firstSum / float64(half)
	secondAvg := secondSum / float64(len(sample)-half)

	switch {
	case secondAvg > firstAvg*1.01:
		return "UP"
	case secondAvg < firstAvg*0.99:
		return "DOWN"
	default:
		return "NONE"
	}
}

// structuralHigh returns the highest High among bars.
func structuralHigh(bars []models.OHLCV) float64 {
	if len(bars) == 0 {
		return 0
	}
	h := bars[0].High
	for _, b := range bars[1:] {
		if b.High > h {
			h = b.High
		}
	}
	return h
}

// structuralLow returns the lowest Low among bars.
func structuralLow(bars []models.OHLCV) float64 {
	if len(bars) == 0 {
		return 0
	}
	l := bars[0].Low
	for _, b := range bars[1:] {
		if b.Low < l {
			l = b.Low
		}
	}
	return l
}
