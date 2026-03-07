package wyckoff

import (
	"testing"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// makeCtx creates a minimal AnalysisContext for testing.
func makeCtx(symbol string) models.AnalysisContext {
	return models.AnalysisContext{
		Symbol:     symbol,
		Timeframes: map[string][]models.OHLCV{},
		Indicators: map[string]float64{},
	}
}

// makeBar creates an OHLCV bar with the given values.
func makeBar(open, high, low, close_, vol float64) models.OHLCV {
	return models.OHLCV{Open: open, High: high, Low: low, Close: close_, Volume: vol}
}

// fillBars returns n identical bars.
func fillBars(n int, open, high, low, close_, vol float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	for i := range bars {
		bars[i] = makeBar(open, high, low, close_, vol)
	}
	return bars
}

// --- Accumulation tests ---

// TestAccumulation_Detected verifies that a tight range below EMA50 with low volume yields a LONG signal.
func TestAccumulation_Detected(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	// 20 bars with tight range: high=102, low=98, close=99 (below EMA50=110), low volume
	ctx.Timeframes["1H"] = fillBars(20, 100, 102, 98, 99, 500)
	ctx.Indicators["1H:EMA_50"] = 110.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffAccumulationRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Rule != "wyckoff_accumulation" {
		t.Errorf("expected rule wyckoff_accumulation, got %s", sig.Rule)
	}
}

// TestAccumulation_WideRange verifies that a wide range (>8%) yields no signal.
func TestAccumulation_WideRange(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	// range = (120-80)/100 = 40% — way above 8%
	ctx.Timeframes["1H"] = fillBars(20, 100, 120, 80, 90, 500)
	ctx.Indicators["1H:EMA_50"] = 110.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffAccumulationRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal, got %+v", sig)
	}
}

// TestAccumulation_AboveEMA50 verifies that price above EMA50 yields no signal.
func TestAccumulation_AboveEMA50(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	// tight range but close=115 > EMA50=110
	ctx.Timeframes["1H"] = fillBars(20, 114, 116, 113, 115, 500)
	ctx.Indicators["1H:EMA_50"] = 110.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffAccumulationRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal (above EMA50), got %+v", sig)
	}
}

// TestAccumulation_InsufficientBars verifies that fewer than 20 bars yields no signal.
func TestAccumulation_InsufficientBars(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	ctx.Timeframes["1H"] = fillBars(10, 100, 102, 98, 99, 500)
	ctx.Indicators["1H:EMA_50"] = 110.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffAccumulationRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal (insufficient bars), got %+v", sig)
	}
}

// --- Distribution tests ---

// TestDistribution_Detected verifies that a tight range above EMA50 with low volume yields a SHORT signal.
func TestDistribution_Detected(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	// tight range: high=122, low=118, close=121 (above EMA50=110), low volume
	ctx.Timeframes["1H"] = fillBars(20, 120, 122, 118, 121, 500)
	ctx.Indicators["1H:EMA_50"] = 110.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffDistributionRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
	if sig.Rule != "wyckoff_distribution" {
		t.Errorf("expected rule wyckoff_distribution, got %s", sig.Rule)
	}
}

// TestDistribution_BelowEMA50 verifies that price below EMA50 yields no signal.
func TestDistribution_BelowEMA50(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	// tight range but close=99 < EMA50=110
	ctx.Timeframes["1H"] = fillBars(20, 100, 102, 98, 99, 500)
	ctx.Indicators["1H:EMA_50"] = 110.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffDistributionRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal (below EMA50), got %+v", sig)
	}
}

// --- Spring tests ---

