package smc

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── Test helpers ─────────────────────────────────────────────────────────────

func makeCtx(symbol string) models.AnalysisContext {
	return models.AnalysisContext{
		Symbol:     symbol,
		Timeframes: map[string][]models.OHLCV{},
		Indicators: map[string]float64{},
	}
}

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

// makeBarAt creates a bar centered at price with ±1 wick.
func makeBarAt(price float64) models.OHLCV {
	return makeBar(price, price+1, price-1, price)
}

// makeFlatBars returns n bars all at the given price.
func makeFlatBars(n int, price float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	for i := range bars {
		bars[i] = makeBarAt(price)
	}
	return bars
}

// makeUptrendBars returns n bars rising from base by step per bar.
func makeUptrendBars(n int, base, step float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	for i := range bars {
		bars[i] = makeBarAt(base + float64(i)*step)
	}
	return bars
}

// makeDowntrendBars returns n bars falling from base by step per bar.
func makeDowntrendBars(n int, base, step float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	for i := range bars {
		bars[i] = makeBarAt(base - float64(i)*step)
	}
	return bars
}

// ── SMCBOSRule ───────────────────────────────────────────────────────────────

// TestBOS_Bullish_1H: uptrend + structural high break → LONG
func TestBOS_Bullish_1H(t *testing.T) {
	rule := &SMCBOSRule{}
	ctx := makeCtx("BTCUSDT")

	// 10 flat bars + 20 uptrend bars (100→115.2) + current at 125
	// structuralHigh of lookbackBars[10:30] = bars[10..29] = ~116.2 (bars[29].high)
	// curr.Close=125 > 116.2 → LONG
	bars := makeFlatBars(10, 100)
	bars = append(bars, makeUptrendBars(20, 100, 0.8)...)
	bars = append(bars, makeBar(118, 128, 117, 125)) // current
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
	if sig.Rule != "smc_bos" {
		t.Errorf("wrong rule name: %s", sig.Rule)
	}
	if sig.Score < 0.1 || sig.Score > 1.0 {
		t.Errorf("score out of range [0.1,1.0]: %f", sig.Score)
	}
}

// TestBOS_Bearish_4H: downtrend + structural low break → SHORT
func TestBOS_Bearish_4H(t *testing.T) {
	rule := &SMCBOSRule{}
	ctx := makeCtx("ETHUSDT")

	// 10 flat bars at 115 + 20 downtrend bars (115→99.8) + current at 85
	// structuralLow of lookbackBars[10:30] = ~98.8 (bars[29].low)
	// curr.Close=85 < 98.8 → SHORT
	bars := makeFlatBars(10, 115)
	bars = append(bars, makeDowntrendBars(20, 115, 0.8)...)
	bars = append(bars, makeBar(87, 90, 82, 85)) // current
	ctx.Timeframes["4H"] = bars

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
	if sig.Rule != "smc_bos" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
}

// TestBOS_NoTrend_ReturnsNil: flat bars → trendDir="NONE" → nil
func TestBOS_NoTrend_ReturnsNil(t *testing.T) {
	rule := &SMCBOSRule{}
	ctx := makeCtx("AAPL")

	// All 31 bars at exactly 100 — no trend
	bars := makeFlatBars(31, 100)
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for NONE trend, got %+v", sig)
	}
}

// TestBOS_InsufficientBars: fewer than 30 bars → nil
func TestBOS_InsufficientBars(t *testing.T) {
	rule := &SMCBOSRule{}
	ctx := makeCtx("AAPL")

	bars := makeUptrendBars(29, 100, 0.5)
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for insufficient bars, got %+v", sig)
	}
}

// TestBOS_NoBreak_ReturnsNil: uptrend exists but current bar doesn't break structural high
func TestBOS_NoBreak_ReturnsNil(t *testing.T) {
	rule := &SMCBOSRule{}
	ctx := makeCtx("AAPL")

	// uptrend, but curr.Close=113 < structuralHigh(~116.2)
	bars := makeFlatBars(10, 100)
	bars = append(bars, makeUptrendBars(20, 100, 0.8)...)
	bars = append(bars, makeBarAt(113)) // current: inside structure, no break
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil when no structural break, got %+v", sig)
	}
}

// TestBOS_MultiTF_BestPicked: same uptrend+break in 1H and 1D; 1D wins (higher weight)
func TestBOS_MultiTF_BestPicked(t *testing.T) {
	rule := &SMCBOSRule{}
	ctx := makeCtx("BTCUSDT")

	bars := makeFlatBars(10, 100)
	bars = append(bars, makeUptrendBars(20, 100, 0.8)...)
	bars = append(bars, makeBar(118, 128, 117, 125))

	ctx.Timeframes["1H"] = bars
	ctx.Timeframes["1D"] = bars // same data, but 1D has tfWeight=1.5

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Timeframe != "1D" {
		t.Errorf("expected 1D (higher weight), got %s", sig.Timeframe)
	}
}

