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

// TestOrderBlock_Mitigated: OB zone already revisited and closed through → nil
func TestOrderBlock_Mitigated(t *testing.T) {
	rule := &ICTOrderBlockRule{}
	ctx := makeCtx("AAPL")

	// bars[0]: bearish OB candle
	// bars[1]: bullish impulse
	// bars[2]: impulse confirmed (close > b0.open)
	// bars[3]: mitigating bar — closes BELOW OB low (mitigated)
	// bars[4]: current — close within OB range, but OB is mitigated
	bars := []models.OHLCV{
		makeBar(90, 95, 88, 92),    // padding
		makeBar(92, 97, 91, 95),    // padding
		makeBar(110, 112, 99, 100), // index 2: bearish OB (open=110, close=100), low=99, high=112
		makeBar(101, 115, 100, 114), // index 3: bullish
		makeBar(114, 120, 113, 118), // index 4: close(118) > OB.open(110) → impulse
		makeBar(95, 100, 90, 92),   // index 5: mitigating bar — close=92 < obLow=99 → mitigated
		makeBar(105, 111, 100, 105), // index 6: current, close=105 inside [99,112]
	}
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for mitigated OB, got %+v", sig)
	}
}

// TestOrderBlock_ImpulseWeak: OB with weak impulse (< 1.5x ATR) → nil
func TestOrderBlock_ImpulseWeak(t *testing.T) {
	rule := &ICTOrderBlockRule{}
	ctx := makeCtx("AAPL")

	// ATR = 20. Combined body of b1+b2 must be >= 30 (1.5*20).
	// b1 body = |114-101| = 13, b2 body = |118-114| = 4, combined = 17 < 30 → weak
	bars := []models.OHLCV{
		makeBar(90, 95, 88, 92),     // padding
		makeBar(92, 97, 91, 95),     // padding
		makeBar(110, 112, 99, 100),  // bearish OB
		makeBar(101, 115, 100, 114), // bullish (body=13)
		makeBar(114, 120, 113, 118), // impulse confirmed (body=4), combined=17
		makeBar(105, 111, 100, 105), // current
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 20.0 // 1.5 * 20 = 30, combined body 17 < 30

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for weak impulse OB, got %+v", sig)
	}
}

// TestOrderBlock_StrongImpulseWithATR: OB with strong impulse → signal
func TestOrderBlock_StrongImpulseWithATR(t *testing.T) {
	rule := &ICTOrderBlockRule{}
	ctx := makeCtx("AAPL")

	// ATR = 5. Combined body of b1+b2 must be >= 7.5 (1.5*5).
	// b1 body = |114-101| = 13, b2 body = |118-114| = 4, combined = 17 >= 7.5 → strong
	bars := []models.OHLCV{
		makeBar(90, 95, 88, 92),     // padding
		makeBar(92, 97, 91, 95),     // padding
		makeBar(110, 112, 99, 100),  // bearish OB
		makeBar(101, 115, 100, 114), // bullish (body=13)
		makeBar(114, 120, 113, 118), // impulse confirmed (body=4), combined=17
		makeBar(105, 111, 100, 105), // current, close=105 in [99,112]
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 5.0 // 1.5 * 5 = 7.5, combined body 17 >= 7.5

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

// TestFVGRelevance_Unit: direct unit test of fvgRelevance function
func TestFVGRelevance_Unit(t *testing.T) {
	tests := []struct {
		name                                              string
		gapSize, atr, impulseBody, impulseVol, volMA float64
		unfilledBars                                      int
		minScore, maxScore                                float64
	}{
		{"no indicators → neutral", 5.0, 0, 3.0, 0, 0, 5, 0.3, 0.6},
		{"large gap + strong impulse + long unfilled", 10.0, 5.0, 12.0, 3000, 1000, 15, 0.7, 1.0},
		{"tiny gap + weak impulse + just formed", 0.5, 5.0, 0.2, 400, 1000, 0, 0.1, 0.2},
		{"medium everything", 3.0, 5.0, 4.0, 1500, 1000, 5, 0.3, 0.7},
		{"zero ATR → neutral gap score", 5.0, 0, 5.0, 2000, 1000, 8, 0.4, 0.8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := fvgRelevance(tt.gapSize, tt.atr, tt.impulseBody, tt.impulseVol, tt.volMA, tt.unfilledBars)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("fvgRelevance(%v, %v, %v, %v, %v, %v) = %f, want [%f, %f]",
					tt.gapSize, tt.atr, tt.impulseBody, tt.impulseVol, tt.volMA, tt.unfilledBars,
					score, tt.minScore, tt.maxScore)
			}
		})
	}
}

