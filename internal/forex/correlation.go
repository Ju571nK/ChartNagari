package forex

import (
	"math"
	"sort"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// Correlation computes Pearson correlation of aligned daily close returns.
// Missing dates, insufficient history and constant return series yield nil.
func Correlation(pair, dxy []models.OHLCV, window int) *float64 {
	if window < 2 {
		return nil
	}
	byDate := func(bars []models.OHLCV) map[string]float64 {
		m := map[string]float64{}
		for _, b := range bars {
			if b.Close > 0 {
				m[TradingDate(b)] = b.Close
			}
		}
		return m
	}
	a, b := byDate(pair), byDate(dxy)
	dates := make([]string, 0)
	for d := range a {
		if b[d] > 0 {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	if len(dates) < window+1 {
		return nil
	}
	dates = dates[len(dates)-window-1:]
	x, y := make([]float64, window), make([]float64, window)
	for i := 0; i < window; i++ {
		x[i] = a[dates[i+1]]/a[dates[i]] - 1
		y[i] = b[dates[i+1]]/b[dates[i]] - 1
	}
	var sx, sy, sxx, syy, sxy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		syy += y[i] * y[i]
		sxy += x[i] * y[i]
	}
	n := float64(window)
	den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
	if den <= 0 {
		return nil
	}
	v := (n*sxy - sx*sy) / den
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	if v > 1 {
		v = 1
	}
	if v < -1 {
		v = -1
	}
	return &v
}

// TradingDate labels a daily candle by its trading close date. OANDA D bars
// open at 17:00 New York on the previous calendar day, including Sunday.
// Yahoo daily bars use the date encoded by their UTC open timestamp.
func TradingDate(bar models.OHLCV) string {
	if bar.Source == "oanda" {
		ny, _ := time.LoadLocation("America/New_York")
		return bar.OpenTime.In(ny).AddDate(0, 0, 1).Format("2006-01-02")
	}
	return bar.OpenTime.UTC().Format("2006-01-02")
}

// DailyDate preserves the historical UTC date helper for callers without source metadata.
func DailyDate(t time.Time) string { return t.UTC().Format("2006-01-02") }
