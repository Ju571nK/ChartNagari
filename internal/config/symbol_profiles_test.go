package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuleMethodology(t *testing.T) {
	tests := []struct {
		rule string
		want string
	}{
		{"ict_order_block", "ict"},
		{"ict_fair_value_gap", "ict"},
		{"ict_liquidity_sweep", "ict"},
		{"ict_breaker_block", "ict"},
		{"ict_kill_zone", "ict"},
		{"wyckoff_accumulation", "wyckoff"},
		{"wyckoff_distribution", "wyckoff"},
		{"wyckoff_spring", "wyckoff"},
		{"wyckoff_upthrust", "wyckoff"},
		{"wyckoff_volume_anomaly", "wyckoff"},
		{"smc_bos", "smc"},
		{"smc_choch", "smc"},
		{"doji", "candlestick"},
		{"hammer", "candlestick"},
		{"bullish_engulfing", "candlestick"},
		{"morning_star", "candlestick"},
		{"three_white_soldiers", "candlestick"},
		{"rsi_overbought_oversold", "general_ta"},
		{"rsi_divergence", "general_ta"},
		{"ema_cross", "general_ta"},
		{"volume_spike", "general_ta"},
		{"support_resistance_breakout", "general_ta"},
		{"fibonacci_confluence", "general_ta"},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			got := RuleMethodology(tt.rule)
			if got != tt.want {
				t.Errorf("RuleMethodology(%q) = %q, want %q", tt.rule, got, tt.want)
			}
		})
	}
}

func TestProfileIsSignalAllowed(t *testing.T) {
	tests := []struct {
		name    string
		profile Profile
		rule    string
		want    bool
	}{
		{
			name: "crypto profile allows ICT",
			profile: Profile{
				AllowedMethodologies: []string{"ict", "smc", "general_ta", "candlestick"},
				BlockedMethodologies: []string{"wyckoff"},
			},
			rule: "ict_order_block",
			want: true,
		},
		{
			name: "crypto profile blocks wyckoff",
			profile: Profile{
				AllowedMethodologies: []string{"ict", "smc", "general_ta", "candlestick"},
				BlockedMethodologies: []string{"wyckoff"},
			},
			rule: "wyckoff_spring",
			want: false,
		},
		{
			name: "large cap allows wyckoff methodology",
			profile: Profile{
				AllowedMethodologies: []string{"wyckoff", "general_ta", "candlestick"},
				AllowedRules:         []string{"ict_order_block", "ict_liquidity_sweep", "ict_kill_zone", "smc_bos"},
			},
			rule: "wyckoff_accumulation",
			want: true,
		},
		{
			name: "large cap allows specific ICT rule",
			profile: Profile{
				AllowedMethodologies: []string{"wyckoff", "general_ta", "candlestick"},
				AllowedRules:         []string{"ict_order_block", "ict_liquidity_sweep", "ict_kill_zone", "smc_bos"},
			},
			rule: "ict_order_block",
			want: true,
		},
		{
			name: "large cap blocks unlisted ICT rule",
			profile: Profile{
				AllowedMethodologies: []string{"wyckoff", "general_ta", "candlestick"},
				AllowedRules:         []string{"ict_order_block", "ict_liquidity_sweep", "ict_kill_zone", "smc_bos"},
			},
			rule: "ict_fair_value_gap",
			want: false,
		},
		{
			name: "small cap allows listed rule",
			profile: Profile{
				AllowedMethodologies: []string{"candlestick", "general_ta"},
				AllowedRules: []string{
					"volume_spike", "rsi_overbought_oversold", "support_resistance_breakout",
					"bullish_engulfing", "bearish_engulfing", "hammer", "shooting_star",
					"morning_star", "evening_star",
				},
			},
			rule: "hammer",
			want: true,
		},
		{
			name: "small cap blocks unlisted wyckoff",
			profile: Profile{
				AllowedMethodologies: []string{"candlestick", "general_ta"},
				AllowedRules: []string{
					"volume_spike", "rsi_overbought_oversold",
				},
			},
			rule: "wyckoff_spring",
			want: false,
		},
		{
			name: "empty profile allows everything",
			profile: Profile{},
			rule:    "ict_order_block",
			want:    true,
		},
		{
			name:    "blocked methodology takes precedence over allowed rules",
			profile: Profile{
				AllowedRules:         []string{"wyckoff_spring"},
				BlockedMethodologies: []string{"wyckoff"},
			},
			rule: "wyckoff_spring",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.profile.IsSignalAllowed(tt.rule)
			if got != tt.want {
				t.Errorf("IsSignalAllowed(%q) = %v, want %v", tt.rule, got, tt.want)
			}
		})
	}
}

