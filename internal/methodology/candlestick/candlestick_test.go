package candlestick

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── Test helpers ─────────────────────────────────────────────────────────────

// makeBars creates n OHLCV bars starting at startClose, moving by step each bar.
// Positive step = uptrend, negative step = downtrend.
func makeBars(n int, startClose, step float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	for i := range bars {
		c := startClose + float64(i)*step
		o := c - step/2
		h := c + 1
		l := o - 1
		if l > c {
			l = c - 1
		}
		if h < o {
			h = o + 1
		}
		bars[i] = models.OHLCV{
			Symbol:    "TEST",
			Timeframe: "1D",
			OpenTime:  time.Now(),
			Open:      o,
			High:      h,
			Low:       l,
			Close:     c,
			Volume:    1000,
		}
	}
	return bars
}

func makeCtx(bars []models.OHLCV) models.AnalysisContext {
	return models.AnalysisContext{
		Symbol:     "TEST",
		Timeframes: map[string][]models.OHLCV{"1D": bars},
		Indicators: map[string]float64{},
	}
}

// ── DojiRule ─────────────────────────────────────────────────────────────────

func TestDojiRule_Name(t *testing.T) {
	r := &DojiRule{}
	if r.Name() != "doji" {
		t.Fatalf("expected doji, got %s", r.Name())
	}
}

func TestDojiRule_Downtrend_LONG(t *testing.T) {
	r := &DojiRule{}
	// Downtrend bars then a doji
	bars := makeBars(9, 110, -2) // 110, 108, 106, ... 94
	// Last bar: doji with tiny body and large range
	bars = append(bars, models.OHLCV{Open: 92.0, High: 97.0, Low: 87.0, Close: 92.1, Volume: 1000})

	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
}

func TestDojiRule_Uptrend_SHORT(t *testing.T) {
	r := &DojiRule{}
	bars := makeBars(9, 90, 2) // 90, 92, 94, ... 106
	// Doji at end
	bars = append(bars, models.OHLCV{Open: 108.0, High: 113.0, Low: 103.0, Close: 108.1, Volume: 1000})

	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestDojiRule_NoPattern_LargeBody(t *testing.T) {
	r := &DojiRule{}
	bars := makeBars(9, 110, -2)
	// Big body, not a doji
	bars = append(bars, models.OHLCV{Open: 90.0, High: 100.0, Low: 85.0, Close: 98.0, Volume: 1000})

	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil for large body candle")
	}
}

// ── HammerRule ────────────────────────────────────────────────────────────────

func TestHammerRule_Name(t *testing.T) {
	r := &HammerRule{}
	if r.Name() != "hammer" {
		t.Fatalf("expected hammer, got %s", r.Name())
	}
}

func TestHammerRule_Downtrend_Detected(t *testing.T) {
	r := &HammerRule{}
	bars := makeBars(9, 110, -2)
	// Hammer: small body near top, long lower shadow
	// bodyRatio ~= 2/12 = 0.167, lowerShadow=8, upperShadow=2, body=2
	// lowerShadow(8) > 2*body(4)? 8>4 yes
	// upperShadow(2) < body(2)? 2<2 no — need to adjust
	// Better: Open=96, Close=98, High=99, Low=86 → body=2, range=13, br=0.154
	// upper=99-98=1, lower=96-86=10 → 10>4 yes, 1<2 yes
	bars = append(bars, models.OHLCV{Open: 96, High: 99, Low: 86, Close: 98, Volume: 1000})

	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
}

func TestHammerRule_Uptrend_NotDetected(t *testing.T) {
	r := &HammerRule{}
	bars := makeBars(9, 90, 2)
	bars = append(bars, models.OHLCV{Open: 106, High: 109, Low: 96, Close: 108, Volume: 1000})

	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — hammer requires downtrend")
	}
}

// ── HangingManRule ───────────────────────────────────────────────────────────

func TestHangingManRule_Name(t *testing.T) {
	r := &HangingManRule{}
	if r.Name() != "hanging_man" {
		t.Fatalf("expected hanging_man, got %s", r.Name())
	}
}

func TestHangingManRule_Uptrend_Detected(t *testing.T) {
	r := &HangingManRule{}
	bars := makeBars(9, 90, 2)
	// Hanging man shape after uptrend
	// Open=106, Close=108, High=109, Low=96 → body=2, range=13, br=0.154
	// lower=106-96=10 > 2*2=4, upper=109-108=1 < 2
	bars = append(bars, models.OHLCV{Open: 106, High: 109, Low: 96, Close: 108, Volume: 1000})

	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestHangingManRule_Downtrend_NotDetected(t *testing.T) {
	r := &HangingManRule{}
	bars := makeBars(9, 110, -2)
	bars = append(bars, models.OHLCV{Open: 96, High: 99, Low: 86, Close: 98, Volume: 1000})

	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — hanging man requires uptrend")
	}
}

