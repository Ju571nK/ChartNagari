package indicator

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

const floatTol = 1e-6

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// makeOHLCV builds a slice of OHLCV bars with the given close prices.
// High = close, Low = close, Open = close, Volume = 1000 for simplicity.
func makeOHLCV(closes []float64) []models.OHLCV {
	bars := make([]models.OHLCV, len(closes))
	for i, c := range closes {
		bars[i] = models.OHLCV{
			Symbol:    "TEST",
			Timeframe: "1H",
			OpenTime:  time.Now().Add(time.Duration(i) * time.Hour),
			Open:      c,
			High:      c,
			Low:       c,
			Close:     c,
			Volume:    1000,
		}
	}
	return bars
}

// ---------- Test 1: RSI sufficient data → valid result ----------

func TestRSI_SufficientData(t *testing.T) {
	// 15 closes (period=14 needs period+1=15)
	closes := make([]float64, 15)
	for i := range closes {
		closes[i] = float64(100 + i) // strictly increasing → RSI near 100
	}
	v, ok := rsi(closes, 14)
	if !ok {
		t.Fatal("expected ok=true for sufficient data")
	}
	if v < 0 || v > 100 {
		t.Fatalf("RSI out of range [0,100]: %f", v)
	}
	// All gains, no losses → RSI should be 100.
	if !almostEqual(v, 100.0, floatTol) {
		t.Fatalf("expected RSI=100 for all-gain series, got %f", v)
	}
}

// ---------- Test 2: RSI insufficient data → ok=false ----------

func TestRSI_InsufficientData(t *testing.T) {
	closes := []float64{1, 2, 3} // only 3 bars, period=14 needs 15
	_, ok := rsi(closes, 14)
	if ok {
		t.Fatal("expected ok=false for insufficient data")
	}
}

// ---------- Test 3: EMA converges to constant price ----------

func TestEMA_ConstantPrice(t *testing.T) {
	// When all prices are the same, EMA must equal that price.
	const price = 50.0
	prices := make([]float64, 100)
	for i := range prices {
		prices[i] = price
	}
	v, ok := ema(prices, 9)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !almostEqual(v, price, floatTol) {
		t.Fatalf("EMA of constant series: expected %f, got %f", price, v)
	}
}

// ---------- Test 4: SMA accuracy ----------

func TestSMA_Accuracy(t *testing.T) {
	// SMA(5) of [1,2,3,4,5,6,7,8,9,10] over last 5 = (6+7+8+9+10)/5 = 8
	prices := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	v, ok := sma(prices, 5)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !almostEqual(v, 8.0, floatTol) {
		t.Fatalf("SMA expected 8.0, got %f", v)
	}
}

// ---------- Test 5: MACD cross detection ----------

func TestMACD_CrossDetection(t *testing.T) {
	// Build a series: 34 flat bars then a sharp spike to create a cross.
	closes := make([]float64, 50)
	for i := 0; i < 34; i++ {
		closes[i] = 100.0
	}
	for i := 34; i < 50; i++ {
		closes[i] = 110.0
	}
	line, sig, hist, ok := macd(closes)
	if !ok {
		t.Fatal("expected ok=true for 50 bars")
	}
	// After the spike, MACD line should be positive (fast EMA > slow EMA).
	if line <= 0 {
		t.Fatalf("expected positive MACD line after upward spike, got %f", line)
	}
	// Histogram = line - signal; verify they are consistent.
	if !almostEqual(hist, line-sig, floatTol) {
		t.Fatalf("hist (%f) != line-signal (%f)", hist, line-sig)
	}
}

func TestMACD_InsufficientData(t *testing.T) {
	closes := make([]float64, 20)
	for i := range closes {
		closes[i] = 100.0
	}
	_, _, _, ok := macd(closes)
	if ok {
		t.Fatal("expected ok=false for only 20 bars (need 34)")
	}
}

// ---------- Test 6: Bollinger Bands with constant price ----------

