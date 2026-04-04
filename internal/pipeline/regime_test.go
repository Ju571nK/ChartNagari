package pipeline

import (
	"testing"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// makeRegimeBars builds a slice of OHLCV bars in DESC order (index 0 = newest).
// Each bar i (0-indexed from oldest) has:
//
//	High  = baseHigh + float64(i) * highStep
//	Low   = baseLow  + float64(i) * lowStep
//	Close = baseLow  + float64(i) * lowStep  (simplified)
//
// The resulting slice is reversed so index 0 is the newest bar.
func makeRegimeBars(n int, baseHigh, highStep, baseLow, lowStep float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	for i := 0; i < n; i++ {
		bars[i] = models.OHLCV{
			OpenTime: time.Now().Add(-time.Duration(n-i) * time.Hour),
			High:     baseHigh + float64(i)*highStep,
			Low:      baseLow + float64(i)*lowStep,
			Close:    baseLow + float64(i)*lowStep,
			Open:     baseLow + float64(i)*lowStep,
		}
	}
	// Reverse so index 0 = newest (DESC order expected by the pipeline).
	for l, r := 0, n-1; l < r; l, r = l+1, r-1 {
		bars[l], bars[r] = bars[r], bars[l]
	}
	return bars
}

// ----------------------------------------------------------------------------
// atrPercentile tests
// ----------------------------------------------------------------------------

func TestATRPercentile_HighVol(t *testing.T) {
	// Bars with small, stable ranges — so the rolling ATR history is ~1.0.
	// We then supply a currentATR much larger than that history.
	bars := makeRegimeBars(90, 101.0, 1.0, 99.0, 1.0) // High-Low spread ≈ 2 each bar

	// A very large currentATR should land at or near the 100th percentile.
	pct := atrPercentile(bars, 9999.0, 14, 90)
	if pct < 0 {
		t.Fatalf("expected valid percentile, got -1 (insufficient data)")
	}
	if pct < 90.0 {
		t.Errorf("expected percentile >= 90 for very high ATR, got %.2f", pct)
	}
}

func TestATRPercentile_LowVol(t *testing.T) {
	// Bars with stable ranges — rolling ATR history will be > 0.
	// Supply a currentATR of 0.0001 (essentially zero).
	bars := makeRegimeBars(90, 101.0, 1.0, 99.0, 1.0)

	pct := atrPercentile(bars, 0.0001, 14, 90)
	if pct < 0 {
		t.Fatalf("expected valid percentile, got -1 (insufficient data)")
	}
	if pct > 10.0 {
		t.Errorf("expected percentile <= 10 for near-zero ATR, got %.2f", pct)
	}
}

func TestATRPercentile_InsufficientData(t *testing.T) {
	// Fewer than 90 bars → should return -1.
	bars := makeRegimeBars(50, 101.0, 1.0, 99.0, 1.0)
	pct := atrPercentile(bars, 5.0, 14, 90)
	if pct != -1 {
		t.Errorf("expected -1 for insufficient data, got %.2f", pct)
	}
}

// ----------------------------------------------------------------------------
// applyVolatilityRegime tests
// ----------------------------------------------------------------------------

func TestApplyVolatilityRegime_HighVolBonus(t *testing.T) {
	// Build bars so rolling ATR ≈ small; supply currentATR that lands above p75.
	bars := makeRegimeBars(90, 101.0, 0.5, 99.0, 0.5)
	tf := "1H"

	signals := []models.Signal{
		{Symbol: "AAPL", Timeframe: tf, Direction: "LONG", Score: 10.0},
	}

	indicators := map[string]float64{
		tf + ":ATR_14": 9999.0, // very high → HIGH_VOL
	}
	allBars := map[string][]models.OHLCV{tf: bars}

	cfg := appconfig.VolatilityRegimeConfig{
		LowVolPercentile:  25,
		HighVolPercentile: 75,
		LowVolPenaltyPct:  20,
		HighVolBonusPct:   15,
	}

	applyVolatilityRegime(signals, allBars, indicators, cfg)

	want := 10.0 * 1.15
	if got := signals[0].Score; got != want {
		t.Errorf("HIGH_VOL bonus: want %.4f, got %.4f", want, got)
	}
}

func TestApplyVolatilityRegime_LowVolPenalty(t *testing.T) {
	bars := makeRegimeBars(90, 101.0, 0.5, 99.0, 0.5)
	tf := "1H"

	signals := []models.Signal{
		{Symbol: "AAPL", Timeframe: tf, Direction: "LONG", Score: 10.0},
	}

	indicators := map[string]float64{
		tf + ":ATR_14": 0.0001, // near-zero → LOW_VOL
	}
	allBars := map[string][]models.OHLCV{tf: bars}

	cfg := appconfig.VolatilityRegimeConfig{
		LowVolPercentile:  25,
		HighVolPercentile: 75,
		LowVolPenaltyPct:  20,
		HighVolBonusPct:   15,
	}

	applyVolatilityRegime(signals, allBars, indicators, cfg)

	want := 10.0 * 0.80
	if got := signals[0].Score; got != want {
		t.Errorf("LOW_VOL penalty: want %.4f, got %.4f", want, got)
	}
}

func TestApplyVolatilityRegime_InsufficientBarsSkipped(t *testing.T) {
	// Only 50 bars — regime classification is skipped, score unchanged.
	bars := makeRegimeBars(50, 101.0, 0.5, 99.0, 0.5)
	tf := "1H"

	signals := []models.Signal{
		{Symbol: "AAPL", Timeframe: tf, Direction: "LONG", Score: 10.0},
	}
	indicators := map[string]float64{tf + ":ATR_14": 9999.0}
	allBars := map[string][]models.OHLCV{tf: bars}

	cfg := appconfig.VolatilityRegimeConfig{
		LowVolPercentile:  25,
		HighVolPercentile: 75,
		LowVolPenaltyPct:  20,
		HighVolBonusPct:   15,
	}

	applyVolatilityRegime(signals, allBars, indicators, cfg)

	if got := signals[0].Score; got != 10.0 {
		t.Errorf("expected score unchanged (10.0) when insufficient bars, got %.4f", got)
	}
}

// ----------------------------------------------------------------------------
// atrSlopeRising tests
// ----------------------------------------------------------------------------

func TestATRSlopeRising(t *testing.T) {
	// Monotonically increasing ATR history → EMA must be rising.
	history := make([]float64, 30)
	for i := range history {
		history[i] = float64(i+1) * 1.0 // 1, 2, 3, …, 30
	}

	if !atrSlopeRising(history, 20) {
		t.Error("expected atrSlopeRising=true for monotonically increasing ATR history")
	}
}

func TestATRSlopeFalling(t *testing.T) {
	// Monotonically decreasing ATR history → EMA must be falling.
	history := make([]float64, 30)
	for i := range history {
		history[i] = float64(30-i) * 1.0 // 30, 29, …, 1
	}

	if atrSlopeRising(history, 20) {
		t.Error("expected atrSlopeRising=false for monotonically decreasing ATR history")
	}
}

func TestATRSlopeRising_InsufficientData(t *testing.T) {
	// Fewer elements than emaPeriod+1 → always false.
	history := []float64{1.0, 2.0, 3.0}
	if atrSlopeRising(history, 20) {
		t.Error("expected atrSlopeRising=false when data < emaPeriod+1")
	}
}

// ----------------------------------------------------------------------------
// applyATRSlopeBonus tests
// ----------------------------------------------------------------------------

func TestApplyATRSlopeBonus_Rising(t *testing.T) {
	// Create 90 bars where High-Low spread increases each bar → ATR history
	// should also increase → slope is rising.
	bars := make([]models.OHLCV, 90)
	for i := 0; i < 90; i++ {
		spread := float64(i+1) * 0.1 // widening spread, oldest → newest (ASC)
		bars[i] = models.OHLCV{
			OpenTime: time.Now().Add(-time.Duration(90-i) * time.Hour),
			High:     100.0 + spread,
			Low:      100.0 - spread,
			Close:    100.0,
			Open:     100.0,
		}
	}
	// Reverse to DESC order (index 0 = newest).
	for l, r := 0, 89; l < r; l, r = l+1, r-1 {
		bars[l], bars[r] = bars[r], bars[l]
	}

	tf := "1H"
	signals := []models.Signal{
		{Symbol: "AAPL", Timeframe: tf, Direction: "LONG", Score: 10.0},
	}
	indicators := map[string]float64{tf + ":ATR_14": 0.5}
	allBars := map[string][]models.OHLCV{tf: bars}

	cfg := appconfig.ATRSlopeConfig{EMAPeriod: 20, RisingBonusPct: 10}

	applyATRSlopeBonus(signals, allBars, indicators, cfg)

	// Score should be boosted by 10 %.
	want := 10.0 * 1.10
	if got := signals[0].Score; got != want {
		t.Errorf("rising ATR slope bonus: want %.4f, got %.4f", want, got)
	}
}

func TestApplyATRSlopeBonus_InsufficientBarsSkipped(t *testing.T) {
	bars := makeRegimeBars(50, 101.0, 0.5, 99.0, 0.5)
	tf := "1H"

	signals := []models.Signal{
		{Symbol: "AAPL", Timeframe: tf, Direction: "LONG", Score: 10.0},
	}
	indicators := map[string]float64{tf + ":ATR_14": 0.5}
	allBars := map[string][]models.OHLCV{tf: bars}
	cfg := appconfig.ATRSlopeConfig{EMAPeriod: 20, RisingBonusPct: 10}

	applyATRSlopeBonus(signals, allBars, indicators, cfg)

	if got := signals[0].Score; got != 10.0 {
		t.Errorf("expected score unchanged (10.0) with insufficient bars, got %.4f", got)
	}
}