// ── ShootingStarRule ─────────────────────────────────────────────────────────

func TestShootingStarRule_Name(t *testing.T) {
	r := &ShootingStarRule{}
	if r.Name() != "shooting_star" {
		t.Fatalf("expected shooting_star, got %s", r.Name())
	}
}

func TestShootingStarRule_Uptrend_Detected(t *testing.T) {
	r := &ShootingStarRule{}
	bars := makeBars(9, 90, 2)
	// Shooting star: small body near bottom, long upper shadow
	// Open=108, Close=106, High=118, Low=105 → body=2, range=13
	// upper=118-108=10 > 2*2=4 yes, lower=106-105=1 < 2 yes, br=2/13=0.154
	bars = append(bars, models.OHLCV{Open: 108, High: 118, Low: 105, Close: 106, Volume: 1000})

	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestShootingStarRule_Downtrend_NotDetected(t *testing.T) {
	r := &ShootingStarRule{}
	bars := makeBars(9, 110, -2)
	bars = append(bars, models.OHLCV{Open: 98, High: 108, Low: 95, Close: 96, Volume: 1000})

	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — shooting star requires uptrend")
	}
}

// ── InvertedHammerRule ───────────────────────────────────────────────────────

func TestInvertedHammerRule_Name(t *testing.T) {
	r := &InvertedHammerRule{}
	if r.Name() != "inverted_hammer" {
		t.Fatalf("expected inverted_hammer, got %s", r.Name())
	}
}

func TestInvertedHammerRule_Downtrend_Detected(t *testing.T) {
	r := &InvertedHammerRule{}
	bars := makeBars(9, 110, -2)
	// Inverted hammer shape after downtrend: long upper shadow, small lower shadow
	// Open=94, Close=96, High=108, Low=93 → body=2, range=15, br=0.133
	// upper=108-96=12 > 2*2=4 yes, lower=94-93=1 < 2 yes
	bars = append(bars, models.OHLCV{Open: 94, High: 108, Low: 93, Close: 96, Volume: 1000})

	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
}

func TestInvertedHammerRule_Uptrend_NotDetected(t *testing.T) {
	r := &InvertedHammerRule{}
	bars := makeBars(9, 90, 2)
	bars = append(bars, models.OHLCV{Open: 104, High: 118, Low: 103, Close: 106, Volume: 1000})

	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — inverted hammer requires downtrend")
	}
}

// ── MarubozuRule ─────────────────────────────────────────────────────────────

func TestMarubozuRule_Name(t *testing.T) {
	r := &MarubozuRule{}
	if r.Name() != "marubozu" {
		t.Fatalf("expected marubozu, got %s", r.Name())
	}
}

