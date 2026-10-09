// Package config — symbol profile configuration for per-symbol rule filtering.
package config

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Profile defines which methodologies and rules are allowed for a symbol class.
type Profile struct {
	AllowedMethodologies []string `yaml:"allowed_methodologies" json:"allowed_methodologies"`
	BlockedMethodologies []string `yaml:"blocked_methodologies" json:"blocked_methodologies"`
	AllowedRules         []string `yaml:"allowed_rules" json:"allowed_rules"`
	Timeframes           []string `yaml:"timeframes,omitempty" json:"timeframes,omitempty"`
	AlertLimitPerDay     int      `yaml:"alert_limit_per_day" json:"alert_limit_per_day"`
	CooldownHours        int      `yaml:"cooldown_hours" json:"cooldown_hours"`
	ScoreThreshold       float64  `yaml:"score_threshold" json:"score_threshold"`
}

// SymbolOverride maps a symbol to a named profile.
type SymbolOverride struct {
	Profile    string   `yaml:"profile" json:"profile"`
	SpreadPips *float64 `yaml:"spread_pips,omitempty" json:"spread_pips,omitempty"`
}

// SymbolProfilesConfig is the top-level structure for config/symbol_profiles.yaml.
type SymbolProfilesConfig struct {
	DefaultProfile  string                    `yaml:"default_profile" json:"default_profile"`
	Profiles        map[string]Profile        `yaml:"profiles" json:"profiles"`
	SymbolOverrides map[string]SymbolOverride `yaml:"symbol_overrides" json:"symbol_overrides"`
}

// SymbolProfilesHolder provides thread-safe access to symbol profile configuration.
type SymbolProfilesHolder struct {
	mu           sync.RWMutex
	cfg          SymbolProfilesConfig
	forexSymbols map[string]struct{}
}

// DefaultForexProfile is used for registered Forex symbols when no profile was selected.
func DefaultForexProfile() Profile {
	return Profile{
		AllowedMethodologies: []string{"ict", "smc", "general_ta", "candlestick"},
		BlockedMethodologies: []string{"wyckoff"},
		ScoreThreshold:       12,
		CooldownHours:        4,
		AlertLimitPerDay:     3,
	}
}

func withForexDefault(cfg SymbolProfilesConfig) SymbolProfilesConfig {
	if _, ok := cfg.Profiles["forex"]; ok {
		return cfg
	}
	profiles := make(map[string]Profile, len(cfg.Profiles)+1)
	for name, profile := range cfg.Profiles {
		profiles[name] = profile
	}
	profiles["forex"] = DefaultForexProfile()
	cfg.Profiles = profiles
	return cfg
}

// NewSymbolProfilesHolder creates a holder with the given initial config.
func NewSymbolProfilesHolder(cfg SymbolProfilesConfig) *SymbolProfilesHolder {
	return &SymbolProfilesHolder{cfg: withForexDefault(cfg)}
}

// Get returns the current configuration.
func (h *SymbolProfilesHolder) Get() SymbolProfilesConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// Set updates the configuration.
func (h *SymbolProfilesHolder) Set(cfg SymbolProfilesConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = withForexDefault(cfg)
}

// SetWatchlist refreshes the registered Forex symbols used for default resolution.
func (h *SymbolProfilesHolder) SetWatchlist(wl WatchlistConfig) {
	symbols := make(map[string]struct{}, len(wl.Symbols.Forex))
	for _, entry := range wl.Symbols.Forex {
		symbols[strings.ToUpper(entry.Symbol)] = struct{}{}
	}
	h.mu.Lock()
	h.forexSymbols = symbols
	h.mu.Unlock()
}

func (h *SymbolProfilesHolder) profileName(symbol string) string {
	if override, ok := h.cfg.SymbolOverrides[symbol]; ok && override.Profile != "" {
		return override.Profile
	}
	if _, ok := h.forexSymbols[strings.ToUpper(symbol)]; ok {
		return "forex"
	}
	return h.cfg.DefaultProfile
}

// GetProfile returns the profile for a given symbol. If the symbol has an override,
// that profile is used; otherwise the default_profile is used. If neither is found,
// an empty Profile (everything allowed) is returned.
func (h *SymbolProfilesHolder) GetProfile(symbol string) Profile {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg.Profiles[h.profileName(symbol)]
}

// GetProfileName returns the profile name assigned to a symbol.
func (h *SymbolProfilesHolder) GetProfileName(symbol string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.profileName(symbol)
}

