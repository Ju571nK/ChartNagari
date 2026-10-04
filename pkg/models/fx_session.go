package models

import "time"

// SessionWindow uses absolute instants so its boundaries remain correct across DST.
type SessionWindow struct {
	Open  time.Time `json:"open"`
	Close time.Time `json:"close"`
}

func (w SessionWindow) Contains(t time.Time) bool {
	return !t.Before(w.Open) && t.Before(w.Close)
}

// SessionContext describes the New York trading day and its completed Asian range.
type SessionContext struct {
	Asia            SessionWindow `json:"asia"`
	London          SessionWindow `json:"london"`
	NewYork         SessionWindow `json:"new_york"`
	LondonClose     SessionWindow `json:"london_close"`
	AsianHigh       float64       `json:"asian_high,omitempty"`
	AsianLow        float64       `json:"asian_low,omitempty"`
	AsianRangeValid bool          `json:"asian_range_valid"`
}
