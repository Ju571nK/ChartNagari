package api

import (
	"encoding/json"
	"testing"

	"github.com/Ju571nK/Chatter/internal/backtest"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

type performanceSpreadRunner struct {
	calls map[string]*float64
}

func (r *performanceSpreadRunner) RunBacktest(string, string, string, float64, float64) (*backtest.BacktestResult, error) {
	return nil, nil
}

func (r *performanceSpreadRunner) RunPerRule(string, string, float64, float64) ([]backtest.RuleStats, error) {
	panic("performance ranks dropped the spread-aware runner")
}

func (r *performanceSpreadRunner) RunPerRuleWithSpread(symbol, _ string, _, _ float64, spread *float64) ([]backtest.RuleStats, error) {
	r.calls[symbol] = spread
	winRate := 50.0
	if spread != nil {
		winRate -= *spread
	}
	return []backtest.RuleStats{{Rule: "ema_cross", Trades: 1, WinRate: winRate}}, nil
}

func TestPerformanceRulesUsesEachSymbolsSpreadOverride(t *testing.T) {
	zero, ten := 0.0, 10.0
	s := setupTest(t)
	s.WithSymbolProfiles(appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		SymbolOverrides: map[string]appconfig.SymbolOverride{
			"EURUSD": {SpreadPips: &zero},
			"GBPUSD": {SpreadPips: &ten},
		},
	}))
	runner := &performanceSpreadRunner{calls: make(map[string]*float64)}
	s.WithBacktestRunner(runner)
	rr := do(t, s, "GET", "/api/performance/rules?symbols=EURUSD,GBPUSD,AAPL,BTCUSDT&timeframe=1H", nil)
	if rr.Code != 200 {
		t.Fatalf("response: %d %s", rr.Code, rr.Body.String())
	}
	for symbol, want := range map[string]*float64{"EURUSD": &zero, "GBPUSD": &ten, "AAPL": nil, "BTCUSDT": nil} {
		got, ok := runner.calls[symbol]
		if !ok || (want == nil && got != nil) || (want != nil && (got == nil || *got != *want)) {
			t.Fatalf("spread for %s: got %v, want %v", symbol, got, want)
		}
	}
	var ranks []AggregatedRuleStat
	if err := json.Unmarshal(rr.Body.Bytes(), &ranks); err != nil {
		t.Fatal(err)
	}
	if len(ranks) != 1 || ranks[0].SymbolsTested != 4 || ranks[0].AvgWinRate != 47.5 {
		t.Fatalf("spread was not reflected in aggregate: %+v", ranks)
	}
}
