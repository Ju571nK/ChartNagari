package sequence

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func sig(rule, dir string, ago time.Duration) models.Signal {
	return models.Signal{
		Symbol:    "BTCUSDT",
		Timeframe: "1H",
		Rule:      rule,
		Direction: dir,
		Score:     1.0,
		CreatedAt: time.Now().Add(-ago),
	}
}

func TestSweepDisplacement(t *testing.T) {
	tr := New()
	// Record a sweep
	matches := tr.Record(sig("ict_liquidity_sweep", "LONG", 10*time.Minute))
	if len(matches) != 0 {
		t.Errorf("single signal should not match, got %d", len(matches))
	}
	// Record a follow-up signal in same direction
	matches = tr.Record(sig("ema_cross", "LONG", 0))
	found := false
	for _, m := range matches {
		if m.Name == "sweep_displacement" {
			found = true
			if m.Bonus != 0.2 {
				t.Errorf("expected bonus 0.2, got %f", m.Bonus)
			}
			if len(m.Signals) != 2 {
				t.Errorf("expected 2 signals in match, got %d", len(m.Signals))
			}
		}
	}
	if !found {
		t.Error("expected sweep_displacement match")
	}
}

func TestSweepDisplacement_WrongDirection(t *testing.T) {
	tr := New()
	tr.Record(sig("ict_liquidity_sweep", "LONG", 10*time.Minute))
	matches := tr.Record(sig("ema_cross", "SHORT", 0)) // opposite direction
	for _, m := range matches {
		if m.Name == "sweep_displacement" {
			t.Error("should not match sweep_displacement with opposite direction")
		}
	}
}

func TestFVGRetest(t *testing.T) {
	tr := New()
	tr.Record(sig("ict_fair_value_gap", "SHORT", 30*time.Minute))
	matches := tr.Record(sig("rsi_overbought", "SHORT", 0))
	found := false
	for _, m := range matches {
		if m.Name == "fvg_retest" {
			found = true
			if m.Bonus != 0.15 {
				t.Errorf("expected bonus 0.15, got %f", m.Bonus)
			}
		}
	}
	if !found {
		t.Error("expected fvg_retest match")
	}
}

func TestOBRetest(t *testing.T) {
	tr := New()
	tr.Record(sig("ict_order_block", "LONG", 1*time.Hour))
	matches := tr.Record(sig("volume_spike", "LONG", 0))
	found := false
	for _, m := range matches {
		if m.Name == "ob_retest" {
			found = true
		}
	}
	if !found {
		t.Error("expected ob_retest match")
	}
}

func TestTrimOldSignals(t *testing.T) {
	tr := New()
	// Record an old signal (beyond maxAge)
	old := sig("ict_liquidity_sweep", "LONG", 49*time.Hour)
	tr.Record(old)
	// Record a new signal
	matches := tr.Record(sig("ema_cross", "LONG", 0))
	// Old signal should have been trimmed, so no sweep_displacement match
	for _, m := range matches {
		if m.Name == "sweep_displacement" {
			t.Error("old signal should have been trimmed")
		}
	}
}

func TestNoMatchWithoutPriorSignal(t *testing.T) {
	tr := New()
	matches := tr.Record(sig("ema_cross", "LONG", 0))
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for first signal, got %d", len(matches))
	}
}

func TestMultipleSequencesCanMatch(t *testing.T) {
	tr := New()
	// Record both sweep and FVG
	tr.Record(sig("ict_liquidity_sweep", "LONG", 20*time.Minute))
	tr.Record(sig("ict_fair_value_gap", "LONG", 10*time.Minute))
	// New signal should match both patterns
	matches := tr.Record(sig("support_resistance_breakout", "LONG", 0))
	names := make(map[string]bool)
	for _, m := range matches {
		names[m.Name] = true
	}
	if !names["sweep_displacement"] {
		t.Error("expected sweep_displacement match")
	}
	if !names["fvg_retest"] {
		t.Error("expected fvg_retest match")
	}
}
