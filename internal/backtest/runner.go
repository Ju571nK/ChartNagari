package backtest

import (
	"sort"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// RuleStats summarizes backtest performance for a single rule.
type RuleStats struct {
	Rule         string  `json:"rule"`
	Trades       int     `json:"trades"`
	WinRate      float64 `json:"win_rate"`
	AvgRR        float64 `json:"avg_rr"`
	ProfitFactor float64 `json:"profit_factor"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	TotalReturn  float64 `json:"total_return_pct"`
}

// OHLCVLoader is satisfied by *storage.DB.
// It loads the complete price history for a symbol+timeframe in ascending order.
type OHLCVLoader interface {
	GetOHLCVAll(symbol, timeframe string) ([]models.OHLCV, error)
}

// Runner combines an OHLCVLoader with an Engine to provide the single
// RunBacktest call consumed by the API server.
type Runner struct {
	store  OHLCVLoader
	engine *Engine
}

// NewRunner creates a Runner.
func NewRunner(store OHLCVLoader, eng *Engine) *Runner {
	return &Runner{store: store, engine: eng}
}

// RunBacktest loads historical bars and runs the backtest engine.
// tpMult > 0 overrides the engine's default TPATRMultiplier.
// slMult > 0 overrides the engine's default SLATRMultiplier.
func (r *Runner) RunBacktest(symbol, timeframe, ruleFilter string, tpMult, slMult float64) (*BacktestResult, error) {
	bars, err := r.store.GetOHLCVAll(symbol, timeframe)
	if err != nil {
		return nil, err
	}
	eng := r.engine
	if tpMult > 0 || slMult > 0 {
		cfg := r.engine.cfg
		if tpMult > 0 {
			cfg.TPATRMultiplier = tpMult
		}
		if slMult > 0 {
			cfg.SLATRMultiplier = slMult
		}
		eng = r.engine.Clone(cfg)
	}
	res := eng.Run(symbol, timeframe, ruleFilter, bars)
	return &res, nil
}

// RunPerRule runs a backtest for each individual rule and returns per-rule stats.
// Rules with 0 trades are excluded from results.
// Results are sorted by WinRate descending.
func (r *Runner) RunPerRule(symbol, timeframe string, tpMult, slMult float64) ([]RuleStats, error) {
	bars, err := r.store.GetOHLCVAll(symbol, timeframe)
	if err != nil {
		return nil, err
	}
	eng := r.engine
	if tpMult > 0 || slMult > 0 {
		cfg := r.engine.cfg
		if tpMult > 0 {
			cfg.TPATRMultiplier = tpMult
		}
		if slMult > 0 {
			cfg.SLATRMultiplier = slMult
		}
		eng = r.engine.Clone(cfg)
	}
	ruleNames := r.engine.RuleNames()
	var stats []RuleStats
	for _, name := range ruleNames {
		result := eng.Run(symbol, timeframe, name, bars)
		if result.Trades == 0 {
			continue
		}
		stats = append(stats, RuleStats{
			Rule:         name,
			Trades:       result.Trades,
			WinRate:      result.Stats.WinRate,
			AvgRR:        result.Stats.AvgRR,
			ProfitFactor: result.Stats.ProfitFactor,
			MaxDrawdown:  result.Stats.MaxDrawdown,
			TotalReturn:  result.Stats.TotalReturnPct,
		})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].WinRate > stats[j].WinRate })
	return stats, nil
}
