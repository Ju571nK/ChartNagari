// Package indicator provides technical indicator computations for the Chart Nagari.
// All indicator functions are package-internal; only Compute is exported.
// Indicator key format: "{TF}:{IndicatorName}" e.g. "1H:RSI_14", "4H:EMA_200".
package indicator

import (
	"fmt"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// Compute calculates all supported indicators for each timeframe in bars.
// It returns a flat map with prefixed keys in the form "{TF}:{indicator}".
// If data is insufficient for a particular indicator, that key is omitted.
func Compute(bars map[string][]models.OHLCV) map[string]float64 {
	result := make(map[string]float64)
	if len(bars) == 0 {
		return result
	}

	for tf, candles := range bars {
		if len(candles) == 0 {
			continue
		}

		// Extract price/volume slices.
		closes := make([]float64, len(candles))
		highs := make([]float64, len(candles))
		lows := make([]float64, len(candles))
		volumes := make([]float64, len(candles))
		for i, c := range candles {
			closes[i] = c.Close
			highs[i] = c.High
			lows[i] = c.Low
			volumes[i] = c.Volume
		}

		prefix := tf + ":"

		// RSI 14
		if v, ok := rsi(closes, 14); ok {
			result[prefix+"RSI_14"] = v
		}

		// EMAs
		for _, p := range []int{9, 20, 50, 200} {
			if v, ok := ema(closes, p); ok {
				result[fmt.Sprintf("%sEMA_%d", prefix, p)] = v
			}
		}

		// SMAs
		for _, p := range []int{20, 50, 200} {
			if v, ok := sma(closes, p); ok {
				result[fmt.Sprintf("%sSMA_%d", prefix, p)] = v
			}
		}

		// Volume MA 20
		if v, ok := volumeMA(volumes, 20); ok {
			result[prefix+"VOLUME_MA_20"] = v
		}

		// MACD (12,26,9)
		if line, sig, hist, ok := macd(closes); ok {
			result[prefix+"MACD_line"] = line
			result[prefix+"MACD_signal"] = sig
			result[prefix+"MACD_hist"] = hist
		}

		// Bollinger Bands (20, 2)
		if upper, middle, lower, width, pct, ok := bollingerBands(closes, 20, 2.0); ok {
			result[prefix+"BB_upper"] = upper
			result[prefix+"BB_middle"] = middle
			result[prefix+"BB_lower"] = lower
			result[prefix+"BB_width"] = width
			result[prefix+"BB_pct"] = pct
		}

		// OBV
		result[prefix+"OBV"] = obv(closes, volumes)

		// ATR 14
		if v, ok := atr(highs, lows, closes, 14); ok {
			result[prefix+"ATR_14"] = v
		}

		// ADX 14 (trend strength 0-100)
		if v, ok := adx(highs, lows, closes, 14); ok {
			result[prefix+"ADX_14"] = v
		}

		// Swing High / Low (lookback=5)
		if sh, sl, ok := swingHighLow(candles, 5); ok {
			result[prefix+"SWING_HIGH"] = sh
			result[prefix+"SWING_LOW"] = sl

			// Fibonacci levels derived from swing points.
			for k, v := range fibonacci(sh, sl) {
				result[prefix+k] = v
			}
		}

		// Volume Profile (20 bins)
		if poc, hvns, lvns, ok := volumeProfile(candles, 20); ok {
			result[prefix+"VP_POC"] = poc
			for j, h := range hvns {
				result[fmt.Sprintf("%sVP_HVN_%d", prefix, j+1)] = h
			}
			for j, l := range lvns {
				result[fmt.Sprintf("%sVP_LVN_%d", prefix, j+1)] = l
			}
		}
	}

	return result
}
