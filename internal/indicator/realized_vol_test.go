package indicator

import (
	"math"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestRealizedVol_Uptrend(t *testing.T) {
	// Steady 0.5% daily increase → low realized vol.
	closes := make([]float64, 25)
	closes[0] = 100
	for i := 1; i < len(closes); i++ {
		closes[i] = closes[i-1] * 1.005
	}

	rv, ok := realizedVol(closes, 20)
	if !ok {
		t.Fatal("expected ok=true for sufficient data")
	}
	// Constant daily return → very small stddev, annualized ~0
	// With 0.5% daily, log return stddev should be ~0 since returns are constant.
	// In practice the stddev is near 0, so annualized vol should be very small.
	if rv > 5.0 {
		t.Errorf("expected low vol for steady uptrend, got %f", rv)
	}
	if rv < 0 {
		t.Errorf("realized vol should be non-negative, got %f", rv)
	}
}

func TestRealizedVol_HighVolatility(t *testing.T) {
	// Alternating +5%/-5% → high realized vol.
	closes := make([]float64, 25)
	closes[0] = 100
	for i := 1; i < len(closes); i++ {
		if i%2 == 1 {
			closes[i] = closes[i-1] * 1.05
		} else {
			closes[i] = closes[i-1] * 0.95
		}
	}

	rv, ok := realizedVol(closes, 20)
	if !ok {
		t.Fatal("expected ok=true for sufficient data")
	}
	// +5%/-5% swings → high annualized vol (should be > 50%)
	if rv < 50.0 {
		t.Errorf("expected high vol for alternating swings, got %f", rv)
	}
}

func TestRealizedVol_InsufficientData(t *testing.T) {
	// Only 10 closes but period=20 → need 21, should fail.
	closes := make([]float64, 10)
	for i := range closes {
		closes[i] = 100 + float64(i)
	}

	_, ok := realizedVol(closes, 20)
	if ok {
		t.Error("expected ok=false for insufficient data")
	}
}

func TestRealizedVol_ZeroPeriod(t *testing.T) {
	closes := []float64{100, 101, 102}
	_, ok := realizedVol(closes, 0)
	if ok {
		t.Error("expected ok=false for zero period")
	}
}

func TestRealizedVol_ZeroPrices(t *testing.T) {
	closes := make([]float64, 25)
	// A zero price in the window should return false.
	for i := range closes {
		closes[i] = 100 + float64(i)
	}
	closes[5] = 0 // zero price in the computation window

	_, ok := realizedVol(closes, 20)
	if ok {
		t.Error("expected ok=false when a close price is zero")
	}
}

func TestRealizedVol_Integration(t *testing.T) {
	// Verify that Compute() produces REALIZED_VOL_20 key.
	closes := make([]float64, 30)
	closes[0] = 100
	for i := 1; i < len(closes); i++ {
		if i%2 == 1 {
			closes[i] = closes[i-1] * 1.02
		} else {
			closes[i] = closes[i-1] * 0.98
		}
	}

	bars := make([]models.OHLCV, len(closes))
	for i, c := range closes {
		bars[i] = models.OHLCV{
			Symbol:    "TEST",
			Timeframe: "1D",
			OpenTime:  time.Now().Add(time.Duration(i) * 24 * time.Hour),
			Open:      c,
			High:      c * 1.01,
			Low:       c * 0.99,
			Close:     c,
			Volume:    1000,
		}
	}

	result := Compute(map[string][]models.OHLCV{"1D": bars})

	key := "1D:REALIZED_VOL_20"
	val, exists := result[key]
	if !exists {
		t.Fatalf("expected key %q in Compute result", key)
	}
	if math.IsNaN(val) || val <= 0 {
		t.Errorf("expected positive REALIZED_VOL_20, got %f", val)
	}
}
