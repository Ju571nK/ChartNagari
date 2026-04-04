package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSignalTuning(t *testing.T) {
	cfg := DefaultSignalTuning()
	if cfg.HTFFilter.CounterTrendPenaltyPct != 50 {
		t.Errorf("expected CounterTrendPenaltyPct=50, got %d", cfg.HTFFilter.CounterTrendPenaltyPct)
	}
	if cfg.VolatilityRegime.LowVolPercentile != 25 {
		t.Errorf("expected LowVolPercentile=25, got %d", cfg.VolatilityRegime.LowVolPercentile)
	}
	if cfg.VolatilityRegime.HighVolPercentile != 75 {
		t.Errorf("expected HighVolPercentile=75, got %d", cfg.VolatilityRegime.HighVolPercentile)
	}
	if cfg.ATRSlope.EMAPeriod != 20 {
		t.Errorf("expected EMAPeriod=20, got %d", cfg.ATRSlope.EMAPeriod)
	}
}

func TestLoadSignalTuning(t *testing.T) {
	content := `htf_filter:
  counter_trend_penalty_pct: 70
volatility_regime:
  low_vol_percentile: 30
  high_vol_percentile: 80
  low_vol_penalty_pct: 25
  high_vol_bonus_pct: 10
atr_slope:
  ema_period: 14
  rising_bonus_pct: 5
`
	tmp := filepath.Join(t.TempDir(), "signal_tuning.yaml")
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadSignalTuning(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HTFFilter.CounterTrendPenaltyPct != 70 {
		t.Errorf("expected 70, got %d", cfg.HTFFilter.CounterTrendPenaltyPct)
	}
	if cfg.VolatilityRegime.LowVolPercentile != 30 {
		t.Errorf("expected 30, got %d", cfg.VolatilityRegime.LowVolPercentile)
	}
	if cfg.VolatilityRegime.HighVolPercentile != 80 {
		t.Errorf("expected 80, got %d", cfg.VolatilityRegime.HighVolPercentile)
	}
	if cfg.VolatilityRegime.LowVolPenaltyPct != 25 {
		t.Errorf("expected 25, got %d", cfg.VolatilityRegime.LowVolPenaltyPct)
	}
	if cfg.VolatilityRegime.HighVolBonusPct != 10 {
		t.Errorf("expected 10, got %d", cfg.VolatilityRegime.HighVolBonusPct)
	}
	if cfg.ATRSlope.EMAPeriod != 14 {
		t.Errorf("expected 14, got %d", cfg.ATRSlope.EMAPeriod)
	}
	if cfg.ATRSlope.RisingBonusPct != 5 {
		t.Errorf("expected 5, got %d", cfg.ATRSlope.RisingBonusPct)
	}
}

func TestLoadSignalTuning_NotFound(t *testing.T) {
	_, err := LoadSignalTuning("/nonexistent/signal_tuning.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestSaveSignalTuning(t *testing.T) {
	cfg := SignalTuningConfig{
		HTFFilter: HTFFilterConfig{CounterTrendPenaltyPct: 60},
		VolatilityRegime: VolatilityRegimeConfig{
			LowVolPercentile:  20,
			HighVolPercentile: 80,
			LowVolPenaltyPct:  15,
			HighVolBonusPct:   20,
		},
		ATRSlope: ATRSlopeConfig{EMAPeriod: 10, RisingBonusPct: 8},
	}

	tmp := filepath.Join(t.TempDir(), "signal_tuning.yaml")
	if err := SaveSignalTuning(tmp, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSignalTuning(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.HTFFilter.CounterTrendPenaltyPct != 60 {
		t.Errorf("expected 60, got %d", loaded.HTFFilter.CounterTrendPenaltyPct)
	}
	if loaded.ATRSlope.EMAPeriod != 10 {
		t.Errorf("expected 10, got %d", loaded.ATRSlope.EMAPeriod)
	}
}

func TestSignalTuningHolder(t *testing.T) {
	cfg := DefaultSignalTuning()
	h := NewSignalTuningHolder(cfg)

	got := h.Get()
	if got.HTFFilter.CounterTrendPenaltyPct != 50 {
		t.Errorf("expected 50, got %d", got.HTFFilter.CounterTrendPenaltyPct)
	}

	updated := cfg
	updated.HTFFilter.CounterTrendPenaltyPct = 80
	h.Set(updated)

	got = h.Get()
	if got.HTFFilter.CounterTrendPenaltyPct != 80 {
		t.Errorf("expected 80 after Set, got %d", got.HTFFilter.CounterTrendPenaltyPct)
	}
}
