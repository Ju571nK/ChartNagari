package indicator

import (
	"testing"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestADX_InsufficientData(t *testing.T) {
	// Need 2*period+1 = 29 bars for period=14
	highs := make([]float64, 20)
	lows := make([]float64, 20)
	closes := make([]float64, 20)

	_, ok := adx(highs, lows, closes, 14)
	if ok {
		t.Error("expected false for insufficient data (20 bars, need 29)")
	}
}

func TestADX_ZeroPeriod(t *testing.T) {
	_, ok := adx([]float64{1}, []float64{1}, []float64{1}, 0)
	if ok {
		t.Error("expected false for zero period")
	}
}

func TestADX_NegativePeriod(t *testing.T) {
	_, ok := adx([]float64{1}, []float64{1}, []float64{1}, -5)
	if ok {
		t.Error("expected false for negative period")
	}
}

func TestADX_MismatchedLengths(t *testing.T) {
	highs := make([]float64, 30)
	lows := make([]float64, 25) // shorter
	closes := make([]float64, 30)

	_, ok := adx(highs, lows, closes, 14)
	if ok {
		t.Error("expected false for mismatched array lengths")
	}
}

func TestADX_StrongUptrend(t *testing.T) {
	// Generate 50 bars of steady uptrend — ADX should be high
	n := 50
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)

	for i := 0; i < n; i++ {
		base := 100.0 + float64(i)*2.0 // steady rise
		highs[i] = base + 1.5
		lows[i] = base - 0.5
		closes[i] = base + 1.0
	}

	val, ok := adx(highs, lows, closes, 14)
	if !ok {
		t.Fatal("expected ok=true for 50-bar uptrend")
	}
	if val < 25 {
		t.Errorf("expected ADX > 25 for strong uptrend, got %.2f", val)
	}
}

func TestADX_StrongDowntrend(t *testing.T) {
	// Generate 50 bars of steady downtrend — ADX should be high
	n := 50
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)

	for i := 0; i < n; i++ {
		base := 200.0 - float64(i)*2.0 // steady decline
		highs[i] = base + 0.5
		lows[i] = base - 1.5
		closes[i] = base - 1.0
	}

	val, ok := adx(highs, lows, closes, 14)
	if !ok {
		t.Fatal("expected ok=true for 50-bar downtrend")
	}
	if val < 25 {
		t.Errorf("expected ADX > 25 for strong downtrend, got %.2f", val)
	}
}

func TestADX_FlatMarket(t *testing.T) {
	// Generate 50 bars of flat/ranging — ADX should be low
	n := 50
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)

	for i := 0; i < n; i++ {
		// Oscillate in a tight range
		offset := float64(i%4-2) * 0.3
		base := 100.0 + offset
		highs[i] = base + 0.5
		lows[i] = base - 0.5
		closes[i] = base
	}

	val, ok := adx(highs, lows, closes, 14)
	if !ok {
		t.Fatal("expected ok=true for 50-bar flat market")
	}
	if val > 25 {
		t.Errorf("expected ADX < 25 for flat/ranging market, got %.2f", val)
	}
}

func TestADX_OutputRange(t *testing.T) {
	// ADX should always be 0-100
	n := 60
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)

	for i := 0; i < n; i++ {
		base := 100.0 + float64(i)*1.5
		highs[i] = base + 2.0
		lows[i] = base - 1.0
		closes[i] = base + 0.5
	}

	val, ok := adx(highs, lows, closes, 14)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val < 0 || val > 100 {
		t.Errorf("ADX should be 0-100, got %.2f", val)
	}
}

func TestADX_MinimumBars(t *testing.T) {
	// Exactly 2*14+1 = 29 bars — should work
	n := 29
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)

	for i := 0; i < n; i++ {
		base := 100.0 + float64(i)*0.5
		highs[i] = base + 1.0
		lows[i] = base - 1.0
		closes[i] = base
	}

	_, ok := adx(highs, lows, closes, 14)
	if !ok {
		t.Error("expected ok=true with exactly 29 bars (minimum for period=14)")
	}
}

func TestADX_Integration(t *testing.T) {
	// Verify ADX_14 appears in Compute() output with enough bars
	candles := make([]models.OHLCV, 50)
	for i := 0; i < 50; i++ {
		base := 100.0 + float64(i)*1.0
		candles[i] = models.OHLCV{
			Open:   base,
			High:   base + 1.5,
			Low:    base - 0.5,
			Close:  base + 1.0,
			Volume: 1000,
		}
	}

	result := Compute(map[string][]models.OHLCV{"1D": candles})
	if _, exists := result["1D:ADX_14"]; !exists {
		t.Fatal("expected 1D:ADX_14 key in Compute() output")
	}
}
