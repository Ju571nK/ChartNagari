// Package config — signal tuning configuration for runtime-adjustable signal scoring parameters.
package config

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// HTFFilterConfig controls counter-trend penalty behavior.
type HTFFilterConfig struct {
	CounterTrendPenaltyPct int `yaml:"counter_trend_penalty_pct" json:"counter_trend_penalty_pct"`
}

// VolatilityRegimeConfig controls ATR percentile thresholds and score adjustments.
type VolatilityRegimeConfig struct {
	LowVolPercentile  int `yaml:"low_vol_percentile" json:"low_vol_percentile"`
	HighVolPercentile int `yaml:"high_vol_percentile" json:"high_vol_percentile"`
	LowVolPenaltyPct  int `yaml:"low_vol_penalty_pct" json:"low_vol_penalty_pct"`
	HighVolBonusPct   int `yaml:"high_vol_bonus_pct" json:"high_vol_bonus_pct"`
}

// ATRSlopeConfig controls ATR slope detection and bonus scoring.
type ATRSlopeConfig struct {
	EMAPeriod       int `yaml:"ema_period" json:"ema_period"`
	RisingBonusPct  int `yaml:"rising_bonus_pct" json:"rising_bonus_pct"`
}

// SignalTuningConfig is the top-level structure for config/signal_tuning.yaml.
type SignalTuningConfig struct {
	HTFFilter        HTFFilterConfig        `yaml:"htf_filter" json:"htf_filter"`
	VolatilityRegime VolatilityRegimeConfig `yaml:"volatility_regime" json:"volatility_regime"`
	ATRSlope         ATRSlopeConfig         `yaml:"atr_slope" json:"atr_slope"`
}

// DefaultSignalTuning returns sensible defaults matching the initial YAML.
func DefaultSignalTuning() SignalTuningConfig {
	return SignalTuningConfig{
		HTFFilter: HTFFilterConfig{
			CounterTrendPenaltyPct: 50,
		},
		VolatilityRegime: VolatilityRegimeConfig{
			LowVolPercentile:  25,
			HighVolPercentile: 75,
			LowVolPenaltyPct:  20,
			HighVolBonusPct:   15,
		},
		ATRSlope: ATRSlopeConfig{
			EMAPeriod:      20,
			RisingBonusPct: 10,
		},
	}
}

// LoadSignalTuning reads a SignalTuningConfig from the given YAML file path.
func LoadSignalTuning(path string) (SignalTuningConfig, error) {
	var cfg SignalTuningConfig
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SaveSignalTuning writes a SignalTuningConfig to the given YAML file path.
func SaveSignalTuning(path string, cfg SignalTuningConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SignalTuningHolder provides thread-safe access to signal tuning configuration.
// It follows the same pattern as AlertConfigHolder.
type SignalTuningHolder struct {
	mu  sync.RWMutex
	cfg SignalTuningConfig
}

// NewSignalTuningHolder creates a holder with the given initial config.
func NewSignalTuningHolder(cfg SignalTuningConfig) *SignalTuningHolder {
	return &SignalTuningHolder{cfg: cfg}
}

// Get returns the current configuration.
func (h *SignalTuningHolder) Get() SignalTuningConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// Set updates the configuration.
func (h *SignalTuningHolder) Set(cfg SignalTuningConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = cfg
}
