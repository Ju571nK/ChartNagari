package market

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestSessionsDST(t *testing.T) {
	for _, tc := range []struct {
		name      string
		date      string
		asiaUTC   string
		londonUTC string
		nyUTC     string
	}{
		{"winter", "2026-01-15T12:00:00Z", "2026-01-15T01:00:00Z", "2026-01-15T07:00:00Z", "2026-01-15T12:00:00Z"},
		{"summer", "2026-07-15T12:00:00Z", "2026-07-15T00:00:00Z", "2026-07-15T06:00:00Z", "2026-07-15T11:00:00Z"},
		{"spring transition", "2026-03-08T12:00:00Z", "2026-03-08T01:00:00Z", "2026-03-08T07:00:00Z", "2026-03-08T11:00:00Z"},
		{"autumn transition", "2026-11-01T12:00:00Z", "2026-11-01T00:00:00Z", "2026-11-01T07:00:00Z", "2026-11-01T12:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			date, _ := time.Parse(time.RFC3339, tc.date)
			s := Sessions(date)
			for _, check := range []struct {
				got  time.Time
				want string
			}{{s.Asia.Open, tc.asiaUTC}, {s.London.Open, tc.londonUTC}, {s.NewYork.Open, tc.nyUTC}} {
				want, _ := time.Parse(time.RFC3339, check.want)
				if !check.got.Equal(want) {
					t.Errorf("got %s, want %s", check.got, want)
				}
			}
		})
	}
}

func TestAsianRange(t *testing.T) {
	date, _ := time.Parse(time.RFC3339, "2026-07-15T12:00:00Z")
	s := Sessions(date)
	bars := []models.OHLCV{
		{OpenTime: s.Asia.Open.Add(-time.Hour), High: 200, Low: 1},
		{OpenTime: s.Asia.Open, High: 1.15, Low: 1.1},
		{OpenTime: s.Asia.Close.Add(-time.Hour), High: 1.2, Low: 1.05},
		{OpenTime: s.Asia.Close, High: 300, Low: 0},
	}
	got := AsianRange(bars, date)
	if !got.AsianRangeValid || got.AsianHigh != 1.2 || got.AsianLow != 1.05 {
		t.Fatalf("unexpected Asian range: %+v", got)
	}
}

func TestSessionsAtEveningAsia(t *testing.T) {
	instant, _ := time.Parse(time.RFC3339, "2026-07-15T00:30:00Z") // Jul 14 20:30 NY
	s := SessionsAt(instant)
	if !s.Asia.Contains(instant) {
		t.Fatalf("evening Asia should contain %s: %+v", instant, s)
	}
	bars := []models.OHLCV{{OpenTime: s.Asia.Open, High: 1.2, Low: 1.1}, {OpenTime: s.Asia.Open.Add(time.Hour), High: 1.4, Low: 1.0}}
	partial := AsianRange(bars[:1], instant)
	if !partial.AsianRangeValid || partial.AsianHigh != 1.2 {
		t.Fatalf("future bar leaked into range: %+v", partial)
	}
}