func TestBollingerBands_ConstantPrice(t *testing.T) {
	// Constant price → stddev=0 → upper=middle=lower.
	closes := make([]float64, 20)
	for i := range closes {
		closes[i] = 50.0
	}
	upper, middle, lower, _, _, ok := bollingerBands(closes, 20, 2.0)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !almostEqual(upper, middle, floatTol) || !almostEqual(lower, middle, floatTol) {
		t.Fatalf("constant price: upper(%f), middle(%f), lower(%f) should all be equal", upper, middle, lower)
	}
	if !almostEqual(middle, 50.0, floatTol) {
		t.Fatalf("expected middle=50, got %f", middle)
	}
}

// ---------- Test 7: ATR calculation accuracy ----------

func TestATR_Accuracy(t *testing.T) {
	// Build bars with known true range = 2 (high-low = 2, prev close = high-1).
	// high=i*10+2, low=i*10, close=i*10+1
	n := 20
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		base := float64(i) * 10
		highs[i] = base + 2
		lows[i] = base
		closes[i] = base + 1
	}
	v, ok := atr(highs, lows, closes, 14)
	if !ok {
		t.Fatal("expected ok=true for 20 bars with period=14")
	}
	// TR for each bar (i>=1):
	// hl  = 2
	// hc  = |(base+2) - ((base-10)+1)| = |11| = 11
	// lc  = |(base)   - ((base-10)+1)| = |9|  = 9
	// TR  = max(2, 11, 9) = 11
	// So ATR should converge to 11 (all TRs are equal).
	if !almostEqual(v, 11.0, floatTol) {
		t.Fatalf("expected ATR=11, got %f", v)
	}
}

// ---------- Test 8: Swing High/Low detection ----------

func TestSwingHighLow_Detection(t *testing.T) {
	// Build a 13-bar series with exactly one unambiguous pivot high and one pivot low.
	// lookback=2: a pivot at index i requires bars[i-2..i+2] all strictly lower (high) /
	// strictly higher (low) than bar[i].
	//
	// Layout (indices 0-12):
	//   Pivot HIGH at index 4: high=200, all neighbours high <= 120
	//   Pivot LOW  at index 8: low=50,  all neighbours low  >= 80
	//   No other bar qualifies as a pivot high or low.
	//
	// highs: 90 95 90 95 200 95 90 95 90 95 90 95 90
	// lows:  80 82 80 82  80 82 80 82 50 82 80 82 80
	// Pivot HIGH at index 4 (200): all surrounding highs are 90 or 95, all < 200.
	// Pivot LOW  at index 8 (50):  all surrounding lows  are 80 or 82, all > 50.
	// No other bar has a strictly-greater high or strictly-lesser low than all neighbours.
	highs := []float64{90, 95, 90, 95, 200, 95, 90, 95, 90, 95, 90, 95, 90}
	lows := []float64{80, 82, 80, 82, 80, 82, 80, 82, 50, 82, 80, 82, 80}

	n := len(highs)
	bars := make([]models.OHLCV, n)
	for i := range bars {
		bars[i] = models.OHLCV{
			High:  highs[i],
			Low:   lows[i],
			Close: (highs[i] + lows[i]) / 2,
		}
	}

	sh, sl, ok := swingHighLow(bars, 2)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if sh != 200.0 {
		t.Fatalf("expected swingHigh=200, got %f", sh)
	}
	if sl != 50.0 {
		t.Fatalf("expected swingLow=50, got %f", sl)
	}
}

// ---------- Test 9: Fibonacci level accuracy ----------

func TestFibonacci_Levels(t *testing.T) {
	sh, sl := 200.0, 100.0
	levels := fibonacci(sh, sl)

	expected := map[string]float64{
		"FIB_0":   100.0,           // sh - diff*1.0 = 200-100 = 100
		"FIB_236": 200 - 100*0.236, // 176.4
		"FIB_382": 200 - 100*0.382, // 161.8
		"FIB_500": 200 - 100*0.5,   // 150.0
		"FIB_618": 200 - 100*0.618, // 138.2
		"FIB_786": 200 - 100*0.786, // 121.4
		"FIB_100": 200.0,
	}

	for k, want := range expected {
		got, exists := levels[k]
		if !exists {
			t.Errorf("key %s missing from fibonacci result", k)
			continue
		}
		if !almostEqual(got, want, floatTol) {
			t.Errorf("%s: expected %f, got %f", k, want, got)
		}
	}

	// swingHigh <= swingLow → empty map.
	empty := fibonacci(100.0, 200.0)
	if len(empty) != 0 {
		t.Fatalf("expected empty map when swingHigh <= swingLow, got %d entries", len(empty))
	}
}

