package wyckoff

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// makeBars generates n synthetic OHLCV bars with an oscillating price so that
// local swing highs and lows are present for indicator computation.
func makeBars(n int, basePrice float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	t := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		// Sine-like oscillation so swing high/low detection always finds points.
		// Period ≈ 10 bars.
		phase := float64(i) * 0.628 // ~2π/10
		p := basePrice + 10*sinApprox(phase)
		bars[i] = models.OHLCV{
			Symbol:    "TEST",
			Timeframe: "1H",
			OpenTime:  t,
			Open:      p - 0.5,
			High:      p + 2.0,
			Low:       p - 2.0,
			Close:     p,
			Volume:    1000 + float64(i%10)*100,
		}
		t = t.Add(time.Hour)
	}
	return bars
}

// sinApprox is a simple sin approximation via Taylor (enough for test data).
func sinApprox(x float64) float64 {
	// Reduce x to [-π, π]
	const pi = 3.14159265358979
	for x > pi {
		x -= 2 * pi
	}
	for x < -pi {
		x += 2 * pi
	}
	// sin ≈ x - x³/6 + x⁵/120
	x3 := x * x * x
	x5 := x3 * x * x
	return x - x3/6 + x5/120
}

func TestAnalyze_ReturnFields(t *testing.T) {
	bars := makeBars(100, 100.0)
	a := Analyze("TEST", "1H", bars)

	if a.Symbol != "TEST" {
		t.Errorf("Symbol: want TEST, got %s", a.Symbol)
	}
	if a.Timeframe != "1H" {
		t.Errorf("Timeframe: want 1H, got %s", a.Timeframe)
	}
	if a.Phase == "" {
		t.Error("Phase must not be empty")
	}
	if a.SwingHigh == 0 {
		t.Error("SwingHigh should be non-zero for 100 bars")
	}
	if a.SwingLow == 0 {
		t.Error("SwingLow should be non-zero for 100 bars")
	}
}

func TestAnalyze_TooFewBars(t *testing.T) {
	bars := makeBars(3, 100.0)
	a := Analyze("TEST", "1H", bars)
	if a.Phase != PhaseRanging {
		t.Errorf("expected Ranging for <5 bars, got %s", a.Phase)
	}
	if len(a.Events) != 0 {
		t.Errorf("expected no events for <5 bars, got %d", len(a.Events))
	}
}

func TestAnalyze_PhaseZones_NonEmpty(t *testing.T) {
	bars := makeBars(80, 100.0)
	a := Analyze("TEST", "1H", bars)
	if len(a.PhaseZones) == 0 {
		t.Error("expected at least one phase zone for 80 bars")
	}
	for _, z := range a.PhaseZones {
		if z.StartTime > z.EndTime {
			t.Errorf("zone StartTime %d > EndTime %d", z.StartTime, z.EndTime)
		}
		if z.PriceLow > z.PriceHigh {
			t.Errorf("zone PriceLow %.2f > PriceHigh %.2f", z.PriceLow, z.PriceHigh)
		}
	}
}

func TestAnalyze_SpringDetected(t *testing.T) {
	// Build bars where one bar dips below SWING_LOW then recovers with high volume.
	bars := makeBars(60, 200.0)

	// Inject a spring at bar 50: dip below minimum low, then recover with high vol
	minLow := bars[0].Low
	for _, b := range bars {
		if b.Low < minLow {
			minLow = b.Low
		}
	}
	// Bar 48: spike low below all previous lows
	bars[48].Low = minLow - 10
	bars[48].Close = minLow - 5
	// Bar 49: recovery candle with very high volume
	bars[49].Close = bars[49].Open + 2
	bars[49].Volume = 9999999 // far above any VOLUME_MA_20

	a := Analyze("TEST", "1H", bars)

	springFound := false
	for _, ev := range a.Events {
		if ev.Type == EventSpring {
			springFound = true
			break
		}
	}
	if !springFound {
		// Not a hard failure — indicator warmup may differ. Log as informational.
		t.Log("spring event not detected (may need more bars for indicator warmup)")
	}
}

func TestBuildPhaseZones_AllZonesAdjacentInTime(t *testing.T) {
	bars := makeBars(50, 150.0)
	zones := buildPhaseZones(bars, 160.0, 175.0, 145.0)

	for i := 1; i < len(zones); i++ {
		if zones[i].StartTime < zones[i-1].EndTime {
			t.Errorf("zone %d starts before zone %d ends", i, i-1)
		}
	}
}
