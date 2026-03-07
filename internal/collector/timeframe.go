// Package collector provides data collection from Binance WebSocket and Yahoo Finance.
package collector

import (
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// tfDuration maps timeframe string to its duration.
var tfDuration = map[string]time.Duration{
	"1H": time.Hour,
	"4H": 4 * time.Hour,
	"1D": 24 * time.Hour,
	"1W": 7 * 24 * time.Hour,
}

// RebuildHigherTF aggregates 1H bars into 4H, 1D, 1W bars.
// Input bars must be sorted ascending by OpenTime.
// Returns a map of timeframe → reconstructed bars.
func RebuildHigherTF(symbol string, bars1H []models.OHLCV) map[string][]models.OHLCV {
	result := map[string][]models.OHLCV{
		"4H": aggregateBars(symbol, "4H", bars1H, 4*time.Hour),
		"1D": aggregateBars(symbol, "1D", bars1H, 24*time.Hour),
		"1W": aggregateBars(symbol, "1W", bars1H, 7*24*time.Hour),
	}
	return result
}

// aggregateBars groups 1H bars into the target timeframe.
// The bucket start time is the floor of the bar's OpenTime to the target duration.
func aggregateBars(symbol, tf string, bars []models.OHLCV, period time.Duration) []models.OHLCV {
	type bucket struct {
		openTime time.Time
		open     float64
		high     float64
		low      float64
		close    float64
		volume   float64
		count    int
	}

	buckets := map[int64]*bucket{}
	order := []int64{}

	for _, b := range bars {
		key := floorTime(b.OpenTime, period).UnixMilli()
		if _, exists := buckets[key]; !exists {
			buckets[key] = &bucket{
				openTime: floorTime(b.OpenTime, period),
				open:     b.Open,
				high:     b.High,
				low:      b.Low,
				close:    b.Close,
				volume:   b.Volume,
				count:    1,
			}
			order = append(order, key)
		} else {
			bk := buckets[key]
			if b.High > bk.high {
				bk.high = b.High
			}
			if b.Low < bk.low {
				bk.low = b.Low
			}
			bk.close = b.Close
			bk.volume += b.Volume
			bk.count++
		}
	}

	var result []models.OHLCV
	for _, key := range order {
		bk := buckets[key]
		result = append(result, models.OHLCV{
			Symbol:    symbol,
			Timeframe: tf,
			OpenTime:  bk.openTime,
			Open:      bk.open,
			High:      bk.high,
			Low:       bk.low,
			Close:     bk.close,
			Volume:    bk.volume,
		})
	}
	return result
}

// floorTime truncates t to the nearest multiple of d from Unix epoch.
func floorTime(t time.Time, d time.Duration) time.Time {
	return t.Truncate(d).UTC()
}

// BinanceTFMap maps our internal TF strings to Binance kline interval strings.
var BinanceTFMap = map[string]string{
	"1H": "1h",
	"4H": "4h",
	"1D": "1d",
	"1W": "1w",
}