// TestSpring_Detected verifies a valid spring: previous bar dipped below SWING_LOW, current closes above, high volume.
func TestSpring_Detected(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	// bars[0..3]: normal bars; bars[3] dips below swing_low=100; bars[4] (current) closes above with high volume
	bars := []models.OHLCV{
		makeBar(105, 108, 103, 106, 1000), // bars[len-5]
		makeBar(104, 107, 102, 105, 1000), // bars[len-4]
		makeBar(103, 106, 101, 104, 1000), // bars[len-3]
		makeBar(102, 105, 98, 103, 1000),  // bars[len-2]: Low=98 < SWING_LOW=100
		makeBar(103, 106, 101, 104, 2000), // bars[len-1]: current, Close=104 > 100, vol=2000 >= 1.5*1000
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffSpringRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected spring signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
}

// TestSpring_NoDip verifies that without any bar dipping below SWING_LOW, no signal is returned.
func TestSpring_NoDip(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	bars := []models.OHLCV{
		makeBar(105, 108, 102, 106, 1000),
		makeBar(104, 107, 101, 105, 1000),
		makeBar(103, 106, 101, 104, 1000),
		makeBar(102, 105, 101, 103, 1000), // Low=101 > SWING_LOW=100
		makeBar(103, 106, 101, 104, 2000),
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffSpringRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil (no dip), got %+v", sig)
	}
}

// TestSpring_NoVolumeConfirmation verifies that a dip without sufficient volume yields no signal.
func TestSpring_NoVolumeConfirmation(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	bars := []models.OHLCV{
		makeBar(105, 108, 103, 106, 1000),
		makeBar(104, 107, 102, 105, 1000),
		makeBar(103, 106, 101, 104, 1000),
		makeBar(102, 105, 98, 103, 1000), // dip below 100
		makeBar(103, 106, 101, 104, 1200), // volume=1200 < 1.5*1000=1500
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffSpringRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil (low volume), got %+v", sig)
	}
}

// --- Upthrust tests ---

// TestUpthrust_Detected verifies a valid upthrust: previous bar pierced above SWING_HIGH, current closes below, high volume.
func TestUpthrust_Detected(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	bars := []models.OHLCV{
		makeBar(95, 99, 93, 97, 1000),
		makeBar(96, 99, 94, 98, 1000),
		makeBar(97, 99, 95, 98, 1000),
		makeBar(98, 102, 96, 99, 1000), // High=102 > SWING_HIGH=100
		makeBar(99, 100, 96, 97, 2000), // current: Close=97 < 100, vol=2000 >= 1.5*1000
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_HIGH"] = 100.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffUpthrustRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected upthrust signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
}

// TestUpthrust_NoPierce verifies that without any bar piercing above SWING_HIGH, no signal is returned.
func TestUpthrust_NoPierce(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	bars := []models.OHLCV{
		makeBar(95, 99, 93, 97, 1000),
		makeBar(96, 99, 94, 98, 1000),
		makeBar(97, 99, 95, 98, 1000),
		makeBar(98, 99, 96, 97, 1000), // High=99 < SWING_HIGH=100
		makeBar(99, 99, 96, 97, 2000),
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_HIGH"] = 100.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffUpthrustRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil (no pierce), got %+v", sig)
	}
}

// --- Volume Anomaly tests ---

// TestVolumeAnomaly_Bullish verifies that 3x volume with close > open yields LONG.
func TestVolumeAnomaly_Bullish(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	ctx.Timeframes["1H"] = []models.OHLCV{
		makeBar(100, 110, 99, 109, 3000), // close > open → bullish
	}
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffVolumeAnomalyRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
}

// TestVolumeAnomaly_Bearish verifies that 3x volume with close < open yields SHORT.
func TestVolumeAnomaly_Bearish(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	ctx.Timeframes["1H"] = []models.OHLCV{
		makeBar(110, 111, 99, 101, 3000), // close < open → bearish
	}
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffVolumeAnomalyRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
}

// TestVolumeAnomaly_Normal verifies that volume below 2.5x threshold yields no signal.
func TestVolumeAnomaly_Normal(t *testing.T) {
	ctx := makeCtx("BTCUSDT")
	ctx.Timeframes["1H"] = []models.OHLCV{
		makeBar(100, 110, 99, 109, 2000), // 2x MA — below 2.5 threshold
	}
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	rule := &WyckoffVolumeAnomalyRule{}
	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil (normal volume), got %+v", sig)
	}
}
