package indicator

import (
	"math"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// swingHighLow finds the most recent swing high and swing low in bars.
// A swing high is a bar whose high is strictly greater than the `lookback` bars on each side.
// A swing low  is a bar whose low  is strictly less    than the `lookback` bars on each side.
// Scans from the second-to-last bar backwards to find the most recent confirmed swings.
// Returns (swingHigh, swingLow, true) when both are found, (0, 0, false) otherwise.
func swingHighLow(bars []models.OHLCV, lookback int) (swingHigh, swingLow float64, ok bool) {
	n := len(bars)
	// Need at least 2*lookback+1 bars to have any valid pivot.
	if lookback <= 0 || n < 2*lookback+1 {
		return 0, 0, false
	}

	foundHigh := false
	foundLow := false
	swingHigh = math.SmallestNonzeroFloat64
	swingLow = math.MaxFloat64

	// Scan from right (excluding the last `lookback` bars which have no right side).
	for i := n - 1 - lookback; i >= lookback; i-- {
		isHigh := true
		isLow := true
		for j := i - lookback; j <= i+lookback; j++ {
			if j == i {
				continue
			}
			if bars[j].High >= bars[i].High {
				isHigh = false
			}
			if bars[j].Low <= bars[i].Low {
				isLow = false
			}
		}
		if isHigh && !foundHigh {
			swingHigh = bars[i].High
			foundHigh = true
		}
		if isLow && !foundLow {
			swingLow = bars[i].Low
			foundLow = true
		}
		if foundHigh && foundLow {
			break
		}
	}

	if !foundHigh || !foundLow {
		return 0, 0, false
	}
	return swingHigh, swingLow, true
}