// ---------- Test 10: Compute() integration — empty input → empty map ----------

func TestCompute_EmptyInput(t *testing.T) {
	result := Compute(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty map for nil input, got %d keys", len(result))
	}

	result2 := Compute(map[string][]models.OHLCV{})
	if len(result2) != 0 {
		t.Fatalf("expected empty map for empty input, got %d keys", len(result2))
	}
}

// ---------- Test 11: Compute() integration — valid data produces expected keys ----------

func TestCompute_ValidData(t *testing.T) {
	// 300 bars should satisfy all indicators.
	n := 300
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = 100.0 + float64(i%50) // oscillating prices
	}
	bars := makeOHLCV(closes)
	// Give bars realistic highs/lows for ATR and swing.
	for i := range bars {
		bars[i].High = bars[i].Close + 1
		bars[i].Low = bars[i].Close - 1
	}

	input := map[string][]models.OHLCV{"1H": bars}
	result := Compute(input)

	mustHave := []string{
		"1H:RSI_14",
		"1H:EMA_9", "1H:EMA_20", "1H:EMA_50", "1H:EMA_200",
		"1H:SMA_20", "1H:SMA_50", "1H:SMA_200",
		"1H:VOLUME_MA_20",
		"1H:MACD_line", "1H:MACD_signal", "1H:MACD_hist",
		"1H:BB_upper", "1H:BB_middle", "1H:BB_lower", "1H:BB_width", "1H:BB_pct",
		"1H:OBV",
		"1H:ATR_14",
	}
	for _, k := range mustHave {
		if _, exists := result[k]; !exists {
			t.Errorf("missing key in Compute result: %s", k)
		}
	}
}

// ---------- Test 12: OBV correctness ----------

func TestOBV_Correctness(t *testing.T) {
	// up, up, down: +vol2 +vol3 -vol4
	closes := []float64{100, 101, 102, 101}
	volumes := []float64{1000, 500, 300, 200}
	// OBV = +500 +300 -200 = 600
	result := obv(closes, volumes)
	if !almostEqual(result, 600.0, floatTol) {
		t.Fatalf("expected OBV=600, got %f", result)
	}
}

// ---------- Test 13: RSI all-loss series → RSI near 0 ----------

func TestRSI_AllLoss(t *testing.T) {
	closes := make([]float64, 15)
	for i := range closes {
		closes[i] = float64(100 - i) // strictly decreasing
	}
	v, ok := rsi(closes, 14)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !almostEqual(v, 0.0, floatTol) {
		t.Fatalf("expected RSI=0 for all-loss series, got %f", v)
	}
}

// ---------- Volume Profile tests ----------

