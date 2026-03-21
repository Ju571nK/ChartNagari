package candlestick

import (
	"math"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// candleBody returns the absolute difference between Close and Open.
func candleBody(b models.OHLCV) float64 {
	return math.Abs(b.Close - b.Open)
}

// candleRange returns High - Low.
func candleRange(b models.OHLCV) float64 {
	return b.High - b.Low
}

// upperShadow returns the distance from the top of the body to the High.
func upperShadow(b models.OHLCV) float64 {
	return b.High - math.Max(b.Open, b.Close)
}

// lowerShadow returns the distance from the bottom of the body to the Low.
func lowerShadow(b models.OHLCV) float64 {
	return math.Min(b.Open, b.Close) - b.Low
}

// bodyRatio returns candleBody / candleRange. Returns 0 if range is zero.
func bodyRatio(b models.OHLCV) float64 {
	r := candleRange(b)
	if r == 0 {
		return 0
	}
	return candleBody(b) / r
}

// isBullish returns true when Close > Open.
func isBullish(b models.OHLCV) bool {
	return b.Close > b.Open
}

// isBearish returns true when Close < Open.
func isBearish(b models.OHLCV) bool {
	return b.Close < b.Open
}

// midpoint returns the center of the candle body.
func midpoint(b models.OHLCV) float64 {
	return (math.Max(b.Open, b.Close) + math.Min(b.Open, b.Close)) / 2
}

// isUptrend returns true if the last bar's close is higher than the bar
// at len(bars)-lookback.
func isUptrend(bars []models.OHLCV, lookback int) bool {
	n := len(bars)
	if n < lookback {
		return false
	}
	return bars[n-1].Close > bars[n-lookback].Close
}

// isDowntrend returns true if the last bar's close is lower than the bar
// at len(bars)-lookback.
func isDowntrend(bars []models.OHLCV, lookback int) bool {
	n := len(bars)
	if n < lookback {
		return false
	}
	return bars[n-1].Close < bars[n-lookback].Close
}
