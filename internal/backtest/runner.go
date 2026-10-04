package backtest

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/Ju571nK/Chatter/pkg/models"
)

var ErrInsufficientHistory = errors.New("insufficient historical data")

// RuleStats summarizes backtest performance for a single rule.
type RuleStats struct {
	Rule         string  `json:"rule"`
	Trades       int     `json:"trades"`
	WinRate      float64 `json:"win_rate"`
	AvgRR        float64 `json:"avg_rr"`
	ProfitFactor float64 `json:"profit_factor"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	TotalReturn  float64 `json:"total_return_pct"`
	NetPips      float64 `json:"net_pips,omitempty"`
	AvgPips      float64 `json:"avg_pips,omitempty"`
}

// OHLCVLoader is satisfied by *storage.DB.
// It loads the complete price history for a symbol+timeframe in ascending order.
type OHLCVLoader interface {
	GetOHLCVAll(symbol, timeframe string) ([]models.OHLCV, error)
}

// Runner combines an OHLCVLoader with an Engine to provide the single
// RunBacktest call consumed by the API server.
type Runner struct {
	store                 OHLCVLoader
	engine                *Engine
	assetClassResolver    func(string) models.AssetClass
	volumeQualityResolver func(string) models.VolumeQuality
}

// NewRunner creates a Runner.
func NewRunner(store OHLCVLoader, eng *Engine) *Runner {
	return &Runner{store: store, engine: eng}
}

// WithAssetClassResolver binds the watchlist resolver used in production.
// An unknown symbol must resolve to stock, never inferred from its spelling.
func (r *Runner) WithAssetClassResolver(resolve func(string) models.AssetClass) *Runner {
	r.assetClassResolver = resolve
	return r
}

// WithVolumeQualityResolver binds quality to the selected source. FX defaults
// to none until a provider explicitly identifies its bars as tick volume.
func (r *Runner) WithVolumeQualityResolver(resolve func(string) models.VolumeQuality) *Runner {
	r.volumeQualityResolver = resolve
	return r
}

// RunBacktest loads historical bars and runs the backtest engine.
// tpMult > 0 overrides the engine's default TPATRMultiplier.
// slMult > 0 overrides the engine's default SLATRMultiplier.
func (r *Runner) RunBacktest(symbol, timeframe, ruleFilter string, tpMult, slMult float64) (*BacktestResult, error) {
	return r.RunBacktestWithSpread(symbol, timeframe, ruleFilter, tpMult, slMult, nil)
}

// RunBacktestWithSpread applies an explicit spread override in pips. A non-nil
// pointer to zero permits frictionless baseline comparisons.
func (r *Runner) RunBacktestWithSpread(symbol, timeframe, ruleFilter string, tpMult, slMult float64, spreadPips *float64) (*BacktestResult, error) {
	if err := validateSpread(spreadPips); err != nil {
		return nil, err
	}
	bars, err := r.store.GetOHLCVAll(symbol, timeframe)
	if err != nil {
		return nil, err
	}
	if len(bars) < r.engine.cfg.WarmupBars+2 {
		return nil, fmt.Errorf("%w: need at least %d candles", ErrInsufficientHistory, r.engine.cfg.WarmupBars+2)
	}
	eng := r.engine
	if tpMult > 0 || slMult > 0 || spreadPips != nil || r.assetClassResolver != nil || r.volumeQualityResolver != nil {
		cfg := r.engine.cfg
		if tpMult > 0 {
			cfg.TPATRMultiplier = tpMult
		}
		if slMult > 0 {
			cfg.SLATRMultiplier = slMult
		}
		cfg.SpreadPipsOverride = spreadPips
		if r.assetClassResolver != nil {
			cfg.AssetClass = r.assetClassResolver(symbol)
		}
		if r.volumeQualityResolver != nil {
			cfg.VolumeQuality = r.volumeQualityResolver(symbol)
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
	return r.RunPerRuleWithSpread(symbol, timeframe, tpMult, slMult, nil)
}

func (r *Runner) RunPerRuleWithSpread(symbol, timeframe string, tpMult, slMult float64, spreadPips *float64) ([]RuleStats, error) {
	if err := validateSpread(spreadPips); err != nil {
		return nil, err
	}
	bars, err := r.store.GetOHLCVAll(symbol, timeframe)
	if err != nil {
		return nil, err
	}
	if len(bars) < r.engine.cfg.WarmupBars+2 {
		return nil, fmt.Errorf("%w: need at least %d candles", ErrInsufficientHistory, r.engine.cfg.WarmupBars+2)
	}
	eng := r.engine
	if tpMult > 0 || slMult > 0 || spreadPips != nil || r.assetClassResolver != nil || r.volumeQualityResolver != nil {
		cfg := r.engine.cfg
		if tpMult > 0 {
			cfg.TPATRMultiplier = tpMult
		}
		if slMult > 0 {
			cfg.SLATRMultiplier = slMult
		}
		cfg.SpreadPipsOverride = spreadPips
		if r.assetClassResolver != nil {
			cfg.AssetClass = r.assetClassResolver(symbol)
		}
		if r.volumeQualityResolver != nil {
			cfg.VolumeQuality = r.volumeQualityResolver(symbol)
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
			NetPips:      result.Stats.NetPips,
			AvgPips:      result.Stats.AvgPips,
		})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].WinRate > stats[j].WinRate })
	return stats, nil
}

func validateSpread(spread *float64) error {
	if spread != nil && (*spread < 0 || math.IsNaN(*spread) || math.IsInf(*spread, 0)) {
		return fmt.Errorf("spread_pips must be finite and non-negative")
	}
	return nil
}
