package forex

import (
	"github.com/Ju571nK/Chatter/pkg/models"
	"math"
	"testing"
	"time"
)

func bars(prices []float64) []models.OHLCV {
	out := make([]models.OHLCV, len(prices))
	for i, p := range prices {
		out[i] = models.OHLCV{OpenTime: time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC), Close: p}
	}
	return out
}

func TestStrengthCoverageAndRank(t *testing.T) {
	one := Strength(map[string][]models.OHLCV{"EURUSD": bars([]float64{1, 1.1})}, 1)
	if len(one) != 2 || one[0].Value != nil || one[0].Coverage != "low" {
		t.Fatalf("low coverage: %+v", one)
	}
	got := Strength(map[string][]models.OHLCV{"EURUSD": bars([]float64{1, 1.1}), "GBPUSD": bars([]float64{1, 1.05}), "EURGBP": bars([]float64{1, 1.04})}, 1)
	if len(got) != 3 || got[0].Currency != "EUR" || got[0].Coverage != "ok" || got[2].Currency != "USD" {
		t.Fatalf("ranking: %+v", got)
	}
	for _, s := range got {
		if s.Value == nil || math.Abs(*s.Value) > 100.0001 {
			t.Fatalf("normalization: %+v", got)
		}
	}
}

func TestCorrelationAlignedReturns(t *testing.T) {
	prices := []float64{1, 1.1, 1.03, 1.13, 1.06, 1.16}
	pair := bars(prices)
	dxy := bars([]float64{2, 1.9, 2.0, 1.8, 1.95, 1.7})
	if corr := Correlation(pair, dxy, 5); corr == nil || *corr > -.8 {
		t.Fatalf("correlation=%v", corr)
	}
	if corr := Correlation(pair, dxy, 6); corr != nil {
		t.Fatalf("insufficient=%v", *corr)
	}
	constant := bars([]float64{1, 1, 1, 1, 1, 1})
	if corr := Correlation(pair, constant, 5); corr != nil {
		t.Fatalf("constant=%v", *corr)
	}
	// Remove one daily observation: the calculation must align dates, not indices.
	dxy = append(dxy[:2], dxy[3:]...)
	if corr := Correlation(pair, dxy, 5); corr != nil {
		t.Fatalf("misaligned=%v", *corr)
	}
}

func TestCorrelationOANDANYCloseTradingDatesAcrossDST(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	for _, monday := range []string{"2026-03-09", "2026-11-02"} {
		date, _ := time.ParseInLocation("2006-01-02", monday, ny)
		pair, dxy := make([]models.OHLCV, 0, 4), make([]models.OHLCV, 0, 4)
		for i, value := range []float64{1, 2, 3, 4} {
			closeDay := date.AddDate(0, 0, i)
			open := closeDay.AddDate(0, 0, -1).Add(17 * time.Hour)
			pair = append(pair, models.OHLCV{OpenTime: open.UTC(), Close: value, Source: "oanda"})
			dxy = append(dxy, models.OHLCV{OpenTime: time.Date(closeDay.Year(), closeDay.Month(), closeDay.Day(), 0, 0, 0, 0, time.UTC), Close: 100 / value, Source: "yahoo"})
		}
		if got := TradingDate(pair[0]); got != monday {
			t.Fatalf("%s mapped to %s", monday, got)
		}
		if corr := Correlation(pair, dxy, 3); corr == nil || *corr > -0.9 {
			t.Fatalf("%s correlation=%v", monday, corr)
		}
	}
}
