// Package config — effective_config.go
//
// EffectiveAlertConfig merges a YAML profile (system default) with a
// SQLite per-symbol override (user customization) into the final
// AlertConfig that the pipeline filter chain consults on each tick.
package config

import "github.com/Ju571nK/Chatter/internal/storage"

// EffectiveConfig is the resolved alert configuration for a single symbol.
// Empty slices mean "no constraint" (allow all).
type EffectiveConfig struct {
	ScoreThreshold       float64
	CooldownHours        int
	AlertLimitPerDay     int
	Timeframes           []string
	AllowedRules         []string
	AllowedMethodologies []string
	BlockedMethodologies []string
}

// OverrideGetter is the minimal interface EffectiveAlertConfig needs.
// *storage.SymbolOverrideStore satisfies it; tests inject a fake.
type OverrideGetter interface {
	Get(symbol string) (*storage.SymbolOverride, error)
}

// EffectiveAlertConfig resolves the final alert config for a symbol.
// Resolution: start from the profile, then for each non-nil override field,
// replace the profile value with the override value.
//
// Defensive behavior:
//   - holder == nil → returns zero EffectiveConfig (everything passes filters).
//   - store == nil  → profile-only resolution.
//   - store.Get error → logged-and-ignored at the call site, profile-only result.
func EffectiveAlertConfig(symbol string, holder *SymbolProfilesHolder, store OverrideGetter) EffectiveConfig {
	if holder == nil {
		return EffectiveConfig{}
	}
	p := holder.GetProfile(symbol)

	cfg := EffectiveConfig{
		ScoreThreshold:       p.ScoreThreshold,
		CooldownHours:        p.CooldownHours,
		AlertLimitPerDay:     p.AlertLimitPerDay,
		Timeframes:           p.Timeframes,
		AllowedRules:         p.AllowedRules,
		AllowedMethodologies: p.AllowedMethodologies,
		BlockedMethodologies: p.BlockedMethodologies,
	}

	if store == nil {
		return cfg
	}
	ov, err := store.Get(symbol)
	if err != nil || ov == nil {
		return cfg
	}

	if ov.ScoreThreshold != nil {
		cfg.ScoreThreshold = *ov.ScoreThreshold
	}
	if ov.CooldownHours != nil {
		cfg.CooldownHours = *ov.CooldownHours
	}
	if ov.AlertLimitPerDay != nil {
		cfg.AlertLimitPerDay = *ov.AlertLimitPerDay
	}
	if ov.Timeframes != nil {
		cfg.Timeframes = ov.Timeframes
	}
	if ov.AllowedRules != nil {
		cfg.AllowedRules = ov.AllowedRules
	}
	return cfg
}
