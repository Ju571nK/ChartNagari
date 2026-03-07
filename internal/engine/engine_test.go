package engine

import (
	"errors"
	"testing"

	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── stub rule implementations ────────────────────────────────────────────────

// staticRule always returns a fixed signal with a given base Score.
type staticRule struct {
	name       string
	required   []string
	baseScore  float64
	direction  string
}

func (r *staticRule) Name() string                         { return r.name }
func (r *staticRule) RequiredIndicators() []string         { return r.required }
func (r *staticRule) Analyze(_ models.AnalysisContext) (*models.Signal, error) {
	return &models.Signal{
		Rule:      r.name,
		Score:     r.baseScore,
		Direction: r.direction,
	}, nil
}

// nilRule always returns nil (no signal condition met).
type nilRule struct{ name string }

func (r *nilRule) Name() string                                             { return r.name }
func (r *nilRule) RequiredIndicators() []string                             { return nil }
func (r *nilRule) Analyze(_ models.AnalysisContext) (*models.Signal, error) { return nil, nil }

// errorRule returns an error from Analyze.
type errorRule struct{ name string }

func (r *errorRule) Name() string                         { return r.name }
func (r *errorRule) RequiredIndicators() []string         { return nil }
func (r *errorRule) Analyze(_ models.AnalysisContext) (*models.Signal, error) {
	return nil, errors.New("analyze failed")
}

// trackingRule records whether Analyze was called.
type trackingRule struct {
	name     string
	required []string
	called   bool
}

func (r *trackingRule) Name() string                         { return r.name }
func (r *trackingRule) RequiredIndicators() []string         { return r.required }
func (r *trackingRule) Analyze(_ models.AnalysisContext) (*models.Signal, error) {
	r.called = true
	return &models.Signal{Rule: r.name, Score: 1.0}, nil
}

// ── helper ───────────────────────────────────────────────────────────────────

func emptyCtx() models.AnalysisContext {
	return models.AnalysisContext{
		Symbol:     "TEST",
		Timeframes: map[string][]models.OHLCV{},
		Indicators: map[string]float64{},
	}
}

func enabledEntry(tf string, weight float64) RuleEntry {
	return RuleEntry{Enabled: true, Timeframe: tf, Weight: weight}
}

// ── tests ────────────────────────────────────────────────────────────────────

// Test 1: empty engine returns empty slice (not nil) — actually nil is fine
// but we verify zero length.
func TestRun_EmptyEngine_ReturnsEmptySlice(t *testing.T) {
	e := New(RuleConfig{Rules: map[string]RuleEntry{}})
	got := e.Run(emptyCtx())
	if len(got) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(got))
	}
}

// Test 2: a disabled rule is never executed even after Register is called.
func TestRegister_DisabledRule_NotExecuted(t *testing.T) {
	cfg := RuleConfig{
		Rules: map[string]RuleEntry{
			"my_rule": {Enabled: false, Timeframe: "1H", Weight: 1.0},
		},
	}
	e := New(cfg)
	tr := &trackingRule{name: "my_rule", required: nil}
	e.Register(tr)

	e.Run(emptyCtx())
	if tr.called {
		t.Fatal("disabled rule's Analyze should not have been called")
	}
}

// Test 3: signal score = base score × TF weight × rule weight.
// Rule: 1W timeframe, weight=1.0, base score=1.0 → final score = 1.0 × 2.0 × 1.0 = 2.0
func TestRun_ScoreCalculation_TFWeightApplied(t *testing.T) {
	cfg := RuleConfig{
		Rules: map[string]RuleEntry{
			"score_rule": enabledEntry("1W", 1.0),
		},
	}
	e := New(cfg)
	e.Register(&staticRule{name: "score_rule", baseScore: 1.0})

	got := e.Run(emptyCtx())
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	const want = 2.0 // 1.0 × TFWeight("1W"=2.0) × Weight(1.0)
	if got[0].Score != want {
		t.Fatalf("expected score %.2f, got %.2f", want, got[0].Score)
	}
}

