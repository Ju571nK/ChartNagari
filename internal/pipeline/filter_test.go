package pipeline

import (
	"math"
	"testing"

	"github.com/Ju571nK/Chatter/internal/wyckoff"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func makeSignal(direction, timeframe string) models.Signal {
	return models.Signal{
		Direction: direction,
		Timeframe: timeframe,
		Symbol:    "TEST",
		Rule:      "test_rule",
	}
}

func makeSignalWithScore(direction, timeframe string, score float64) models.Signal {
	s := makeSignal(direction, timeframe)
	s.Score = score
	return s
}

func TestFilterMTFConsensus_MinTFs1(t *testing.T) {
	// minTFs=1: all non-NEUTRAL signals pass
	signals := []models.Signal{
		makeSignal("LONG", "1H"),
		makeSignal("SHORT", "4H"),
		makeSignal("NEUTRAL", "1D"),
	}
	result := filterMTFConsensus(signals, 1)
	if len(result) != 3 {
		t.Errorf("expected 3 signals, got %d", len(result))
	}
}

func TestFilterMTFConsensus_MinTFs2_Passes(t *testing.T) {
	// minTFs=2: direction with signals in ≥2 TFs passes
	signals := []models.Signal{
		makeSignal("SHORT", "1H"),
		makeSignal("SHORT", "4H"),
		makeSignal("LONG", "1D"),
	}
	result := filterMTFConsensus(signals, 2)
	// SHORT has 2 TFs → passes; LONG has only 1 TF → filtered
	for _, sig := range result {
		if sig.Direction == "LONG" {
			t.Errorf("LONG signal should have been filtered out (only 1 TF)")
		}
	}
	shortCount := 0
	for _, sig := range result {
		if sig.Direction == "SHORT" {
			shortCount++
		}
	}
	if shortCount != 2 {
		t.Errorf("expected 2 SHORT signals to pass, got %d", shortCount)
	}
}

func TestFilterMTFConsensus_NeutralAlwaysPasses(t *testing.T) {
	// NEUTRAL signals always pass regardless of minTFs
	signals := []models.Signal{
		makeSignal("NEUTRAL", "1H"),
		makeSignal("LONG", "1H"), // only 1 TF for LONG
	}
	result := filterMTFConsensus(signals, 2)
	neutralFound := false
	for _, sig := range result {
		if sig.Direction == "NEUTRAL" {
			neutralFound = true
		}
	}
	if !neutralFound {
		t.Error("NEUTRAL signal should always pass")
	}
	// LONG should be filtered (only 1 TF)
	for _, sig := range result {
		if sig.Direction == "LONG" {
			t.Error("LONG with single TF should be filtered when minTFs=2")
		}
	}
}

func TestFilterMTFConsensus_EmptyList(t *testing.T) {
	// Empty signal list: returns empty
	result := filterMTFConsensus([]models.Signal{}, 2)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestFilterMTFConsensus_AllSameTF_FilteredWhenMinTFs2(t *testing.T) {
	// All same TF: filtered when minTFs=2
	signals := []models.Signal{
		makeSignal("LONG", "1H"),
		makeSignal("LONG", "1H"),
	}
	result := filterMTFConsensus(signals, 2)
	// Both are LONG but only 1 distinct TF → all filtered
	for _, sig := range result {
		if sig.Direction == "LONG" {
			t.Error("LONG signals with only 1 distinct TF should be filtered when minTFs=2")
		}
	}
	if len(result) != 0 {
		t.Errorf("expected 0 signals, got %d", len(result))
	}
}

// ── penalizeHTFContext tests ──────────────────────────────────────────────────

// htfIndicators creates indicator map with EMA_50 > EMA_200 for the given TF
// to simulate a LONG HTF trend.
func htfIndicatorsLong(tf string) map[string]float64 {
	return map[string]float64{
		tf + ":EMA_50":  110,
		tf + ":EMA_200": 100,
		tf + ":ADX_14":  30, // strong trend
	}
}

func htfBarsAboveEMA(tf string) map[string][]models.OHLCV {
	return map[string][]models.OHLCV{
		tf: {{Close: 115}}, // price above EMA_50
	}
}

func TestPenalizeHTFContext_ZeroPenalty_PassAll(t *testing.T) {
	signals := []models.Signal{
		makeSignalWithScore("SHORT", "1H", 10.0), // counter-trend vs LONG HTF
		makeSignalWithScore("LONG", "1H", 10.0),
	}
	indicators := htfIndicatorsLong("1D")
	bars := htfBarsAboveEMA("1D")

	result := penalizeHTFContext(signals, indicators, bars, "", 0)
	if len(result) != 2 {
		t.Fatalf("penalty=0 should pass all signals, got %d", len(result))
	}
	// Scores should be unchanged
	for _, sig := range result {
		if sig.Score != 10.0 {
			t.Errorf("score should be unchanged at 10.0, got %.1f", sig.Score)
		}
	}
}

func TestPenalizeHTFContext_FullSuppress(t *testing.T) {
	signals := []models.Signal{
		makeSignalWithScore("SHORT", "1H", 10.0), // counter-trend
		makeSignalWithScore("LONG", "1H", 10.0),  // aligned
	}
	indicators := htfIndicatorsLong("1D")
	bars := htfBarsAboveEMA("1D")

	result := penalizeHTFContext(signals, indicators, bars, "", 100)
	if len(result) != 1 {
		t.Fatalf("penalty=100 should remove counter-trend, expected 1, got %d", len(result))
	}
	if result[0].Direction != "LONG" {
		t.Errorf("expected LONG signal to survive, got %s", result[0].Direction)
	}
}

func TestPenalizeHTFContext_PartialPenalty(t *testing.T) {
	signals := []models.Signal{
		makeSignalWithScore("SHORT", "1H", 10.0), // counter-trend
		makeSignalWithScore("LONG", "1H", 10.0),  // aligned
	}
	indicators := htfIndicatorsLong("1D")
	bars := htfBarsAboveEMA("1D")

	result := penalizeHTFContext(signals, indicators, bars, "", 50)
	if len(result) != 2 {
		t.Fatalf("penalty=50 should keep all signals, got %d", len(result))
	}
	for _, sig := range result {
		if sig.Direction == "SHORT" && math.Abs(sig.Score-5.0) > 0.01 {
			t.Errorf("SHORT score should be 5.0 (50%% penalty on 10), got %.1f", sig.Score)
		}
		if sig.Direction == "LONG" && sig.Score != 10.0 {
			t.Errorf("LONG score should remain 10.0, got %.1f", sig.Score)
		}
	}
}

func TestPenalizeHTFContext_HTFSignalsNeverPenalized(t *testing.T) {
	signals := []models.Signal{
		makeSignalWithScore("SHORT", "1D", 10.0), // HTF signal, counter-trend vs 1W
		makeSignalWithScore("SHORT", "1W", 10.0), // HTF signal
	}
	// Set up 1W as LONG trend
	indicators := htfIndicatorsLong("1W")
	bars := htfBarsAboveEMA("1W")

	result := penalizeHTFContext(signals, indicators, bars, "", 100)
	// 1D and 1W signals should never be filtered
	if len(result) != 2 {
		t.Fatalf("HTF signals should never be penalized, expected 2, got %d", len(result))
	}
}

func TestPenalizeHTFContext_WyckoffOverride(t *testing.T) {
	signals := []models.Signal{
		makeSignalWithScore("LONG", "1H", 10.0), // would be counter-trend if SHORT
	}
	// Set up SHORT trend
	indicators := map[string]float64{
		"1D:EMA_50":  90,
		"1D:EMA_200": 100,
		"1D:ADX_14":  30,
	}
	bars := map[string][]models.OHLCV{
		"1D": {{Close: 85}}, // price below EMA_50
	}

	// Without Wyckoff: LONG counter-trend would be penalized
	result := penalizeHTFContext(signals, indicators, bars, "", 100)
	if len(result) != 0 {
		t.Fatalf("without Wyckoff, counter-trend LONG should be removed, got %d", len(result))
	}

	// With accumulation: LONG should pass through (Wyckoff override)
	signals2 := []models.Signal{makeSignalWithScore("LONG", "1H", 10.0)}
	result2 := penalizeHTFContext(signals2, indicators, bars, wyckoff.PhaseAccumulation, 100)
	if len(result2) != 1 {
		t.Fatalf("Wyckoff accumulation should allow LONG, expected 1, got %d", len(result2))
	}
}

func TestFilterHTFContext_BackwardCompat(t *testing.T) {
	// filterHTFContext should still work as full suppress
	signals := []models.Signal{
		makeSignalWithScore("SHORT", "1H", 10.0),
		makeSignalWithScore("LONG", "1H", 10.0),
	}
	indicators := htfIndicatorsLong("1D")
	bars := htfBarsAboveEMA("1D")

	result := filterHTFContext(signals, indicators, bars, "")
	if len(result) != 1 {
		t.Fatalf("legacy filterHTFContext should fully suppress, expected 1, got %d", len(result))
	}
	if result[0].Direction != "LONG" {
		t.Errorf("expected LONG, got %s", result[0].Direction)
	}
}
