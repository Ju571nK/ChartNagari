package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── mock rules ────────────────────────────────────────────────────────────────

// alwaysLongRule always returns a LONG signal.
type alwaysLongRule struct{ score float64 }

func (r *alwaysLongRule) Name() string                 { return "always_long" }
func (r *alwaysLongRule) RequiredIndicators() []string { return nil }
func (r *alwaysLongRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: "1H",
		Rule:      r.Name(),
		Direction: "LONG",
		Score:     r.score,
		CreatedAt: time.Now(),
	}, nil
}

// alwaysShortRule always returns a SHORT signal.
type alwaysShortRule struct{ score float64 }

func (r *alwaysShortRule) Name() string                 { return "always_short" }
func (r *alwaysShortRule) RequiredIndicators() []string { return nil }
func (r *alwaysShortRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: "1H",
		Rule:      r.Name(),
		Direction: "SHORT",
		Score:     r.score,
		CreatedAt: time.Now(),
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// engCfgFor builds a RuleConfig with the listed rule names enabled.
func engCfgFor(names ...string) engine.RuleConfig {
	rules := make(map[string]engine.RuleEntry, len(names))
	for _, n := range names {
		rules[n] = engine.RuleEntry{Enabled: true, Timeframe: "1H", Weight: 1.0}
	}
	return engine.RuleConfig{Rules: rules}
}

// makeBars creates n synthetic OHLCV bars with basePrice and atrSpread.
func makeBars(n int, basePrice, atrSpread float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	t := time.Unix(0, 0).UTC()
	for i := range bars {
		bars[i] = models.OHLCV{
			Symbol:    "TEST",
			Timeframe: "1H",
			OpenTime:  t,
			Open:      basePrice,
			High:      basePrice + atrSpread,
			Low:       basePrice - atrSpread,
			Close:     basePrice,
			Volume:    1000,
		}
		t = t.Add(time.Hour)
	}
	return bars
}

// ── tests ─────────────────────────────────────────────────────────────────────

// Test 1: empty bars → zero result.
func TestRun_EmptyBars(t *testing.T) {
	eng := New(nil, engine.RuleConfig{}, DefaultConfig())
	result := eng.Run("TEST", "1H", "", nil)
	if result.Trades != 0 || len(result.Outcomes) != 0 {
		t.Fatalf("expected 0 trades, got %d", result.Trades)
	}
}

// Test 2: fewer bars than warmup → zero trades.
func TestRun_InsufficientBars(t *testing.T) {
	cfg := Config{WarmupBars: 200, MaxExitBars: 20, TPATRMultiplier: 2.0, SLATRMultiplier: 1.0}
	eng := New(
		[]rule.AnalysisRule{&alwaysLongRule{score: 1.0}},
		engCfgFor("always_long"),
		cfg,
	)
	bars := makeBars(50, 100.0, 1.0)
	result := eng.Run("TEST", "1H", "", bars)
	if result.Trades != 0 {
		t.Fatalf("expected 0 trades with insufficient bars, got %d", result.Trades)
	}
}

// Test 3: LONG signal, TP hit → win recorded.
// ATR with spread=1 is ~2; TP = entry + 2*ATR ≈ 104. Spike to 105 guarantees TP hit.
func TestRun_LongTPHit(t *testing.T) {
	cfg := Config{WarmupBars: 14, MaxExitBars: 10, TPATRMultiplier: 2.0, SLATRMultiplier: 1.0}
	eng := New(
		[]rule.AnalysisRule{&alwaysLongRule{score: 1.0}},
		engCfgFor("always_long"),
		cfg,
	)

	bars := makeBars(25, 100.0, 1.0)
	// Signal fires at i=14; spike well above TP at i=16.
	bars[16].High = 110.0

	result := eng.Run("TEST", "1H", "", bars)
	if result.Trades == 0 {
		t.Fatal("expected at least one trade")
	}
	for _, o := range result.Outcomes {
		if o.Direction == "LONG" && o.Win {
			return // pass
		}
	}
	t.Errorf("expected at least one LONG win; outcomes: %+v", result.Outcomes)
}

// Test 4: SHORT signal, TP hit → win recorded.
// ATR with spread=1 is ~2; TP = entry - 2*ATR ≈ 96. Drop to 85 guarantees TP hit.
func TestRun_ShortTPHit(t *testing.T) {
	cfg := Config{WarmupBars: 14, MaxExitBars: 10, TPATRMultiplier: 2.0, SLATRMultiplier: 1.0}
	eng := New(
		[]rule.AnalysisRule{&alwaysShortRule{score: 1.0}},
		engCfgFor("always_short"),
		cfg,
	)

	bars := makeBars(25, 100.0, 1.0)
	// Drop well below TP at i=16.
	bars[16].Low = 85.0
	bars[16].High = 100.5

	result := eng.Run("TEST", "1H", "", bars)
	if result.Trades == 0 {
		t.Fatal("expected at least one trade")
	}
	for _, o := range result.Outcomes {
		if o.Direction == "SHORT" && o.Win {
			return // pass
		}
	}
	t.Errorf("expected at least one SHORT win; outcomes: %+v", result.Outcomes)
}

// Test 5: LONG signal, SL hit → loss recorded.
func TestRun_LongSLHit(t *testing.T) {
	cfg := Config{WarmupBars: 14, MaxExitBars: 10, TPATRMultiplier: 2.0, SLATRMultiplier: 1.0}
	eng := New(
		[]rule.AnalysisRule{&alwaysLongRule{score: 1.0}},
		engCfgFor("always_long"),
		cfg,
	)

	basePrice := 100.0
	atrSpread := 1.0 // TP=102, SL=99

	bars := makeBars(25, basePrice, atrSpread)
	// Drop below SL at i=16 without hitting TP.
	bars[16].Low = 97.0
	bars[16].High = 100.5

	result := eng.Run("TEST", "1H", "", bars)
	if result.Trades == 0 {
		t.Fatal("expected at least one trade")
	}
	for _, o := range result.Outcomes {
		if o.Direction == "LONG" && !o.Win {
			return // pass
		}
	}
	t.Error("expected at least one LONG loss when SL is hit")
}

// Test 6: timeout exit — neither TP nor SL is ever hit within MaxExitBars.
// Extreme multipliers ensure levels are unreachable; all exits must be timeouts.
func TestRun_TimeoutExit(t *testing.T) {
	cfg := Config{WarmupBars: 14, MaxExitBars: 3, TPATRMultiplier: 500.0, SLATRMultiplier: 500.0}
	eng := New(
		[]rule.AnalysisRule{&alwaysLongRule{score: 1.0}},
		engCfgFor("always_long"),
		cfg,
	)

	// Use enough bars so at least one signal can fully use MaxExitBars.
	// With warmup=14, first signal at i=14, limit=i+1+3=18; bars need len>=18.
	bars := makeBars(30, 100.0, 0.5)
	result := eng.Run("TEST", "1H", "", bars)
	if result.Trades == 0 {
		t.Fatal("expected timeout trades")
	}
	// No trade should exit beyond MaxExitBars.
	for _, o := range result.Outcomes {
		if o.ExitBars > cfg.MaxExitBars {
			t.Errorf("exit bars %d exceeds MaxExitBars=%d", o.ExitBars, cfg.MaxExitBars)
		}
	}
	// Verify at least one trade exits exactly at MaxExitBars (first signal can do so).
	found := false
	for _, o := range result.Outcomes {
		if o.ExitBars == cfg.MaxExitBars {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one trade exiting at MaxExitBars=3")
	}
}

// Test 7: ruleFilter excludes unrelated rule trades.
func TestRun_RuleFilter(t *testing.T) {
	cfg := Config{WarmupBars: 14, MaxExitBars: 5, TPATRMultiplier: 2.0, SLATRMultiplier: 1.0}
	eng := New(
		[]rule.AnalysisRule{
			&alwaysLongRule{score: 1.0},
			&alwaysShortRule{score: 1.0},
		},
		engCfgFor("always_long", "always_short"),
		cfg,
	)

	bars := makeBars(25, 100.0, 1.0)
	result := eng.Run("TEST", "1H", "always_long", bars)
	for _, o := range result.Outcomes {
		if o.Rule != "always_long" {
			t.Errorf("filter 'always_long' should exclude rule %q", o.Rule)
		}
	}
}

// Test 8: ComputeStats with known outcomes.
func TestComputeStats_KnownOutcomes(t *testing.T) {
	outcomes := []TradeOutcome{
		{Win: true, PnLPct: 4.0},
		{Win: true, PnLPct: 2.0},
		{Win: false, PnLPct: -2.0},
		{Win: false, PnLPct: -1.0},
	}
	s := ComputeStats(outcomes)

	if math.Abs(s.WinRate-0.5) > 1e-9 {
		t.Errorf("WinRate: want 0.5, got %f", s.WinRate)
	}
	if math.Abs(s.TotalReturnPct-3.0) > 1e-9 {
		t.Errorf("TotalReturnPct: want 3.0, got %f", s.TotalReturnPct)
	}
	if s.MaxConsecLosses != 2 {
		t.Errorf("MaxConsecLosses: want 2, got %d", s.MaxConsecLosses)
	}
	// ProfitFactor = (4+2) / (2+1) = 2.0
	if math.Abs(s.ProfitFactor-2.0) > 1e-9 {
		t.Errorf("ProfitFactor: want 2.0, got %f", s.ProfitFactor)
	}
}

// Test 9: ComputeStats empty → zero stats.
func TestComputeStats_Empty(t *testing.T) {
	s := ComputeStats(nil)
	if s.WinRate != 0 || s.Sharpe != 0 || s.MaxDrawdown != 0 {
		t.Error("expected zero stats for empty outcomes")
	}
}

// Test 10: calcMaxDrawdown — known series.
func TestCalcMaxDrawdown(t *testing.T) {
	// equity: 100 → 110 (+10) → 90 (-20); MDD = 20/110
	pnl := []float64{10.0, -20.0}
	mdd := calcMaxDrawdown(pnl)
	expected := 20.0 / 110.0
	if math.Abs(mdd-expected) > 1e-9 {
		t.Errorf("MaxDrawdown: want %f, got %f", expected, mdd)
	}
}