// makeVPBar builds a single OHLCV bar with explicit OHLCV fields.
func makeVPBar(open, high, low, close, volume float64) models.OHLCV {
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

// TestVolumeProfile_Basic verifies that POC lands in the price region carrying
// the heaviest volume and that HVN / LVN slices have the expected lengths.
func TestVolumeProfile_Basic(t *testing.T) {
	// Ten bars: first 7 carry heavy volume clustered around 103-107; the last 3
	// carry light volume at the extremes so we get clear LVN candidates.
	candles := []models.OHLCV{
		makeVPBar(100, 106, 99, 105, 100),  // light, wide range
		makeVPBar(104, 107, 103, 106, 500), // heavy around 103-107
		makeVPBar(105, 108, 104, 107, 500), // heavy around 104-108
		makeVPBar(106, 107, 105, 106, 400), // heavy around 105-107
		makeVPBar(103, 110, 100, 108, 100), // spread vol
		makeVPBar(107, 109, 106, 108, 300), // vol around 106-109
		makeVPBar(104, 107, 103, 106, 450), // heavy around 103-107
		makeVPBar(100, 101, 99, 100, 10),   // light at the low end
		makeVPBar(109, 110, 108, 110, 10),  // light at the high end
		makeVPBar(100, 101, 99, 101, 10),   // light at the low end
	}

	poc, hvns, lvns, ok := volumeProfile(candles, 20)
	if !ok {
		t.Fatal("expected ok=true, got false")
	}

	// POC should sit in the high-volume region 103-109.
	if poc < 103 || poc > 109 {
		t.Errorf("expected POC in [103, 109], got %.2f", poc)
	}

	// HVN: up to 3, all non-nil.
	if len(hvns) == 0 {
		t.Error("expected at least one HVN")
	}
	if len(hvns) > 3 {
		t.Errorf("expected at most 3 HVNs, got %d", len(hvns))
	}

	// LVN: up to 3, all non-nil.
	if len(lvns) == 0 {
		t.Error("expected at least one LVN")
	}
	if len(lvns) > 3 {
		t.Errorf("expected at most 3 LVNs, got %d", len(lvns))
	}

	t.Logf("POC=%.4f  HVNs=%v  LVNs=%v", poc, hvns, lvns)
}

// TestVolumeProfile_InsufficientData verifies that fewer than 10 bars returns ok=false.
func TestVolumeProfile_InsufficientData(t *testing.T) {
	tests := []struct {
		name string
		n    int
	}{
		{"zero bars", 0},
		{"one bar", 1},
		{"nine bars", 9}, // boundary: 9 < 10 must be false
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candles := make([]models.OHLCV, tc.n)
			for i := range candles {
				candles[i] = makeVPBar(100, 105, 99, 102, 100)
			}
			_, _, _, ok := volumeProfile(candles, 20)
			if ok {
				t.Errorf("expected ok=false for %d candles", tc.n)
			}
		})
	}

	// Exactly 10 bars must succeed.
	tenBars := make([]models.OHLCV, 10)
	for i := range tenBars {
		tenBars[i] = makeVPBar(100+float64(i), 101+float64(i), 99+float64(i), 100+float64(i), 100)
	}
	_, _, _, ok := volumeProfile(tenBars, 20)
	if !ok {
		t.Error("expected ok=true for exactly 10 candles")
	}
}

// TestVolumeProfile_Integration calls Compute() and verifies VP_POC is present
// and within the price range used to build the bars.
func TestVolumeProfile_Integration(t *testing.T) {
	// Build 12 bars with varying volume so the profile has meaningful structure.
	candles := []models.OHLCV{
		makeVPBar(100, 105, 99, 104, 100),
		makeVPBar(101, 106, 100, 105, 200),
		makeVPBar(102, 107, 101, 106, 300),
		makeVPBar(103, 108, 102, 107, 200),
		makeVPBar(104, 109, 103, 108, 100),
		makeVPBar(105, 110, 104, 109, 150),
		makeVPBar(103, 107, 102, 106, 400),
		makeVPBar(104, 108, 103, 107, 350),
		makeVPBar(100, 103, 99, 102, 50),
		makeVPBar(108, 110, 107, 109, 50),
		makeVPBar(101, 105, 100, 104, 180),
		makeVPBar(102, 106, 101, 105, 220),
	}

	bars := map[string][]models.OHLCV{"1H": candles}
	result := Compute(bars)

	// VP_POC must be present.
	poc, exists := result["1H:VP_POC"]
	if !exists {
		t.Fatal("expected 1H:VP_POC key in Compute() output")
	}

	// POC must be within the overall price range [99, 110].
	if poc < 99 || poc > 110 {
		t.Errorf("VP_POC=%.4f outside expected range [99, 110]", poc)
	}

	// Log all VP keys for visibility.
	for k, v := range result {
		if len(k) >= 7 && k[3:6] == "VP_" {
			t.Logf("%s = %.4f", k, v)
		}
	}

	// Any HVN/LVN keys that are present must also be within the price range.
	for i := 1; i <= 3; i++ {
		if v, ok := result[fmt.Sprintf("1H:VP_HVN_%d", i)]; ok {
			if v < 99 || v > 110 {
				t.Errorf("VP_HVN_%d=%.4f outside range [99, 110]", i, v)
			}
		}
		if v, ok := result[fmt.Sprintf("1H:VP_LVN_%d", i)]; ok {
			if v < 99 || v > 110 {
				t.Errorf("VP_LVN_%d=%.4f outside range [99, 110]", i, v)
			}
		}
	}
}
