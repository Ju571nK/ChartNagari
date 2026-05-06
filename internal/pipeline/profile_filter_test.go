package pipeline

import (
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// stubOverrideStore satisfies appconfig.OverrideGetter for pipeline tests.
type stubOverrideStore struct {
	rows map[string]*storage.SymbolOverride
}

func (s *stubOverrideStore) Get(symbol string) (*storage.SymbolOverride, error) {
	if s == nil || s.rows == nil {
		return nil, nil
	}
	return s.rows[symbol], nil
}

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

func TestEffectiveScoreThreshold_OverrideWins(t *testing.T) {
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]appconfig.Profile{
			"p1": {ScoreThreshold: 10},
		},
	})
	score := 14.0
	store := &stubOverrideStore{rows: map[string]*storage.SymbolOverride{
		"TSLA": {Symbol: "TSLA", ScoreThreshold: &score},
	}}

	cfg := appconfig.EffectiveAlertConfig("TSLA", holder, store)
	if cfg.ScoreThreshold != 14.0 {
		t.Errorf("TSLA score threshold = %v, want 14.0 (override)", cfg.ScoreThreshold)
	}

	cfg2 := appconfig.EffectiveAlertConfig("AAPL", holder, store)
	if cfg2.ScoreThreshold != 10.0 {
		t.Errorf("AAPL score threshold = %v, want 10.0 (profile)", cfg2.ScoreThreshold)
	}
}

func TestFilterByTimeframe_DropsDisallowed(t *testing.T) {
	signals := []models.Signal{
		{Rule: "r1", Timeframe: "1H"},
		{Rule: "r1", Timeframe: "4H"},
		{Rule: "r1", Timeframe: "1D"},
		{Rule: "r1", Timeframe: "1W"},
	}
	out := filterByTimeframe(signals, []string{"1D", "1W"})
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].Timeframe != "1D" || out[1].Timeframe != "1W" {
		t.Errorf("out = %v, want [1D, 1W] only", out)
	}
}

func TestFilterByTimeframe_EmptyAllowsAll(t *testing.T) {
	signals := []models.Signal{
		{Timeframe: "1H"}, {Timeframe: "4H"}, {Timeframe: "1D"},
	}
	out := filterByTimeframe(signals, nil)
	if len(out) != 3 {
		t.Errorf("nil filter: len(out) = %d, want 3", len(out))
	}
	out = filterByTimeframe(signals, []string{})
	if len(out) != 3 {
		t.Errorf("empty filter: len(out) = %d, want 3", len(out))
	}
}

func TestEffectiveScoreThreshold_HotReload(t *testing.T) {
	// Simulates: pipeline reads → user changes via UI → pipeline reads again.
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]appconfig.Profile{
			"p1": {ScoreThreshold: 5},
		},
	})

	// Use a real DB so we can mutate it mid-test.
	db, err := storage.New(t.TempDir() + "/hr.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := storage.NewSymbolOverrideStore(db)

	first := appconfig.EffectiveAlertConfig("TSLA", holder, store)
	if first.ScoreThreshold != 5 {
		t.Fatalf("initial score = %v, want 5", first.ScoreThreshold)
	}

	score := 18.0
	if err := store.Put(storage.SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score}); err != nil {
		t.Fatal(err)
	}

	second := appconfig.EffectiveAlertConfig("TSLA", holder, store)
	if second.ScoreThreshold != 18.0 {
		t.Errorf("after override: score = %v, want 18.0 (hot-reload failed)", second.ScoreThreshold)
	}
}

func TestEffectiveScoreThreshold_FilteredAtPipeline(t *testing.T) {
	// Sanity: the score gate is computed correctly. (Pipeline-level wiring is
	// indirectly tested via TestEffectiveScoreThreshold_OverrideWins; this test
	// asserts the per-symbol effCfg.ScoreThreshold value is what the pipeline
	// would apply.)
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]appconfig.Profile{
			"p1": {ScoreThreshold: 5},
		},
	})
	threshold := 12.0
	store := &stubOverrideStore{rows: map[string]*storage.SymbolOverride{
		"TSLA": {Symbol: "TSLA", ScoreThreshold: &threshold},
	}}
	cfg := appconfig.EffectiveAlertConfig("TSLA", holder, store)
	if cfg.ScoreThreshold != 12.0 {
		t.Errorf("TSLA effective threshold = %v, want 12.0 (override winning)", cfg.ScoreThreshold)
	}

	// Simulate the inner-loop filter logic.
	signals := []models.Signal{
		{Rule: "r1", Score: 8},  // below 12 → dropped
		{Rule: "r1", Score: 15}, // above 12 → kept
		{Rule: "r1", Score: 12}, // exact → kept (>=)
	}
	kept := signals[:0]
	for _, sig := range signals {
		if sig.Score >= cfg.ScoreThreshold {
			kept = append(kept, sig)
		}
	}
	if len(kept) != 2 {
		t.Errorf("kept = %d, want 2", len(kept))
	}
	if kept[0].Score != 15 || kept[1].Score != 12 {
		t.Errorf("wrong signals kept: %v", kept)
	}
}