// TestFVG_HighRelevance: large gap, high volume impulse, unfilled for many bars → high score
func TestFVG_HighRelevance(t *testing.T) {
	rule := &ICTFairValueGapRule{}
	ctx := makeCtx("BTCUSDT")

	// Build bars with a bullish FVG that has large gap, strong impulse, and many unfilled bars
	// ATR=5, gap=8 (1.6x ATR), middle candle has huge body and volume
	bars := []models.OHLCV{
		makeBarWithVolume(95, 100, 93, 99, 100),    // index 0: high=100
		makeBarWithVolume(101, 112, 100, 111, 5000), // index 1: big impulse candle (body=10)
		makeBarWithVolume(110, 115, 108, 113, 200),  // index 2: low=108 > b0.high=100 → gap [100,108]
		// Several unfilled bars (close stays above gap)
		makeBarWithVolume(112, 114, 110, 113, 150),
		makeBarWithVolume(113, 116, 111, 115, 150),
		makeBarWithVolume(114, 117, 112, 116, 150),
		makeBarWithVolume(115, 118, 113, 117, 150),
		makeBarWithVolume(116, 119, 114, 118, 150),
		makeBarWithVolume(117, 120, 115, 119, 150),
		makeBarWithVolume(118, 121, 116, 120, 150),
		makeBarWithVolume(119, 122, 117, 121, 150),
		// Current bar: price enters gap
		makeBarWithVolume(108, 110, 101, 104, 200), // close=104 in [100,108]
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 5.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

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
	// High relevance: score should be >= 0.6
	if sig.Score < 0.6 {
		t.Errorf("expected high relevance score >= 0.6, got %f", sig.Score)
	}
}

// TestFVG_LowRelevance: tiny gap, low volume, just formed → low score
func TestFVG_LowRelevance(t *testing.T) {
	rule := &ICTFairValueGapRule{}
	ctx := makeCtx("BTCUSDT")

	// Tiny gap relative to ATR, weak impulse, just formed (0 unfilled bars)
	bars := []models.OHLCV{
		makeBarWithVolume(100, 100.5, 99, 100.3, 100), // index 0: high=100.5
		makeBarWithVolume(100.6, 101, 100.4, 100.8, 300), // index 1: weak impulse (body=0.2)
		makeBarWithVolume(100.8, 101.2, 100.7, 101, 100), // index 2: low=100.7 > b0.high=100.5 → gap [100.5, 100.7] = 0.2
		makeBarWithVolume(100.6, 100.8, 100.4, 100.6, 100), // current: close=100.6 in [100.5, 100.7]
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 5.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	// Low relevance: score should be < 0.4
	if sig.Score >= 0.4 {
		t.Errorf("expected low relevance score < 0.4, got %f", sig.Score)
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
	// Without VOLUME_MA_20, score should still be > 0
	if sig.Score <= 0 {
		t.Errorf("expected positive score, got %f", sig.Score)
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

// makeBarWithVolume is a helper to create bars with explicit volume for quality score tests.
func makeBarWithVolume(open, high, low, close, volume float64) models.OHLCV {
	return models.OHLCV{
		Symbol:    "TEST",
		Timeframe: "1H",
		OpenTime:  time.Now(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     close,
		Volume:    volume,
	}
}

// TestSweepQuality_HighQuality: high volume + long wick + strong reversal → high score
func TestSweepQuality_HighQuality(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("BTCUSDT")

	// SWING_LOW = 100. Bar sweeps to 95 (big wick), closes at 108 (strong reversal).
	// Range = 110 - 95 = 15. Wick beyond = 100-95=5. Reversal = 108-100=8.
	// Volume = 3000, MA = 1000 → ratio 3.0 (high).
	bars := []models.OHLCV{
		makeBarWithVolume(103, 110, 95, 108, 3000),
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 120.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

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
	// High quality sweep: score should be >= 0.7
	if sig.Score < 0.7 {
		t.Errorf("expected high quality score >= 0.7, got %f", sig.Score)
	}
}

// TestSweepQuality_LowQuality: low volume + tiny wick + weak reversal → low score
func TestSweepQuality_LowQuality(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("BTCUSDT")

	// SWING_LOW = 100. Bar barely sweeps to 99.5, closes at 100.2.
	// Range = 102 - 99.5 = 2.5. Wick beyond = 100-99.5=0.5. Reversal = 100.2-100=0.2.
	// Volume = 500, MA = 1000 → ratio 0.5 (low).
	bars := []models.OHLCV{
		makeBarWithVolume(101, 102, 99.5, 100.2, 500),
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 120.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	// Low quality: score should be < 0.4
	if sig.Score >= 0.4 {
		t.Errorf("expected low quality score < 0.4, got %f", sig.Score)
	}
}

// TestSweepQuality_BearishHighVolume: bearish sweep with high volume → higher score
func TestSweepQuality_BearishHighVolume(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("AAPL")

	// SWING_HIGH = 115. Bar pierces to 119, closes at 111.
	// Range = 119 - 110 = 9. Wick beyond = 119-115=4. Reversal = 115-111=4.
	// Volume = 2500, MA = 1000 → ratio 2.5.
	bars := []models.OHLCV{
		makeBarWithVolume(114, 119, 110, 111, 2500),
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 115.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

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
	// Good quality: score should be >= 0.6
	if sig.Score < 0.6 {
		t.Errorf("expected quality score >= 0.6, got %f", sig.Score)
	}
}

// TestSweepQuality_Unit: direct unit test of the sweepQuality function
func TestSweepQuality_Unit(t *testing.T) {
	tests := []struct {
		name                                       string
		volRatio, wickBeyond, reversalDist, cRange float64
		minScore, maxScore                          float64
	}{
		{"zero range", 2.0, 1.0, 1.0, 0, 0.1, 0.1},
		{"no volume data", 0, 3.0, 4.0, 10.0, 0.3, 0.8},
		{"high volume + strong wick + strong reversal", 3.0, 5.0, 5.0, 10.0, 0.8, 1.0},
		{"low volume + weak wick + weak reversal", 0.5, 0.5, 0.2, 10.0, 0.1, 0.2},
		{"medium everything", 1.5, 2.0, 2.0, 10.0, 0.3, 0.6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := sweepQuality(tt.volRatio, tt.wickBeyond, tt.reversalDist, tt.cRange)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("sweepQuality(%v, %v, %v, %v) = %f, want [%f, %f]",
					tt.volRatio, tt.wickBeyond, tt.reversalDist, tt.cRange,
					score, tt.minScore, tt.maxScore)
			}
		})
	}
}

// ── Sweep vs Breakout ─────────────────────────────────────────────────────────

// TestSweepBreakout_ConfirmedBreakout: sweep followed by 3 strong continuation bars → nil
func TestSweepBreakout_ConfirmedBreakout(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("AAPL")

	// SWING_LOW = 100. bars[0] sweeps below 100 (low=97, close=102).
	// Then 3 subsequent bars close below 100 with strong bodies → breakout, not sweep.
	bars := []models.OHLCV{
		makeBar(102, 105, 97, 102), // index 0: sweep candidate (low=97 < 100, close=102 > 100)
		makeBar(99, 101, 95, 96),   // index 1: close=96 < 100, body=3/6=50% → strong
		makeBar(96, 98, 92, 93),    // index 2: close=93 < 100, body=3/6=50% → strong
		makeBar(93, 95, 89, 90),    // index 3: close=90 < 100, body=3/6=50% → strong
		makeBar(90, 92, 87, 88),    // index 4: current bar
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 120.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The sweep at index 0 should be classified as a breakout → suppressed
	// The current bar (index 4) doesn't qualify as a sweep either
	if sig != nil {
		t.Errorf("expected nil (breakout suppressed), got %+v", sig)
	}
}

// TestSweepBreakout_ConfirmedSweep: sweep followed by reversal bars → signal emitted
func TestSweepBreakout_ConfirmedSweep(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("AAPL")

	// SWING_LOW = 100. bars[0] sweeps below 100 (low=97, close=102).
	// Subsequent bars close ABOVE 100 → confirmed sweep (not breakout).
	bars := []models.OHLCV{
		makeBar(102, 105, 97, 102), // index 0: sweep candidate (low=97 < 100, close=102)
		makeBar(103, 108, 101, 107), // index 1: close=107 > 100 → reversed
		makeBar(107, 112, 106, 110), // index 2: close=110 > 100 → reversed
		makeBar(110, 115, 109, 113), // index 3: close=113 > 100 → reversed
		makeBar(113, 116, 111, 114), // index 4: current bar
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 130.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal for confirmed sweep, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
}

// TestSweepBreakout_InsufficientConfirmation: sweep is too recent → signal still emitted
func TestSweepBreakout_InsufficientConfirmation(t *testing.T) {
	rule := &ICTLiquiditySweepRule{}
	ctx := makeCtx("AAPL")

	// The latest bar IS the sweep bar — no confirmation bars available
	bars := []models.OHLCV{
		makeBar(102, 105, 97, 102), // current bar = sweep (low=97 < 100, close=102 > 100)
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_LOW"] = 100.0
	ctx.Indicators["1H:SWING_HIGH"] = 120.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal (insufficient confirmation → not suppressed), got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
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