func TestGetProfile(t *testing.T) {
	cfg := SymbolProfilesConfig{
		DefaultProfile: "large_cap_stock",
		Profiles: map[string]Profile{
			"crypto": {
				AllowedMethodologies: []string{"ict", "smc"},
				CooldownHours:        6,
				ScoreThreshold:       12.0,
			},
			"large_cap_stock": {
				AllowedMethodologies: []string{"wyckoff", "general_ta"},
				CooldownHours:        8,
				ScoreThreshold:       10.0,
			},
		},
		SymbolOverrides: map[string]SymbolOverride{
			"BTCUSDT": {Profile: "crypto"},
		},
	}

	tests := []struct {
		symbol         string
		wantCooldown   int
		wantThreshold  float64
	}{
		{"BTCUSDT", 6, 12.0},       // has override
		{"SPY", 8, 10.0},           // uses default
		{"UNKNOWN", 8, 10.0},       // uses default
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			p := getProfile(cfg, tt.symbol)
			if p.CooldownHours != tt.wantCooldown {
				t.Errorf("CooldownHours = %d, want %d", p.CooldownHours, tt.wantCooldown)
			}
			if p.ScoreThreshold != tt.wantThreshold {
				t.Errorf("ScoreThreshold = %f, want %f", p.ScoreThreshold, tt.wantThreshold)
			}
		})
	}
}

func TestLoadSymbolProfiles(t *testing.T) {
	content := `
default_profile: crypto

profiles:
  crypto:
    allowed_methodologies: [ict, smc]
    cooldown_hours: 6
    score_threshold: 12.0

symbol_overrides:
  BTCUSDT: { profile: crypto }
`
	dir := t.TempDir()
	path := filepath.Join(dir, "symbol_profiles.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadSymbolProfiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProfile != "crypto" {
		t.Errorf("DefaultProfile = %q, want %q", cfg.DefaultProfile, "crypto")
	}
	if len(cfg.Profiles) != 1 {
		t.Errorf("len(Profiles) = %d, want 1", len(cfg.Profiles))
	}
	if len(cfg.SymbolOverrides) != 1 {
		t.Errorf("len(SymbolOverrides) = %d, want 1", len(cfg.SymbolOverrides))
	}
}

func TestLoadSymbolProfiles_Missing(t *testing.T) {
	cfg, err := LoadSymbolProfiles("/nonexistent/path/symbol_profiles.yaml")
	if err != nil {
		t.Fatal("expected nil error for missing file, got:", err)
	}
	if cfg.DefaultProfile != "" {
		t.Errorf("expected empty default profile, got %q", cfg.DefaultProfile)
	}
}

func TestSymbolProfilesHolder(t *testing.T) {
	cfg := SymbolProfilesConfig{
		DefaultProfile: "large_cap_stock",
		Profiles: map[string]Profile{
			"crypto":         {CooldownHours: 6},
			"large_cap_stock": {CooldownHours: 8},
		},
		SymbolOverrides: map[string]SymbolOverride{},
	}
	h := NewSymbolProfilesHolder(cfg)

	// Default profile
	if name := h.GetProfileName("SPY"); name != "large_cap_stock" {
		t.Errorf("GetProfileName(SPY) = %q, want large_cap_stock", name)
	}

	// Set override
	h.SetSymbolProfile("SPY", "crypto")
	if name := h.GetProfileName("SPY"); name != "crypto" {
		t.Errorf("after override, GetProfileName(SPY) = %q, want crypto", name)
	}

	p := h.GetProfile("SPY")
	if p.CooldownHours != 6 {
		t.Errorf("after override, CooldownHours = %d, want 6", p.CooldownHours)
	}

	// ProfileNames
	names := h.ProfileNames()
	if len(names) != 2 {
		t.Errorf("ProfileNames() len = %d, want 2", len(names))
	}
}

func TestSaveAndLoadSymbolProfiles(t *testing.T) {
	cfg := SymbolProfilesConfig{
		DefaultProfile: "crypto",
		Profiles: map[string]Profile{
			"crypto": {
				AllowedMethodologies: []string{"ict"},
				CooldownHours:        6,
				ScoreThreshold:       12.0,
			},
		},
		SymbolOverrides: map[string]SymbolOverride{
			"BTCUSDT": {Profile: "crypto"},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "symbol_profiles.yaml")

	if err := SaveSymbolProfiles(path, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSymbolProfiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultProfile != "crypto" {
		t.Errorf("DefaultProfile = %q, want crypto", loaded.DefaultProfile)
	}
	if len(loaded.Profiles["crypto"].AllowedMethodologies) != 1 {
		t.Errorf("AllowedMethodologies len = %d, want 1", len(loaded.Profiles["crypto"].AllowedMethodologies))
	}
}
