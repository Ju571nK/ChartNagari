package general_ta

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

func makeBars(closes []float64) []models.OHLCV {
	bars := make([]models.OHLCV, len(closes))
	for i, c := range closes {
		bars[i] = models.OHLCV{
			OpenTime: time.Now().Add(time.Duration(i) * time.Hour),
			Open:     c,
			High:     c * 1.01,
			Low:      c * 0.99,
			Close:    c,
			Volume:   1000,
		}
	}
	return bars
}

// --- RSIOverboughtOversold tests ---

func TestRSIOverboughtOversold_Overbought(t *testing.T) {
	r := &RSIOverboughtOversoldRule{}
	ctx := makeCtx("TEST")
	ctx.Indicators["1H:RSI_14"] = 80.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
	if sig.Timeframe != "1H" {
		t.Errorf("expected 1H, got %s", sig.Timeframe)
	}
}

func TestRSIOverboughtOversold_Oversold(t *testing.T) {
	r := &RSIOverboughtOversoldRule{}
	ctx := makeCtx("TEST")
	ctx.Indicators["1H:RSI_14"] = 20.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestRSIOverboughtOversold_Neutral(t *testing.T) {
	r := &RSIOverboughtOversoldRule{}
	ctx := makeCtx("TEST")
	ctx.Indicators["1H:RSI_14"] = 50.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal for neutral RSI, got %+v", sig)
	}
}

