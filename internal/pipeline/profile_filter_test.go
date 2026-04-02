package pipeline

import (
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestFilterByProfile(t *testing.T) {
	cfg := appconfig.SymbolProfilesConfig{
		DefaultProfile: "large_cap_stock",
		Profiles: map[string]appconfig.Profile{
			"crypto": {
				AllowedMethodologies: []string{"ict", "smc", "general_ta", "candlestick"},
				BlockedMethodologies: []string{"wyckoff"},
			},
			"large_cap_stock": {
				AllowedMethodologies: []string{"wyckoff", "general_ta", "candlestick"},
				AllowedRules:         []string{"ict_order_block", "smc_bos"},
			},
			"small_cap_stock": {
				AllowedMethodologies: []string{"candlestick", "general_ta"},
			},
		},
		SymbolOverrides: map[string]appconfig.SymbolOverride{
			"BTCUSDT": {Profile: "crypto"},
			"IBRX":    {Profile: "small_cap_stock"},
		},
	}
	holder := appconfig.NewSymbolProfilesHolder(cfg)

	signals := []models.Signal{
		{Symbol: "BTCUSDT", Rule: "ict_order_block"},
		{Symbol: "BTCUSDT", Rule: "wyckoff_spring"},
		{Symbol: "BTCUSDT", Rule: "smc_bos"},
		{Symbol: "BTCUSDT", Rule: "hammer"},
		{Symbol: "BTCUSDT", Rule: "rsi_divergence"},
	}

	tests := []struct {
		name      string
		symbol    string
		signals   []models.Signal
		wantCount int
		wantRules []string
	}{
		{
			name:      "crypto: wyckoff blocked",
			symbol:    "BTCUSDT",
			signals:   signals,
			wantCount: 4,
			wantRules: []string{"ict_order_block", "smc_bos", "hammer", "rsi_divergence"},
		},
		{
			name:   "large_cap: wyckoff allowed, specific ICT rules",
			symbol: "SPY",
			signals: []models.Signal{
				{Symbol: "SPY", Rule: "wyckoff_spring"},
				{Symbol: "SPY", Rule: "ict_order_block"},
				{Symbol: "SPY", Rule: "ict_fair_value_gap"},
				{Symbol: "SPY", Rule: "hammer"},
				{Symbol: "SPY", Rule: "smc_bos"},
			},
			wantCount: 4,
			wantRules: []string{"wyckoff_spring", "ict_order_block", "hammer", "smc_bos"},
		},
		{
			name:   "small_cap: only candlestick and general_ta",
			symbol: "IBRX",
			signals: []models.Signal{
				{Symbol: "IBRX", Rule: "hammer"},
				{Symbol: "IBRX", Rule: "rsi_divergence"},
				{Symbol: "IBRX", Rule: "ict_order_block"},
				{Symbol: "IBRX", Rule: "wyckoff_spring"},
			},
			wantCount: 2,
			wantRules: []string{"hammer", "rsi_divergence"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterByProfile(tt.signals, holder, tt.symbol)
			if len(got) != tt.wantCount {
				t.Errorf("filterByProfile() returned %d signals, want %d", len(got), tt.wantCount)
				for _, s := range got {
					t.Logf("  rule: %s", s.Rule)
				}
			}
			for i, wantRule := range tt.wantRules {
				if i >= len(got) {
					break
				}
				if got[i].Rule != wantRule {
					t.Errorf("signal[%d].Rule = %q, want %q", i, got[i].Rule, wantRule)
				}
			}
		})
	}
}

func TestFilterByProfile_NilHolder(t *testing.T) {
	signals := []models.Signal{
		{Rule: "ict_order_block"},
		{Rule: "wyckoff_spring"},
	}
	got := filterByProfile(signals, nil, "BTCUSDT")
	if len(got) != 2 {
		t.Errorf("nil holder: expected all signals to pass, got %d", len(got))
	}
}

func TestProfileScoreThreshold(t *testing.T) {
	cfg := appconfig.SymbolProfilesConfig{
		DefaultProfile: "crypto",
		Profiles: map[string]appconfig.Profile{
			"crypto": {ScoreThreshold: 12.0},
		},
	}
	holder := appconfig.NewSymbolProfilesHolder(cfg)

	if got := profileScoreThreshold(holder, "BTCUSDT"); got != 12.0 {
		t.Errorf("profileScoreThreshold = %f, want 12.0", got)
	}
	if got := profileScoreThreshold(nil, "BTCUSDT"); got != 0 {
		t.Errorf("nil holder: profileScoreThreshold = %f, want 0", got)
	}
}

func TestProfileCooldownHours(t *testing.T) {
	cfg := appconfig.SymbolProfilesConfig{
		DefaultProfile: "crypto",
		Profiles: map[string]appconfig.Profile{
			"crypto": {CooldownHours: 6},
		},
	}
	holder := appconfig.NewSymbolProfilesHolder(cfg)

	if got := profileCooldownHours(holder, "BTCUSDT"); got != 6 {
		t.Errorf("profileCooldownHours = %d, want 6", got)
	}
	if got := profileCooldownHours(nil, "BTCUSDT"); got != 0 {
		t.Errorf("nil holder: profileCooldownHours = %d, want 0", got)
	}
}