// TestBOS_Name: Name() returns correct identifier
func TestBOS_Name(t *testing.T) {
	rule := &SMCBOSRule{}
	if rule.Name() != "smc_bos" {
		t.Errorf("expected smc_bos, got %s", rule.Name())
	}
}

// ── SMCChoCHRule ─────────────────────────────────────────────────────────────

// TestChoCH_Bullish_Reversal: prior downtrend + break of structural high → LONG
func TestChoCH_Bullish_Reversal(t *testing.T) {
	rule := &SMCChoCHRule{}
	ctx := makeCtx("BTCUSDT")

	// 10 flat + 20 downtrend (110→94.8) + 4 transition + current at 120
	// priorBars = bars[:30], trend = DOWN
	// structuralHigh(priorBars[10:30]) = bars[10].high = 111
	// curr.Close=120 > 111 → LONG
	bars := makeFlatBars(10, 110)
	bars = append(bars, makeDowntrendBars(20, 110, 0.8)...)
	bars = append(bars, makeFlatBars(4, 95)...)
	bars = append(bars, makeBar(118, 125, 117, 120)) // current
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
	if sig.Rule != "smc_choch" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
}

// TestChoCH_Bearish_Reversal: prior uptrend + break of structural low → SHORT
func TestChoCH_Bearish_Reversal(t *testing.T) {
	rule := &SMCChoCHRule{}
	ctx := makeCtx("ETHUSDT")

	// 10 flat at 95 + 20 uptrend (95→110.2) + 4 transition + current at 80
	// priorBars = bars[:30], trend = UP
	// structuralLow(priorBars[10:30]) = bars[10].low = 94
	// curr.Close=80 < 94 → SHORT
	bars := makeFlatBars(10, 95)
	bars = append(bars, makeUptrendBars(20, 95, 0.8)...)
	bars = append(bars, makeFlatBars(4, 110)...)
	bars = append(bars, makeBar(82, 85, 78, 80)) // current
	ctx.Timeframes["4H"] = bars

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
	if sig.Rule != "smc_choch" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
}

// TestChoCH_NoTrend_ReturnsNil: flat priorBars → NONE trend → nil
func TestChoCH_NoTrend_ReturnsNil(t *testing.T) {
	rule := &SMCChoCHRule{}
	ctx := makeCtx("AAPL")

	// 35 flat bars — priorBars[:30] has NONE trend
	bars := makeFlatBars(35, 100)
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for NONE trend, got %+v", sig)
	}
}

// TestChoCH_InsufficientBars: fewer than 30 bars → nil
func TestChoCH_InsufficientBars(t *testing.T) {
	rule := &SMCChoCHRule{}
	ctx := makeCtx("AAPL")

	bars := makeUptrendBars(29, 100, 0.5)
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for insufficient bars, got %+v", sig)
	}
}

// TestChoCH_TrendContinuation_NotChoCH: prior uptrend + break above high = BOS, not CHoCH → nil
func TestChoCH_TrendContinuation_NotChoCH(t *testing.T) {
	rule := &SMCChoCHRule{}
	ctx := makeCtx("AAPL")

	// priorBars show UP trend; curr breaks ABOVE structuralHigh (BOS territory)
	// CHoCH for UP trend only triggers on curr.Close < structuralLow, so → nil
	bars := makeFlatBars(10, 95)
	bars = append(bars, makeUptrendBars(20, 95, 0.8)...)
	bars = append(bars, makeFlatBars(4, 110)...)
	bars = append(bars, makeBar(118, 125, 117, 120)) // close above structHigh → continuation, not reversal
	ctx.Timeframes["1H"] = bars

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil for trend continuation (not CHoCH), got %+v", sig)
	}
}

// TestChoCH_MultiTF_BestPicked: same reversal in 1H and 1D; 1D wins (higher weight)
func TestChoCH_MultiTF_BestPicked(t *testing.T) {
	rule := &SMCChoCHRule{}
	ctx := makeCtx("BTCUSDT")

	bars := makeFlatBars(10, 110)
	bars = append(bars, makeDowntrendBars(20, 110, 0.8)...)
	bars = append(bars, makeFlatBars(4, 95)...)
	bars = append(bars, makeBar(118, 125, 117, 120))

	ctx.Timeframes["1H"] = bars
	ctx.Timeframes["1D"] = bars // same data; 1D has tfWeight=1.5

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Timeframe != "1D" {
		t.Errorf("expected 1D (higher weight), got %s", sig.Timeframe)
	}
}

// TestChoCH_Name: Name() returns correct identifier
func TestChoCH_Name(t *testing.T) {
	rule := &SMCChoCHRule{}
	if rule.Name() != "smc_choch" {
		t.Errorf("expected smc_choch, got %s", rule.Name())
	}
}