func TestRSIOverboughtOversold_BestTF(t *testing.T) {
	// RSI=75 on 1D (score=(75-70)/30=0.167, weighted=0.167*1.5=0.25)
	// RSI=65 on 1H → no signal (< 70)
	// so 1D should win
	r := &RSIOverboughtOversoldRule{}
	ctx := makeCtx("TEST")
	ctx.Indicators["1D:RSI_14"] = 75.0
	ctx.Indicators["1H:RSI_14"] = 65.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Timeframe != "1D" {
		t.Errorf("expected 1D to win, got %s", sig.Timeframe)
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
}

// --- RSI Divergence tests ---

// buildDivBars constructs bars for divergence testing by directly verifying
// that the swing pairs and RSI conditions hold before returning.
// For bullish: closes[lo2] < closes[lo1] (price lower low) AND rsiVals[lo2] > rsiVals[lo1] (RSI higher low)
// For bearish: closes[hi2] > closes[hi1] (price higher high) AND rsiVals[hi2] < rsiVals[hi1] (RSI lower high)
//
// We construct 34+ bars (divLookback+divRSIPeriod) and use the helper functions
// directly to verify our test data is valid.

func TestRSIDivergence_Bullish(t *testing.T) {
	r := &RSIDivergenceRule{}
	ctx := makeCtx("TEST")

	// Build bars designed to produce bullish divergence.
	// Total bars = divLookback + divRSIPeriod = 34.
	// The last 20 bars are the lookback window for swing detection and RSI comparison.
	//
	// Strategy:
	//   Swing low 1: reached via rapid crash (lots of consecutive down bars → high avgLoss → very low RSI)
	//   Swing low 2: price even lower but reached via slow grinding decline → RSI not as depressed
	//
	// Warmup (first 14 bars): stable at 100 so RSI ~50.
	// Recent window pattern (indices 14..33 = recentCloses[0..19]):
	//   [0] 100  [1] 98  [2] 95  [3] 85 ← swing low 1 (95→85→92: fast crash from 100)
	//   [4] 92  [5] 96  [6] 98  [7] 97
	//   [8] 96  [9] 95  [10] 94  [11] 93
	//   [12] 92  [13] 91  [14] 90  [15] 89
	//   [16] 88  [17] 86  [18] 84 ← swing low 2 (86→84→86: very slow grind)
	//   [19] 86
	//   Price: 84 < 85 ✓ (lower low)
	//   RSI at [18] should be higher than RSI at [3] because the descent to 84 is slow/gradual
	closes := make([]float64, 34)
	for i := 0; i < 14; i++ {
		closes[i] = 100.0
	}
	// Recent window (indices 14..33 = recentCloses[0..19]):
	closes[14] = 100 // [0]
	closes[15] = 98  // [1]
	closes[16] = 95  // [2]
	closes[17] = 85  // [3] ← swing low 1 (95→85→92: rapid drop; 85<95 && 85<92)
	closes[18] = 92  // [4]
	closes[19] = 96  // [5]
	closes[20] = 98  // [6]
	closes[21] = 97  // [7]
	closes[22] = 96  // [8]
	closes[23] = 95  // [9]
	closes[24] = 94  // [10]
	closes[25] = 93  // [11]
	closes[26] = 92  // [12]
	closes[27] = 91  // [13]
	closes[28] = 90  // [14]
	closes[29] = 89  // [15]
	closes[30] = 88  // [16]
	closes[31] = 86  // [17]
	closes[32] = 84  // [18] ← swing low 2 (86→84→86: 84<86 && 84<86): price lower low (84<85)
	closes[33] = 86  // [19]

	// Verify our test data
	recentCloses := closes[14:]
	lo1, lo2 := swingLowPair(recentCloses)
	if lo1 < 0 || lo2 < 0 {
		t.Fatalf("test data has no swing low pair in recent window, lo1=%d lo2=%d", lo1, lo2)
	}

	rsiAll := rollingRSI(closes, divRSIPeriod)
	if rsiAll == nil || len(rsiAll) < divLookback {
		t.Fatalf("RSI computation failed: len=%d", len(rsiAll))
	}
	rsiVals := rsiAll[len(rsiAll)-divLookback:]

	priceLowerLow := recentCloses[lo2] < recentCloses[lo1]
	rsiHigherLow := rsiVals[lo2] > rsiVals[lo1]
	if !priceLowerLow {
		t.Logf("WARNING: price lower low not satisfied: closes[lo2]=%.2f closes[lo1]=%.2f", recentCloses[lo2], recentCloses[lo1])
	}
	if !rsiHigherLow {
		t.Logf("WARNING: RSI higher low not satisfied: rsi[lo2]=%.2f rsi[lo1]=%.2f", rsiVals[lo2], rsiVals[lo1])
	}
	if !priceLowerLow || !rsiHigherLow {
		t.Fatal("test data does not produce bullish divergence conditions")
	}

	bars := make([]models.OHLCV, len(closes))
	for i, c := range closes {
		bars[i] = models.OHLCV{
			OpenTime: time.Now().Add(time.Duration(i) * time.Hour),
			Open:     c,
			High:     c + 1,
			Low:      c - 1,
			Close:    c,
			Volume:   1000,
		}
	}
	ctx.Timeframes["1H"] = bars

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected bullish divergence signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestRSIDivergence_Bearish(t *testing.T) {
	r := &RSIDivergenceRule{}
	ctx := makeCtx("TEST")

	// Build bars designed to produce bearish divergence.
	// Total bars = 34 (divLookback + divRSIPeriod).
	// Warmup: 14 bars oscillating around 100.
	// Recent window (last 20 bars):
	//   Strategy for bearish divergence:
	//   - Swing high 1: reached via rapid climb → high RSI (lots of gains)
	//   - Swing high 2: price higher but reached via slow/grinding climb → RSI lower
	//     because the gradual rise doesn't push RSI as high as a rapid one.
	//
	//   Warmup: flat at 100 so RSI ~50.
	//   Recent[0..2]: 100, 102, 108 (fast rise)
	//   Recent[3]: 115 ← swing high 1 (rapid gains → high RSI ~70-80)
	//   Recent[4]: 110
	//   Recent[5..10]: 105, 102, 100, 99, 98, 99 (consolidation / pullback)
	//   Recent[11..18]: slow grind up: 101,103,105,107,109,111,113,116
	//   Recent[19]: 114 ← after swing high 2 at index 18 (116 > 113 and 116 > 114)
	//     price higher high: 116 > 115 ✓
	//     RSI at 18: slow grind → RSI lower than swing high 1 ✓
	closes := make([]float64, 34)
	for i := 0; i < 14; i++ {
		closes[i] = 100.0
	}
	// Recent window (indices 14..33 = recentCloses[0..19]):
	closes[14] = 100 // [0]
	closes[15] = 102 // [1]
	closes[16] = 108 // [2]
	closes[17] = 115 // [3] ← swing high 1 (108→115→110: 115>108 && 115>110)
	closes[18] = 110 // [4]
	closes[19] = 105 // [5]
	closes[20] = 102 // [6]
	closes[21] = 100 // [7]
	closes[22] = 99  // [8]
	closes[23] = 98  // [9]
	closes[24] = 99  // [10]
	closes[25] = 101 // [11]
	closes[26] = 103 // [12]
	closes[27] = 105 // [13]
	closes[28] = 107 // [14]
	closes[29] = 109 // [15]
	closes[30] = 111 // [16]
	closes[31] = 113 // [17]
	closes[32] = 116 // [18] ← swing high 2 (113→116→114: 116>113 && 116>114), price higher high (116>115)
	closes[33] = 114 // [19]

	// Verify our test data
	recentCloses := closes[14:]
	hi1, hi2 := swingHighPair(recentCloses)
	if hi1 < 0 || hi2 < 0 {
		t.Fatalf("test data has no swing high pair in recent window, hi1=%d hi2=%d", hi1, hi2)
	}

	rsiAll := rollingRSI(closes, divRSIPeriod)
	if rsiAll == nil || len(rsiAll) < divLookback {
		t.Fatalf("RSI computation failed: len=%d", len(rsiAll))
	}
	rsiVals := rsiAll[len(rsiAll)-divLookback:]

	priceHigherHigh := recentCloses[hi2] > recentCloses[hi1]
	rsiLowerHigh := rsiVals[hi2] < rsiVals[hi1]
	if !priceHigherHigh {
		t.Logf("WARNING: price higher high not satisfied: closes[hi2]=%.2f closes[hi1]=%.2f", recentCloses[hi2], recentCloses[hi1])
	}
	if !rsiLowerHigh {
		t.Logf("WARNING: RSI lower high not satisfied: rsi[hi2]=%.2f rsi[hi1]=%.2f", rsiVals[hi2], rsiVals[hi1])
	}
	if !priceHigherHigh || !rsiLowerHigh {
		t.Fatal("test data does not produce bearish divergence conditions")
	}

	bars := make([]models.OHLCV, len(closes))
	for i, c := range closes {
		bars[i] = models.OHLCV{
			OpenTime: time.Now().Add(time.Duration(i) * time.Hour),
			Open:     c,
			High:     c + 0.5,
			Low:      c - 0.5,
			Close:    c,
			Volume:   1000,
		}
	}
	ctx.Timeframes["1H"] = bars

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected bearish divergence signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestRSIDivergence_InsufficientData(t *testing.T) {
	r := &RSIDivergenceRule{}
	ctx := makeCtx("TEST")
	// Only 10 bars – not enough (need divLookback+divRSIPeriod = 34)
	ctx.Timeframes["1H"] = makeBars(make([]float64, 10))

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal for insufficient data, got %+v", sig)
	}
}

// --- EMA Cross tests ---

func TestEMACross_GoldenCross(t *testing.T) {
	r := &EMACrossRule{}
	ctx := makeCtx("TEST")

	// Strategy: many bars of declining price so EMA9 < EMA20, then at the
	// very last bar a huge spike that forces EMA9 to cross above EMA20.
	// We verify that the cross condition holds: prevFast < prevSlow && currFast >= currSlow.
	//
	// Use 50 bars: 48 bars declining slowly (100→76), then bar49 = 76 (flat prev),
	// bar50 is the giant spike. We generate until we find a dataset that yields a cross.
	// Simpler: construct so prevFast < prevSlow deterministically.
	//
	// 40 bars at 80.0 then 1 bar at 200.0 → EMA9 spikes above EMA20 at last bar.
	closes := make([]float64, 41)
	for i := 0; i < 40; i++ {
		closes[i] = 80.0
	}
	// When all closes are equal, EMA9 == EMA20 == 80. We need prev cross.
	// Use declining for first 39, then flat, then spike:
	// Declining by 1 each bar for 39 bars (100→62), then same, then spike.
	for i := 0; i < 39; i++ {
		closes[i] = 100 - float64(i)
	}
	closes[39] = closes[38] // flat (prev bar, EMA9 still slightly < EMA20 due to slow weighting)
	closes[40] = 300        // giant spike: pushes EMA9 far above EMA20

	// Verify the cross using the same logic as EMACrossRule
	fastEMA := rollingEMA(closes, emaFast)
	slowEMA := rollingEMA(closes, emaSlow)
	if fastEMA == nil || slowEMA == nil {
		t.Fatal("EMA computation returned nil for test data")
	}
	prevFast := fastEMA[len(fastEMA)-2]
	currFast := fastEMA[len(fastEMA)-1]
	prevSlow := slowEMA[len(slowEMA)-2]
	currSlow := slowEMA[len(slowEMA)-1]
	if !(prevFast < prevSlow && currFast >= currSlow) {
		t.Fatalf("test data does not produce golden cross: prevF=%.4f prevS=%.4f currF=%.4f currS=%.4f",
			prevFast, prevSlow, currFast, currSlow)
	}

	ctx.Timeframes["1H"] = makeBars(closes)

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected golden cross signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", sig.Score)
	}
}

func TestEMACross_DeathCross(t *testing.T) {
	r := &EMACrossRule{}
	ctx := makeCtx("TEST")

	// Rising prices for 39 bars (62→100), flat, then giant drop → EMA9 crosses below EMA20.
	closes := make([]float64, 41)
	for i := 0; i < 39; i++ {
		closes[i] = 62 + float64(i)
	}
	closes[39] = closes[38]
	closes[40] = 1 // giant drop: pushes EMA9 far below EMA20

	// Verify the cross
	fastEMA := rollingEMA(closes, emaFast)
	slowEMA := rollingEMA(closes, emaSlow)
	if fastEMA == nil || slowEMA == nil {
		t.Fatal("EMA computation returned nil for test data")
	}
	prevFast := fastEMA[len(fastEMA)-2]
	currFast := fastEMA[len(fastEMA)-1]
	prevSlow := slowEMA[len(slowEMA)-2]
	currSlow := slowEMA[len(slowEMA)-1]
	if !(prevFast > prevSlow && currFast <= currSlow) {
		t.Fatalf("test data does not produce death cross: prevF=%.4f prevS=%.4f currF=%.4f currS=%.4f",
			prevFast, prevSlow, currFast, currSlow)
	}

	ctx.Timeframes["1H"] = makeBars(closes)

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected death cross signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
	if sig.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", sig.Score)
	}
}

