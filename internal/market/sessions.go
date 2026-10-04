package market

import (
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// Sessions returns ICT session windows for the New York calendar date containing date.
// Asia starts the prior evening. Converting local wall times individually handles DST.
func Sessions(date time.Time) models.SessionContext {
	local := date.In(nyLoc)
	y, m, d := local.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, nyLoc)
	prior := midnight.AddDate(0, 0, -1)
	window := func(day time.Time, start, end int) models.SessionWindow {
		y, m, d := day.Date()
		return models.SessionWindow{
			Open:  nyWallTime(y, m, d, start),
			Close: nyWallTime(y, m, d, end),
		}
	}
	return models.SessionContext{
		Asia:        models.SessionWindow{Open: window(prior, 20, 21).Open, Close: midnight},
		London:      window(midnight, 2, 5),
		NewYork:     window(midnight, 7, 10),
		LondonClose: window(midnight, 10, 12),
	}
}

// nyWallTime moves a skipped spring-forward wall hour to the first valid time
// after the jump. time.Date alone maps 02:00 back to 01:00 on that Sunday.
func nyWallTime(y int, m time.Month, d, hour int) time.Time {
	t := time.Date(y, m, d, hour, 0, 0, 0, nyLoc)
	for t.In(nyLoc).Hour() < hour {
		t = t.Add(time.Minute)
	}
	return t
}

// SessionsAt chooses the trading day containing an instant. The evening Asian
// window belongs to the following New York calendar day.
func SessionsAt(t time.Time) models.SessionContext {
	local := t.In(nyLoc)
	if local.Hour() >= 20 {
		local = local.AddDate(0, 0, 1)
	}
	return Sessions(local)
}

// AsianRange adds the high/low of bars whose open instant falls in the Asian window.
func AsianRange(bars []models.OHLCV, date time.Time) models.SessionContext {
	s := SessionsAt(date)
	for _, bar := range bars {
		if !s.Asia.Contains(bar.OpenTime) {
			continue
		}
		if !s.AsianRangeValid {
			s.AsianHigh, s.AsianLow, s.AsianRangeValid = bar.High, bar.Low, true
		} else {
			if bar.High > s.AsianHigh {
				s.AsianHigh = bar.High
			}
			if bar.Low < s.AsianLow {
				s.AsianLow = bar.Low
			}
		}
	}
	return s
}
