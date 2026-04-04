package general_ta

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// makeVSABar creates an OHLCV bar with explicit volume for VSA tests.
func makeVSABar(open, high, low, close, volume float64) models.OHLCV {
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

// TestVSA_StoppingVolume: high volume + narrow body + close in bottom 40% + downtrend → LONG
func TestVSA_StoppingVolume(t *testing.T) {
	rule := &VSAEffortCandleRule{}
	ctx := makeCtx("BTCUSDT")

	// Prior 5-bar downtrend, then current bar:
	// ATR = 10, body threshold = 0.3*10 = 3
	// Volume MA = 1000, threshold = 1.5*1000 = 1500
	// Current: open=101, high=105, low=95, close=96, volume=2000
	// Body = |101-96| = 5 ... too big. Let's make it narrower.
	// Current: open=97, high=105, low=95, close=96, volume=2000
	// Body = |97-96| = 1 <= 3 ✓
	// Close position = (96-95)/(105-95) = 0.1 <= 0.4 ✓
	// Volume ratio = 2000/1000 = 2.0 >= 1.5 ✓
	bars := []models.OHLCV{
		makeVSABar(120, 122, 118, 119, 800), // downtrend start
		makeVSABar(119, 120, 115, 116, 900),
		makeVSABar(116, 117, 112, 113, 850),
		makeVSABar(113, 114, 108, 109, 900),
		makeVSABar(109, 110, 104, 105, 950),
		makeVSABar(105, 106, 100, 101, 900), // prior bar (downtrend confirmed: 101 < 120)
		makeVSABar(97, 105, 95, 96, 2000),   // current: stopping volume
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 10.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

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
	if sig.Rule != "vsa_effort_candle" {
		t.Errorf("wrong rule: %s", sig.Rule)
	}
	// Stopping volume score = volRatio/3.0 = 2.0/3.0 ≈ 0.667
	if sig.Score < 0.5 || sig.Score > 0.8 {
		t.Errorf("expected score ~0.67, got %f", sig.Score)
	}
}

// TestVSA_NoDemand: low volume + narrow body + bullish candle + uptrend → SHORT
func TestVSA_NoDemand(t *testing.T) {
	rule := &VSAEffortCandleRule{}
	ctx := makeCtx("BTCUSDT")

	// ATR = 10, body threshold = 3
	// Volume MA = 1000, low volume threshold = 0.8*1000 = 800
	// Current: open=100, close=101 (bullish, body=1 <= 3), volume=500 (< 800)
	// Prior uptrend
	bars := []models.OHLCV{
		makeVSABar(80, 82, 78, 81, 800),
		makeVSABar(81, 84, 80, 83, 900),
		makeVSABar(83, 87, 82, 86, 850),
		makeVSABar(86, 90, 85, 89, 900),
		makeVSABar(89, 93, 88, 92, 950),
		makeVSABar(92, 96, 91, 95, 900), // prior bar (uptrend: 95 > 80)
		makeVSABar(100, 102, 99, 101, 500), // current: no demand
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 10.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

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
	if sig.Score != 0.5 {
		t.Errorf("expected score 0.5, got %f", sig.Score)
	}
}

// TestVSA_NoSupply: low volume + narrow body + bearish candle + downtrend → LONG
func TestVSA_NoSupply(t *testing.T) {
	rule := &VSAEffortCandleRule{}
	ctx := makeCtx("BTCUSDT")

	// ATR = 10, body threshold = 3
	// Volume MA = 1000, low volume threshold = 800
	// Current: open=101, close=100 (bearish, body=1 <= 3), volume=500 (< 800)
	// Prior downtrend
	bars := []models.OHLCV{
		makeVSABar(120, 122, 118, 119, 800),
		makeVSABar(119, 120, 115, 116, 900),
		makeVSABar(116, 117, 112, 113, 850),
		makeVSABar(113, 114, 108, 109, 900),
		makeVSABar(109, 110, 104, 105, 950),
		makeVSABar(105, 106, 100, 101, 900), // prior bar (downtrend: 101 < 120)
		makeVSABar(101, 103, 99, 100, 500),  // current: no supply
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 10.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

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
	if sig.Score != 0.5 {
		t.Errorf("expected score 0.5, got %f", sig.Score)
	}
}

// TestVSA_NoSignal: conditions not met → nil
func TestVSA_NoSignal(t *testing.T) {
	rule := &VSAEffortCandleRule{}
	ctx := makeCtx("BTCUSDT")

	// Normal volume, wide body — no VSA pattern
	bars := []models.OHLCV{
		makeVSABar(80, 82, 78, 81, 800),
		makeVSABar(81, 84, 80, 83, 900),
		makeVSABar(83, 87, 82, 86, 850),
		makeVSABar(86, 90, 85, 89, 900),
		makeVSABar(89, 93, 88, 92, 950),
		makeVSABar(92, 96, 91, 95, 900),
		makeVSABar(95, 105, 93, 103, 1100), // current: body=8 > 3 (not narrow), volume=1100 (normal)
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:ATR_14"] = 10.0
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	sig, err := rule.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil, got %+v", sig)
	}
}
