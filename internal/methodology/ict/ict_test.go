package ict

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func makeCtx(symbol string) models.AnalysisContext {
	return models.AnalysisContext{
		Symbol:     symbol,
		Timeframes: map[string][]models.OHLCV{},
		Indicators: map[string]float64{},
	}
}

// makeBar is a convenience helper for building OHLCV bars.
func makeBar(open, high, low, close float64) models.OHLCV {
	return models.OHLCV{
		Symbol:    "TEST",
		Timeframe: "1H",
		OpenTime:  time.Now(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     close,
		Volume:    1000,
	}
}

// ── Order Block ──────────────────────────────────────────────────────────────

// TestOrderBlock_Bullish: bearish candle → impulse up → price returns to OB → LONG
func TestOrderBlock_Bullish(t *testing.T) {
	rule := &ICTOrderBlockRule{}
	ctx := makeCtx("AAPL")

	// Build bars:
	// bars[0]: bearish OB candle (open=110, close=100 → bearish), high=112, low=99
	// bars[1]: bullish impulse start
	// bars[2]: close > bars[0].open = 110  → impulse confirmed
	// bars[3]: current bar — close within [99, 112]
	bars := []models.OHLCV{
		makeBar(110, 112, 99, 100), // index 0: bearish OB
		makeBar(101, 115, 100, 114), // index 1: bullish
		makeBar(114, 120, 113, 118), // index 2: close(118) > OB.open(110) → impulse
		makeBar(105, 111, 100, 105), // index 3: current, close=105 inside [99,112]
	}
	// We need at least 5 bars; pad with neutral bars at the front.
	padding := []models.OHLCV{
		makeBar(90, 95, 88, 92),
		makeBar(92, 97, 91, 95),
	}
	bars = append(padding, bars...)

	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Rule != "ict_order_block" {
		t.Errorf("wrong rule name: %s", sig.Rule)
	}
	if sig.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", sig.Score)
	}
}

// TestOrderBlock_Bearish: bullish candle → impulse down → price returns to OB → SHORT
func TestOrderBlock_Bearish(t *testing.T) {
	rule := &ICTOrderBlockRule{}
	ctx := makeCtx("AAPL")

	// bars[0]: bullish OB candle (open=100, close=110), high=112, low=99
	// bars[1]: bearish impulse start
	// bars[2]: close < bars[0].open = 100 → impulse confirmed
	// bars[3]: current bar — close within [99, 112]
	bars := []models.OHLCV{
		makeBar(100, 112, 99, 110), // index 0: bullish OB
		makeBar(109, 110, 95, 98),  // index 1: bearish
		makeBar(98, 99, 88, 90),    // index 2: close(90) < OB.open(100) → impulse
		makeBar(108, 111, 100, 105), // index 3: current, close=105 inside [99,112]
	}
	padding := []models.OHLCV{
		makeBar(90, 95, 88, 92),
		makeBar(92, 97, 91, 95),
	}
	bars = append(padding, bars...)

	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
}

// TestOrderBlock_NoPattern: no OB pattern → nil
func TestOrderBlock_NoPattern(t *testing.T) {
	rule := &ICTOrderBlockRule{}
	ctx := makeCtx("AAPL")

	// All bars are bullish and price is outside any OB zone
	bars := []models.OHLCV{
		makeBar(100, 105, 99, 104),
		makeBar(104, 108, 103, 107),
		makeBar(107, 112, 106, 111),
		makeBar(111, 115, 110, 114),
		makeBar(114, 120, 113, 119), // current: no pattern triggers
	}
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal, got %+v", sig)
	}
}

// TestOrderBlock_InsufficientBars: < 5 bars → nil
func TestOrderBlock_InsufficientBars(t *testing.T) {
	rule := &ICTOrderBlockRule{}
	ctx := makeCtx("AAPL")

	bars := []models.OHLCV{
		makeBar(100, 105, 99, 104),
		makeBar(104, 108, 103, 102),
		makeBar(102, 106, 101, 105),
	}
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for insufficient bars, got %+v", sig)
	}
}

// ── Fair Value Gap ────────────────────────────────────────────────────────────

// TestFVG_Bullish: bars[i].high < bars[i+2].low, price in gap → LONG
func TestFVG_Bullish(t *testing.T) {
	rule := &ICTFairValueGapRule{}
	ctx := makeCtx("BTCUSDT")

	// Pattern: bars[0].high=100, bars[2].low=105 → gap [100,105]
	// current close = 102 → inside gap → LONG
	bars := []models.OHLCV{
		makeBar(95, 100, 93, 99),   // index 0: high=100
		makeBar(101, 104, 100, 103), // index 1: middle candle
		makeBar(104, 108, 105, 107), // index 2: low=105 > high_0(100) → bullish FVG
		makeBar(103, 106, 101, 102), // index 3: current, close=102 in [100,105]
	}
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Rule != "ict_fair_value_gap" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
}

// TestFVG_Bearish: bars[i].low > bars[i+2].high, price in gap → SHORT
func TestFVG_Bearish(t *testing.T) {
	rule := &ICTFairValueGapRule{}
	ctx := makeCtx("BTCUSDT")

	// Pattern: bars[0].low=105, bars[2].high=100 → gap [100,105] (bearish FVG)
	// current close = 102 → inside gap → SHORT
	bars := []models.OHLCV{
		makeBar(110, 112, 105, 106), // index 0: low=105
		makeBar(104, 105, 101, 102), // index 1: middle
		makeBar(99, 100, 97, 98),   // index 2: high=100 < low_0(105) → bearish FVG
		makeBar(103, 104, 101, 102), // index 3: current, close=102 in [100,105]
	}
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
}