func TestEMACross_NoRecentCross(t *testing.T) {
	r := &EMACrossRule{}
	ctx := makeCtx("TEST")

	// Flat prices – no cross
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = 100.0
	}

	ctx.Timeframes["1H"] = makeBars(closes)

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal for flat prices, got %+v", sig)
	}
}

// --- Support/Resistance Breakout tests ---

func TestSRBreakout_Above(t *testing.T) {
	r := &SupportResistanceBreakoutRule{}
	ctx := makeCtx("TEST")

	// prev close <= swingHigh, curr close > swingHigh
	bars := []models.OHLCV{
		{Open: 99, High: 100, Low: 98, Close: 99, Volume: 1000, OpenTime: time.Now()},
		{Open: 101, High: 103, Low: 100, Close: 102, Volume: 1000, OpenTime: time.Now().Add(time.Hour)},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_HIGH"] = 100.0
	ctx.Indicators["1H:SWING_LOW"] = 90.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected breakout signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestSRBreakout_Below(t *testing.T) {
	r := &SupportResistanceBreakoutRule{}
	ctx := makeCtx("TEST")

	// prev close >= swingLow, curr close < swingLow
	bars := []models.OHLCV{
		{Open: 91, High: 92, Low: 90, Close: 91, Volume: 1000, OpenTime: time.Now()},
		{Open: 89, High: 90, Low: 87, Close: 88, Volume: 1000, OpenTime: time.Now().Add(time.Hour)},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_HIGH"] = 100.0
	ctx.Indicators["1H:SWING_LOW"] = 90.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected breakdown signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestSRBreakout_NoBreak(t *testing.T) {
	r := &SupportResistanceBreakoutRule{}
	ctx := makeCtx("TEST")

	// Price stays within range
	bars := []models.OHLCV{
		{Open: 95, High: 96, Low: 94, Close: 95, Volume: 1000, OpenTime: time.Now()},
		{Open: 96, High: 97, Low: 95, Close: 96, Volume: 1000, OpenTime: time.Now().Add(time.Hour)},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:SWING_HIGH"] = 100.0
	ctx.Indicators["1H:SWING_LOW"] = 90.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal for no breakout, got %+v", sig)
	}
}

// --- Fibonacci Confluence tests ---

func TestFibonacciConfluence_NearLevel(t *testing.T) {
	r := &FibonacciConfluenceRule{}
	ctx := makeCtx("TEST")

	// FIB_618 = 61.8, price = 61.9 (0.16% away, within 0.5% tolerance)
	// FIB_500 = 50.0, price < FIB_500? No, 61.9 > 50 → SHORT
	fib618 := 61.8
	price := 61.9 // within 0.3% of FIB_618
	bars := []models.OHLCV{
		{Open: price, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 1000, OpenTime: time.Now()},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:FIB_500"] = 50.0
	ctx.Indicators["1H:FIB_236"] = 23.6
	ctx.Indicators["1H:FIB_382"] = 38.2
	ctx.Indicators["1H:FIB_618"] = fib618
	ctx.Indicators["1H:FIB_786"] = 78.6

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected fibonacci confluence signal, got nil")
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestFibonacciConfluence_NotNear(t *testing.T) {
	r := &FibonacciConfluenceRule{}
	ctx := makeCtx("TEST")

	// Price far from all levels
	price := 55.0
	bars := []models.OHLCV{
		{Open: price, High: price + 0.1, Low: price - 0.1, Close: price, Volume: 1000, OpenTime: time.Now()},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:FIB_500"] = 50.0
	ctx.Indicators["1H:FIB_236"] = 23.6
	ctx.Indicators["1H:FIB_382"] = 38.2
	ctx.Indicators["1H:FIB_618"] = 61.8
	ctx.Indicators["1H:FIB_786"] = 78.6

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal (price far from all fib levels), got %+v", sig)
	}
}

// --- Volume Spike tests ---

func TestVolumeSpike_Bullish(t *testing.T) {
	r := &VolumeSpikeRule{}
	ctx := makeCtx("TEST")

	// volume = 3000, MA = 1000 → ratio=3 ≥ 2, close > open → LONG
	bars := []models.OHLCV{
		{Open: 99, High: 102, Low: 98, Close: 101, Volume: 3000, OpenTime: time.Now()},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected volume spike signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Errorf("expected LONG, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestVolumeSpike_Bearish(t *testing.T) {
	r := &VolumeSpikeRule{}
	ctx := makeCtx("TEST")

	// volume = 3000, MA = 1000 → ratio=3 ≥ 2, close < open → SHORT
	bars := []models.OHLCV{
		{Open: 101, High: 102, Low: 98, Close: 99, Volume: 3000, OpenTime: time.Now()},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected volume spike signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Errorf("expected SHORT, got %s", sig.Direction)
	}
	if sig.Score <= 0 {
		t.Errorf("expected score > 0, got %f", sig.Score)
	}
}

func TestVolumeSpike_Normal(t *testing.T) {
	r := &VolumeSpikeRule{}
	ctx := makeCtx("TEST")

	// volume = 1500, MA = 1000 → ratio=1.5 < 2 → no spike
	bars := []models.OHLCV{
		{Open: 99, High: 102, Low: 98, Close: 101, Volume: 1500, OpenTime: time.Now()},
	}
	ctx.Timeframes["1H"] = bars
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000.0

	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal for normal volume, got %+v", sig)
	}
}