func TestMarubozuRule_Bullish_Detected(t *testing.T) {
	r := &MarubozuRule{}
	// body=9.5, range=10, br=0.95 > 0.90
	bars := []models.OHLCV{
		{Open: 100, High: 109.8, Low: 99.8, Close: 109.5, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
}

func TestMarubozuRule_Bearish_Detected(t *testing.T) {
	r := &MarubozuRule{}
	// body=9.5, range=10, br=0.95
	bars := []models.OHLCV{
		{Open: 109.5, High: 109.8, Low: 99.8, Close: 100, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestMarubozuRule_SmallBody_NotDetected(t *testing.T) {
	r := &MarubozuRule{}
	// body=2, range=10, br=0.20
	bars := []models.OHLCV{
		{Open: 100, High: 108, Low: 98, Close: 102, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil for small body ratio")
	}
}

// ── BullishEngulfingRule ─────────────────────────────────────────────────────

func TestBullishEngulfingRule_Name(t *testing.T) {
	r := &BullishEngulfingRule{}
	if r.Name() != "bullish_engulfing" {
		t.Fatalf("expected bullish_engulfing, got %s", r.Name())
	}
}

func TestBullishEngulfingRule_Detected(t *testing.T) {
	r := &BullishEngulfingRule{}
	bars := []models.OHLCV{
		// prev: bearish, Open=105 Close=100 → body=5
		{Open: 105, High: 106, Low: 99, Close: 100, Volume: 1000},
		// curr: bullish, Open=98 Close=108 → body=10, engulfs prev body [100,105]
		// curr.Open(98) <= prev.Close(100) yes, curr.Close(108) >= prev.Open(105) yes
		{Open: 98, High: 109, Low: 97, Close: 108, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
}

func TestBullishEngulfingRule_NotEngulfing(t *testing.T) {
	r := &BullishEngulfingRule{}
	bars := []models.OHLCV{
		{Open: 105, High: 106, Low: 99, Close: 100, Volume: 1000},
		// curr body smaller than prev → not engulfing
		{Open: 101, High: 104, Low: 100, Close: 103, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — curr body does not engulf prev")
	}
}

// ── BearishEngulfingRule ─────────────────────────────────────────────────────

func TestBearishEngulfingRule_Name(t *testing.T) {
	r := &BearishEngulfingRule{}
	if r.Name() != "bearish_engulfing" {
		t.Fatalf("expected bearish_engulfing, got %s", r.Name())
	}
}

func TestBearishEngulfingRule_Detected(t *testing.T) {
	r := &BearishEngulfingRule{}
	bars := []models.OHLCV{
		// prev: bullish, Open=100 Close=105 → body=5
		{Open: 100, High: 106, Low: 99, Close: 105, Volume: 1000},
		// curr: bearish, Open=108 Close=97 → body=11, engulfs prev body [100,105]
		// curr.Open(108) >= prev.Close(105) yes, curr.Close(97) <= prev.Open(100) yes
		{Open: 108, High: 109, Low: 96, Close: 97, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestBearishEngulfingRule_NotEngulfing(t *testing.T) {
	r := &BearishEngulfingRule{}
	bars := []models.OHLCV{
		{Open: 100, High: 106, Low: 99, Close: 105, Volume: 1000},
		// curr body smaller
		{Open: 104, High: 105, Low: 101, Close: 102, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — curr body does not engulf prev")
	}
}

// ── BullishHaramiRule ────────────────────────────────────────────────────────

func TestBullishHaramiRule_Name(t *testing.T) {
	r := &BullishHaramiRule{}
	if r.Name() != "bullish_harami" {
		t.Fatalf("expected bullish_harami, got %s", r.Name())
	}
}

func TestBullishHaramiRule_Detected(t *testing.T) {
	r := &BullishHaramiRule{}
	bars := []models.OHLCV{
		// prev: large bearish, Open=110 Close=95 → body=15, range=17, br=0.882
		{Open: 110, High: 112, Low: 93, Close: 95, Volume: 1000},
		// curr: small bullish inside prev body, Open=97 Close=100 → body=3 < 15*0.5=7.5
		// curr.Open(97) > min(110,95)=95 yes, curr.Close(100) < max(110,95)=110 yes
		{Open: 97, High: 101, Low: 96, Close: 100, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
}

func TestBullishHaramiRule_CurrTooLarge(t *testing.T) {
	r := &BullishHaramiRule{}
	bars := []models.OHLCV{
		{Open: 110, High: 112, Low: 93, Close: 95, Volume: 1000},
		// curr body too large: 12 >= 15*0.5=7.5
		{Open: 96, High: 109, Low: 95, Close: 108, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — curr body too large for harami")
	}
}

// ── BearishHaramiRule ────────────────────────────────────────────────────────

func TestBearishHaramiRule_Name(t *testing.T) {
	r := &BearishHaramiRule{}
	if r.Name() != "bearish_harami" {
		t.Fatalf("expected bearish_harami, got %s", r.Name())
	}
}

func TestBearishHaramiRule_Detected(t *testing.T) {
	r := &BearishHaramiRule{}
	bars := []models.OHLCV{
		// prev: large bullish, Open=90 Close=110 → body=20, range=22, br=0.909
		{Open: 90, High: 112, Low: 88, Close: 110, Volume: 1000},
		// curr: small bearish inside prev body, Open=105 Close=95 → body=10... too big
		// body must be < 20*0.5=10 → use Open=104 Close=98 → body=6
		// curr.Open(104) < max(90,110)=110 yes, curr.Close(98) > min(90,110)=90 yes
		{Open: 104, High: 105, Low: 97, Close: 98, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestBearishHaramiRule_PrevNotBullish(t *testing.T) {
	r := &BearishHaramiRule{}
	bars := []models.OHLCV{
		// prev: bearish
		{Open: 110, High: 112, Low: 88, Close: 90, Volume: 1000},
		{Open: 104, High: 105, Low: 97, Close: 98, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — prev must be bullish")
	}
}

// ── MorningStarRule ──────────────────────────────────────────────────────────

func TestMorningStarRule_Name(t *testing.T) {
	r := &MorningStarRule{}
	if r.Name() != "morning_star" {
		t.Fatalf("expected morning_star, got %s", r.Name())
	}
}

func TestMorningStarRule_Detected(t *testing.T) {
	r := &MorningStarRule{}
	bars := []models.OHLCV{
		// first: large bearish, Open=110 Close=90 → body=20, range=22, br=0.909
		{Open: 110, High: 112, Low: 88, Close: 90, Volume: 1000},
		// star: small body, Open=89 Close=89.5 → body=0.5, range=5, br=0.10
		{Open: 89, High: 91, Low: 86, Close: 89.5, Volume: 1000},
		// last: bullish, Open=91 Close=105 → body=14, range=16, br=0.875
		// close(105) > midpoint(first)= (110+90)/2 = 100 yes
		{Open: 91, High: 106, Low: 90, Close: 105, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
	if sig.Score != 4.0 {
		t.Fatalf("expected score 4.0, got %f", sig.Score)
	}
}

func TestMorningStarRule_StarTooLarge(t *testing.T) {
	r := &MorningStarRule{}
	bars := []models.OHLCV{
		{Open: 110, High: 112, Low: 88, Close: 90, Volume: 1000},
		// star with large body: br > 0.25
		{Open: 85, High: 92, Low: 84, Close: 91, Volume: 1000}, // body=6, range=8, br=0.75
		{Open: 91, High: 106, Low: 90, Close: 105, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — star body too large")
	}
}

// ── EveningStarRule ──────────────────────────────────────────────────────────

func TestEveningStarRule_Name(t *testing.T) {
	r := &EveningStarRule{}
	if r.Name() != "evening_star" {
		t.Fatalf("expected evening_star, got %s", r.Name())
	}
}

func TestEveningStarRule_Detected(t *testing.T) {
	r := &EveningStarRule{}
	bars := []models.OHLCV{
		// first: large bullish, Open=90 Close=110 → body=20, range=22, br=0.909
		{Open: 90, High: 112, Low: 88, Close: 110, Volume: 1000},
		// star: small body, Open=111 Close=111.5 → body=0.5, range=5, br=0.10
		{Open: 111, High: 114, Low: 109, Close: 111.5, Volume: 1000},
		// last: bearish, Open=109 Close=95 → body=14, range=16, br=0.875
		// close(95) < midpoint(first)=(110+90)/2=100 yes
		{Open: 109, High: 110, Low: 94, Close: 95, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestEveningStarRule_FirstNotBullish(t *testing.T) {
	r := &EveningStarRule{}
	bars := []models.OHLCV{
		// first: bearish
		{Open: 110, High: 112, Low: 88, Close: 90, Volume: 1000},
		{Open: 89, High: 91, Low: 86, Close: 89.5, Volume: 1000},
		{Open: 109, High: 110, Low: 94, Close: 95, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — first candle must be bullish")
	}
}

// ── ThreeWhiteSoldiersRule ───────────────────────────────────────────────────

func TestThreeWhiteSoldiersRule_Name(t *testing.T) {
	r := &ThreeWhiteSoldiersRule{}
	if r.Name() != "three_white_soldiers" {
		t.Fatalf("expected three_white_soldiers, got %s", r.Name())
	}
}

func TestThreeWhiteSoldiersRule_Detected(t *testing.T) {
	r := &ThreeWhiteSoldiersRule{}
	bars := []models.OHLCV{
		// b1: bullish, Open=100 Close=108, body=8, range=12, br=0.667
		{Open: 100, High: 110, Low: 98, Close: 108, Volume: 1000},
		// b2: bullish, Open=104 Close=115, body=11, range=14, br=0.786
		// b2.Open(104) > min(100,108)=100 yes, b2.Open(104) < b1.Close(108) yes
		{Open: 104, High: 116, Low: 102, Close: 115, Volume: 1000},
		// b3: bullish, Open=112 Close=122, body=10, range=12, br=0.833
		// b3.Open(112) > min(104,115)=104 yes, b3.Open(112) < b2.Close(115) yes
		// closes: 108 < 115 < 122 yes
		{Open: 112, High: 123, Low: 111, Close: 122, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected LONG signal, got nil")
	}
	if sig.Direction != "LONG" {
		t.Fatalf("expected LONG, got %s", sig.Direction)
	}
}

func TestThreeWhiteSoldiersRule_OneBearish(t *testing.T) {
	r := &ThreeWhiteSoldiersRule{}
	bars := []models.OHLCV{
		{Open: 100, High: 110, Low: 98, Close: 108, Volume: 1000},
		// b2: bearish
		{Open: 115, High: 116, Low: 102, Close: 104, Volume: 1000},
		{Open: 112, High: 123, Low: 111, Close: 122, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — all three must be bullish")
	}
}

// ── ThreeBlackCrowsRule ──────────────────────────────────────────────────────

func TestThreeBlackCrowsRule_Name(t *testing.T) {
	r := &ThreeBlackCrowsRule{}
	if r.Name() != "three_black_crows" {
		t.Fatalf("expected three_black_crows, got %s", r.Name())
	}
}

func TestThreeBlackCrowsRule_Detected(t *testing.T) {
	r := &ThreeBlackCrowsRule{}
	bars := []models.OHLCV{
		// b1: bearish, Open=120 Close=110 → body=10, range=14, br=0.714
		{Open: 120, High: 122, Low: 108, Close: 110, Volume: 1000},
		// b2: bearish, Open=115 Close=102 → body=13, range=15, br=0.867
		// b2.Open(115) < max(120,110)=120 yes, b2.Open(115) > b1.Close(110) yes
		{Open: 115, High: 116, Low: 101, Close: 102, Volume: 1000},
		// b3: bearish, Open=106 Close=93 → body=13, range=15, br=0.867
		// b3.Open(106) < max(115,102)=115 yes, b3.Open(106) > b2.Close(102) yes
		// closes: 110 > 102 > 93 yes
		{Open: 106, High: 108, Low: 91, Close: 93, Volume: 1000},
	}
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected SHORT signal, got nil")
	}
	if sig.Direction != "SHORT" {
		t.Fatalf("expected SHORT, got %s", sig.Direction)
	}
}

func TestThreeBlackCrowsRule_ClosesNotDescending(t *testing.T) {
	r := &ThreeBlackCrowsRule{}
	bars := []models.OHLCV{
		{Open: 120, High: 122, Low: 108, Close: 110, Volume: 1000},
		// b2 close higher than b1 close
		{Open: 115, High: 116, Low: 110, Close: 112, Volume: 1000},
		{Open: 106, High: 108, Low: 91, Close: 93, Volume: 1000},
	}
	sig, _ := r.Analyze(makeCtx(bars))
	if sig != nil {
		t.Fatal("expected nil — closes must be descending")
	}
}

// ── Multi-timeframe selection ────────────────────────────────────────────────

func TestMultiTF_HigherWeightWins(t *testing.T) {
	r := &MarubozuRule{}
	bar := models.OHLCV{Open: 100, High: 109.8, Low: 99.8, Close: 109.5, Volume: 1000}
	ctx := models.AnalysisContext{
		Symbol: "TEST",
		Timeframes: map[string][]models.OHLCV{
			"1H": {bar},
			"1D": {bar},
		},
		Indicators: map[string]float64{},
	}
	sig, err := r.Analyze(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Timeframe != "1D" {
		t.Fatalf("expected 1D (higher weight), got %s", sig.Timeframe)
	}
}

// ── RequiredIndicators ───────────────────────────────────────────────────────

func TestAllRules_RequiredIndicators_Nil(t *testing.T) {
	rules := []interface {
		RequiredIndicators() []string
	}{
		&DojiRule{}, &HammerRule{}, &HangingManRule{},
		&ShootingStarRule{}, &InvertedHammerRule{}, &MarubozuRule{},
		&BullishEngulfingRule{}, &BearishEngulfingRule{},
		&BullishHaramiRule{}, &BearishHaramiRule{},
		&MorningStarRule{}, &EveningStarRule{},
		&ThreeWhiteSoldiersRule{}, &ThreeBlackCrowsRule{},
	}
	for _, r := range rules {
		if r.RequiredIndicators() != nil {
			t.Errorf("expected nil RequiredIndicators for %T", r)
		}
	}
}

// ── Insufficient bars ────────────────────────────────────────────────────────

func TestDojiRule_InsufficientBars(t *testing.T) {
	r := &DojiRule{}
	bars := makeBars(3, 100, -1)
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig != nil {
		t.Fatal("expected nil for insufficient bars")
	}
}

func TestHammerRule_InsufficientBars(t *testing.T) {
	r := &HammerRule{}
	bars := makeBars(4, 100, -1)
	sig, err := r.Analyze(makeCtx(bars))
	if err != nil {
		t.Fatal(err)
	}
	if sig != nil {
		t.Fatal("expected nil for insufficient bars")
	}
}