// SetSymbolProfile sets the profile override for a symbol.
func (h *SymbolProfilesHolder) SetSymbolProfile(symbol, profileName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg.SymbolOverrides == nil {
		h.cfg.SymbolOverrides = make(map[string]SymbolOverride)
	}
	entry := h.cfg.SymbolOverrides[symbol]
	entry.Profile = profileName
	h.cfg.SymbolOverrides[symbol] = entry
}

func (h *SymbolProfilesHolder) SpreadOverride(symbol string) *float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if value := h.cfg.SymbolOverrides[symbol].SpreadPips; value != nil {
		if *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
			return nil
		}
		v := *value
		return &v
	}
	return nil
}

// ProfileNames returns all available profile names.
func (h *SymbolProfilesHolder) ProfileNames() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	names := make([]string, 0, len(h.cfg.Profiles))
	for name := range h.cfg.Profiles {
		names = append(names, name)
	}
	return names
}

// getProfile resolves the effective profile for a symbol from the config.
func getProfile(cfg SymbolProfilesConfig, symbol string) Profile {
	profileName := cfg.DefaultProfile
	if override, ok := cfg.SymbolOverrides[symbol]; ok && override.Profile != "" {
		profileName = override.Profile
	}
	if p, ok := cfg.Profiles[profileName]; ok {
		return p
	}
	return Profile{}
}

// LoadSymbolProfiles reads and parses symbol_profiles.yaml from the given path.
// Returns a zero-value config (everything allowed) if the file does not exist.
func LoadSymbolProfiles(path string) (SymbolProfilesConfig, error) {
	var cfg SymbolProfilesConfig
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, err
	}
	if err := validateSpreads(cfg); err != nil {
		return SymbolProfilesConfig{}, err
	}
	return cfg, nil
}

// SaveSymbolProfiles writes the config to the given YAML path.
func SaveSymbolProfiles(path string, cfg SymbolProfilesConfig) error {
	if err := validateSpreads(cfg); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func validateSpreads(cfg SymbolProfilesConfig) error {
	for symbol, override := range cfg.SymbolOverrides {
		if override.SpreadPips != nil && (*override.SpreadPips < 0 || math.IsNaN(*override.SpreadPips) || math.IsInf(*override.SpreadPips, 0)) {
			return fmt.Errorf("invalid spread_pips for %s", symbol)
		}
	}
	return nil
}

// IsSignalAllowed checks whether a signal with the given rule name is allowed
// by the profile. The rule name is used to infer its methodology.
func (p Profile) IsSignalAllowed(ruleName string) bool {
	methodology := RuleMethodology(ruleName)

	// Check blocked methodologies first.
	for _, blocked := range p.BlockedMethodologies {
		if blocked == methodology {
			return false
		}
	}

	// If allowed_methodologies is set, check the methodology is in the list.
	if len(p.AllowedMethodologies) > 0 {
		methodAllowed := false
		for _, m := range p.AllowedMethodologies {
			if m == methodology {
				methodAllowed = true
				break
			}
		}
		if methodAllowed {
			return true
		}
	}

	// If allowed_rules is set, check the specific rule.
	if len(p.AllowedRules) > 0 {
		for _, r := range p.AllowedRules {
			if r == ruleName {
				return true
			}
		}
	}

	// If neither allowed_methodologies nor allowed_rules is set, allow everything.
	if len(p.AllowedMethodologies) == 0 && len(p.AllowedRules) == 0 {
		return true
	}

	return false
}

// RuleMethodology extracts the methodology category from a rule name.
// Convention: "ict_*" -> "ict", "wyckoff_*" -> "wyckoff", "smc_*" -> "smc".
// Candlestick patterns (hammer, doji, engulfing, etc.) -> "candlestick".
// Everything else -> "general_ta".
func RuleMethodology(ruleName string) string {
	if strings.HasPrefix(ruleName, "ict_") {
		return "ict"
	}
	if strings.HasPrefix(ruleName, "wyckoff_") {
		return "wyckoff"
	}
	if strings.HasPrefix(ruleName, "smc_") {
		return "smc"
	}

	// Known candlestick pattern names
	candlestickRules := map[string]bool{
		"doji": true, "hammer": true, "hanging_man": true,
		"shooting_star": true, "inverted_hammer": true, "marubozu": true,
		"bullish_engulfing": true, "bearish_engulfing": true,
		"bullish_harami": true, "bearish_harami": true,
		"morning_star": true, "evening_star": true,
		"three_white_soldiers": true, "three_black_crows": true,
	}
	if candlestickRules[ruleName] {
		return "candlestick"
	}

	return "general_ta"
}
