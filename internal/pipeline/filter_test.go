package pipeline

import (
	"testing"

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
