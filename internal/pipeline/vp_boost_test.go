package pipeline

import (
	"testing"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestVPBoost_SweepNearHVN(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_liquidity_sweep", Timeframe: "1D", EntryPrice: 100.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14":   5.0,
		"1D:VP_POC":   105.0,
		"1D:VP_HVN_1": 100.5, // within 0.5*ATR=2.5 of entry 100.0
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score < 1.14 || signals[0].Score > 1.16 {
		t.Errorf("expected ~1.15 after HVN boost, got %f", signals[0].Score)
	}
}

func TestVPBoost_SweepNoHVN(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_liquidity_sweep", Timeframe: "1D", EntryPrice: 100.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14":   5.0,
		"1D:VP_POC":   200.0, // far away
		"1D:VP_HVN_1": 200.0,
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score != 1.0 {
		t.Errorf("expected no change (1.0), got %f", signals[0].Score)
	}
}

func TestVPBoost_FVGNearLVN(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_fair_value_gap", Timeframe: "1D", ZoneLow: 98.0, ZoneHigh: 102.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14":   5.0,
		"1D:VP_LVN_1": 100.5, // gapMid=100, within 0.3*ATR=1.5
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score < 1.14 || signals[0].Score > 1.16 {
		t.Errorf("expected ~1.15 after LVN boost, got %f", signals[0].Score)
	}
}

func TestVPBoost_FVGNearPOC_Penalty(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_fair_value_gap", Timeframe: "1D", ZoneLow: 99.0, ZoneHigh: 101.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14": 5.0,
		"1D:VP_POC": 100.2, // gapMid=100, within 0.3*ATR=1.5
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score < 0.84 || signals[0].Score > 0.86 {
		t.Errorf("expected ~0.85 after POC penalty, got %f", signals[0].Score)
	}
}

func TestVPBoost_OBOverlapsHVN(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_order_block", Timeframe: "1D", ZoneLow: 95.0, ZoneHigh: 105.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14":   5.0,
		"1D:VP_HVN_1": 100.0, // inside OB zone
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score < 1.19 || signals[0].Score > 1.21 {
		t.Errorf("expected ~1.20 after HVN boost, got %f", signals[0].Score)
	}
}

func TestVPBoost_OBContainsPOC(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_order_block", Timeframe: "1D", ZoneLow: 95.0, ZoneHigh: 105.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14": 5.0,
		"1D:VP_POC": 100.0, // inside OB zone, no HVN overlap
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score < 1.09 || signals[0].Score > 1.11 {
		t.Errorf("expected ~1.10 after POC boost, got %f", signals[0].Score)
	}
}

func TestVPBoost_OBOverlapsLVN_Penalty(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_order_block", Timeframe: "1D", ZoneLow: 95.0, ZoneHigh: 105.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14":   5.0,
		"1D:VP_LVN_1": 100.0, // inside OB zone, no HVN/POC
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score < 0.89 || signals[0].Score > 0.91 {
		t.Errorf("expected ~0.90 after LVN penalty, got %f", signals[0].Score)
	}
}

func TestVPBoost_NoATR_Skip(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_liquidity_sweep", Timeframe: "1D", EntryPrice: 100.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:VP_HVN_1": 100.0, // no ATR
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score != 1.0 {
		t.Errorf("expected no change without ATR, got %f", signals[0].Score)
	}
}

func TestVPBoost_UnrelatedRule_NoChange(t *testing.T) {
	signals := []models.Signal{
		{Rule: "rsi_overbought_oversold", Timeframe: "1D", EntryPrice: 100.0, Score: 1.0},
	}
	indicators := map[string]float64{
		"1D:ATR_14":   5.0,
		"1D:VP_HVN_1": 100.0,
	}
	applyVolumeProfileBoost(signals, indicators)
	if signals[0].Score != 1.0 {
		t.Errorf("expected no change for non-ICT rule, got %f", signals[0].Score)
	}
}