// Test 4: multiple rules → signals sorted by Score descending.
func TestRun_MultipleRules_SortedDescending(t *testing.T) {
	cfg := RuleConfig{
		Rules: map[string]RuleEntry{
			"low_rule":  enabledEntry("1H", 1.0),  // 1.0 × 1.0 × 1.0 = 1.0
			"mid_rule":  enabledEntry("4H", 1.0),  // 2.0 × 1.2 × 1.0 = 2.4
			"high_rule": enabledEntry("1D", 3.0),  // 1.0 × 1.5 × 3.0 = 4.5
		},
	}
	e := New(cfg)
	e.Register(&staticRule{name: "low_rule", baseScore: 1.0})
	e.Register(&staticRule{name: "mid_rule", baseScore: 2.0})
	e.Register(&staticRule{name: "high_rule", baseScore: 1.0})

	got := e.Run(emptyCtx())
	if len(got) != 3 {
		t.Fatalf("expected 3 signals, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Score > got[i-1].Score {
			t.Fatalf("signals not sorted descending: got[%d].Score=%.2f > got[%d].Score=%.2f",
				i, got[i].Score, i-1, got[i-1].Score)
		}
	}
}

// Test 5: rule with empty RequiredIndicators runs normally.
func TestRun_NoRequiredIndicators_Executes(t *testing.T) {
	cfg := RuleConfig{
		Rules: map[string]RuleEntry{
			"no_req_rule": enabledEntry("1H", 1.0),
		},
	}
	e := New(cfg)
	e.Register(&staticRule{name: "no_req_rule", baseScore: 3.0, required: []string{}})

	got := e.Run(emptyCtx())
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
}

// Test 6: rule with RequiredIndicators that are NOT in ctx.Indicators → skipped silently.
func TestRun_MissingRequiredIndicator_RuleSkipped(t *testing.T) {
	cfg := RuleConfig{
		Rules: map[string]RuleEntry{
			"req_rule": enabledEntry("1H", 1.0),
		},
	}
	e := New(cfg)
	tr := &trackingRule{
		name:     "req_rule",
		required: []string{"RSI_14", "EMA_200"},
	}
	e.Register(tr)

	// Provide only one of the two required indicators.
	ctx := emptyCtx()
	ctx.Indicators["RSI_14"] = 55.0

	got := e.Run(ctx)
	if len(got) != 0 {
		t.Fatalf("expected 0 signals (rule skipped), got %d", len(got))
	}
	if tr.called {
		t.Fatal("Analyze should not have been called when a required indicator is missing")
	}
}

// Test 7: rule whose Name() is not in config is silently skipped.
func TestRegister_UnknownRuleName_Skipped(t *testing.T) {
	e := New(RuleConfig{Rules: map[string]RuleEntry{}})
	tr := &trackingRule{name: "unknown_rule"}
	e.Register(tr)

	e.Run(emptyCtx())
	if tr.called {
		t.Fatal("rule not in config should never be executed")
	}
}

// Test 8: a rule that returns nil signal is treated as no signal (not an error).
func TestRun_NilSignal_Skipped(t *testing.T) {
	cfg := RuleConfig{
		Rules: map[string]RuleEntry{
			"nil_rule": enabledEntry("1H", 1.0),
		},
	}
	e := New(cfg)
	e.Register(&nilRule{name: "nil_rule"})

	got := e.Run(emptyCtx())
	if len(got) != 0 {
		t.Fatalf("expected 0 signals from nil-returning rule, got %d", len(got))
	}
}

// Test 9: TFWeight helper returns correct values for all defined timeframes.
func TestTFWeight(t *testing.T) {
	cases := []struct {
		tf   string
		want float64
	}{
		{"1W", 2.0},
		{"1D", 1.5},
		{"4H", 1.2},
		{"1H", 1.0},
		{"ALL", 1.0},
		{"unknown", 1.0},
		{"", 1.0},
	}
	for _, tc := range cases {
		got := TFWeight(tc.tf)
		if got != tc.want {
			t.Errorf("TFWeight(%q): want %.2f, got %.2f", tc.tf, tc.want, got)
		}
	}
}

// Test 10: rule returns an error — signal is dropped, no panic.
func TestRun_AnalyzeError_SignalDropped(t *testing.T) {
	cfg := RuleConfig{
		Rules: map[string]RuleEntry{
			"err_rule": enabledEntry("1H", 1.0),
		},
	}
	e := New(cfg)
	e.Register(&errorRule{name: "err_rule"})

	got := e.Run(emptyCtx())
	if len(got) != 0 {
		t.Fatalf("expected 0 signals from error rule, got %d", len(got))
	}
}

// Compile-time interface check.
var _ rule.AnalysisRule = (*staticRule)(nil)
var _ rule.AnalysisRule = (*nilRule)(nil)
var _ rule.AnalysisRule = (*errorRule)(nil)
var _ rule.AnalysisRule = (*trackingRule)(nil)