// TestFVG_NoGap: no FVG pattern → nil
func TestFVG_NoGap(t *testing.T) {
	rule := &ICTFairValueGapRule{}
	ctx := makeCtx("BTCUSDT")

	// Overlapping bars — no gap
	bars := []models.OHLCV{
		makeBar(100, 110, 99, 105),
		makeBar(104, 112, 103, 108),
		makeBar(107, 115, 106, 111),
		makeBar(110, 114, 109, 113), // current: no gap condition
	}
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil, got %+v", sig)
	}
}

// ── Liquidity Sweep ───────────────────────────────────────────────────────────

// TestLiquiditySweep_Bullish: curr.Low < SWING_LOW && curr.Close > SWING_LOW → LONG
func TestLiquiditySweep_Bullish(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("AAPL")

	// SWING_LOW = 100; current bar: low=98 (swept below), close=103 (recovered)
	bars := []models.OHLCV{
		makeBar(102, 105, 98, 103), // current bar
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 115.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Rule != "ict_liquidity_sweep" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
}

// TestLiquiditySweep_Bearish: curr.High > SWING_HIGH && curr.Close < SWING_HIGH → SHORT
func TestLiquiditySweep_Bearish(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("AAPL")

	// SWING_HIGH = 115; current bar: high=118 (swept above), close=112 (reversed)
	bars := []models.OHLCV{
		makeBar(113, 118, 111, 112), // current bar
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 115.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
}

// TestLiquiditySweep_NoSweep: no sweep → nil
func TestLiquiditySweep_NoSweep(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("AAPL")

	// Current bar stays well within swing range
	bars := []models.OHLCV{
		makeBar(105, 110, 103, 107), // low=103 > SWING_LOW=100, high=110 < SWING_HIGH=115
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 115.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil, got %+v", sig)
	}
}

// ── Breaker Block ─────────────────────────────────────────────────────────────

// TestBreakerBlock_Bullish: prev bars closed below SWING_LOW, current above → LONG
func TestBreakerBlock_Bullish(t *testing.T) {
	rule := &ICTBreakerBlockRule{}
	ctx := makeCtx("AAPL")

	// SWING_LOW = 100
	// bars[0..4]: some of them closed below 100
	// bars[5] (current): close=105 > SWING_LOW=100
	bars := []models.OHLCV{
		makeBar(102, 105, 99, 101),  // index 0: close > SWING_LOW
		makeBar(101, 103, 97, 98),   // index 1: close=98 < SWING_LOW=100
		makeBar(99, 101, 96, 97),    // index 2: close=97 < SWING_LOW
		makeBar(97, 100, 95, 99),    // index 3: close=99 < SWING_LOW
		makeBar(99, 102, 98, 101),   // index 4: close=101 > SWING_LOW
		makeBar(101, 108, 100, 105), // index 5 (current): close=105 > SWING_LOW
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 120.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Rule != "ict_breaker_block" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
}

// TestBreakerBlock_Bearish: prev bars closed above SWING_HIGH, current below → SHORT
func TestBreakerBlock_Bearish(t *testing.T) {
	rule := &ICTBreakerBlockRule{}
	ctx := makeCtx("AAPL")

	// SWING_HIGH = 120
	// Some prior bars closed above 120, current bar is back below
	bars := []models.OHLCV{
		makeBar(118, 122, 117, 119), // index 0: close < SWING_HIGH
		makeBar(119, 125, 118, 123), // index 1: close=123 > SWING_HIGH=120
		makeBar(122, 126, 121, 124), // index 2: close=124 > SWING_HIGH
		makeBar(123, 125, 120, 121), // index 3: close=121 > SWING_HIGH
		makeBar(120, 122, 118, 119), // index 4: close=119 < SWING_HIGH
		makeBar(119, 121, 116, 117), // index 5 (current): close=117 < SWING_HIGH=120
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 120.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
}

// ── Kill Zone ─────────────────────────────────────────────────────────────────

// TestKillZone_London: 09:30 UTC → NEUTRAL signal (London session)
func TestKillZone_London(t *testing.T) {
	rule := &ICTKillZoneRule{now: func() time.Time {
		return time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)
	}}
	ctx := makeCtx("AAPL")

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected NEUTRAL signal for London kill zone, got nil")
	}
	if sig.Direction != "NEUTRAL" {
		t.Errorf("expected NEUTRAL, got %s", sig.Direction)
	}
	if sig.Timeframe != "ALL" {
		t.Errorf("expected timeframe ALL, got %s", sig.Timeframe)
	}
	if sig.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", sig.Score)
	}
	if sig.Rule != "ict_kill_zone" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
}

// TestKillZone_NewYork: 14:00 UTC → NEUTRAL signal (New York session)
func TestKillZone_NewYork(t *testing.T) {
	rule := &ICTKillZoneRule{now: func() time.Time {
		return time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC)
	}}
	ctx := makeCtx("AAPL")

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected NEUTRAL signal for New York kill zone, got nil")
	}
	if sig.Direction != "NEUTRAL" {
		t.Errorf("expected NEUTRAL, got %s", sig.Direction)
	}
	if sig.Timeframe != "ALL" {
		t.Errorf("expected timeframe ALL, got %s", sig.Timeframe)
	}
}

// TestKillZone_Outside: 20:00 UTC → nil (outside all kill zones)
func TestKillZone_Outside(t *testing.T) {
	rule := &ICTKillZoneRule{now: func() time.Time {
		return time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	}}
	ctx := makeCtx("AAPL")

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil outside kill zones, got %+v", sig)
	}
}
