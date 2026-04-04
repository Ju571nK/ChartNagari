// Package backtest provides a historical rule replay engine for performance analysis.
// It runs registered AnalysisRule plugins over a sliding OHLCV window,
// simulates ATR-based trade outcomes, and computes key performance statistics.
package backtest

import (
	"math"
	"time"

	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/indicator"
	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// TradeOutcome records the result of one simulated trade.
type TradeOutcome struct {
	EntryTime  time.Time `json:"entry_time"`
	EntryPrice float64   `json:"entry_price"`
	Direction  string    `json:"direction"` // "LONG" | "SHORT"
	Rule       string    `json:"rule"`
	Score      float64   `json:"score"`
	TP         float64   `json:"tp"`
	SL         float64   `json:"sl"`
	ExitPrice  float64   `json:"exit_price"`
	ExitBars   int       `json:"exit_bars"`
	Win        bool      `json:"win"`
	PnLPct     float64   `json:"pnl_pct"` // 수익률 %
}

// RegimeStats holds aggregated performance metrics for a single volatility regime.
type RegimeStats struct {
	Regime       string  `json:"regime"`          // "LOW_VOL", "NORMAL", "HIGH_VOL"
	Trades       int     `json:"trades"`
	WinRate      float64 `json:"win_rate"`
	AvgRR        float64 `json:"avg_rr"`
	ProfitFactor float64 `json:"profit_factor"`
	TotalReturn  float64 `json:"total_return_pct"`
}

// BacktestResult holds the full output of a backtest run.
type BacktestResult struct {
	Symbol      string         `json:"symbol"`
	Timeframe   string         `json:"timeframe"`
	Bars        int            `json:"bars"`
	Trades      int            `json:"trades"`
	Stats       Stats          `json:"stats"`
	Outcomes    []TradeOutcome `json:"outcomes"`
	RegimeStats []RegimeStats  `json:"regime_stats,omitempty"`
}

// Config controls the simulation parameters.
type Config struct {
	WarmupBars      int     // bars before the first signal (default 200)
	MaxExitBars     int     // max bars to wait for TP/SL (default 20)
	TPATRMultiplier float64 // TP = entry ± ATR × this (default 2.0)
	SLATRMultiplier float64 // SL = entry ∓ ATR × this (default 1.0)
}

// DefaultConfig returns sensible backtest defaults.
func DefaultConfig() Config {
	return Config{
		WarmupBars:      200,
		MaxExitBars:     20,
		TPATRMultiplier: 2.0,
		SLATRMultiplier: 1.0,
	}
}

// Engine runs a backtest over historical OHLCV bars.
type Engine struct {
	rules  []rule.AnalysisRule
	engCfg engine.RuleConfig
	cfg    Config
}

// New creates a backtest Engine with the given rules, rule config, and parameters.
func New(rules []rule.AnalysisRule, engCfg engine.RuleConfig, cfg Config) *Engine {
	return &Engine{rules: rules, engCfg: engCfg, cfg: cfg}
}

// NewWithConfig creates an Engine with explicit configuration.
// Used by Runner to apply per-request TP/SL overrides.
func NewWithConfig(rules []rule.AnalysisRule, engCfg engine.RuleConfig, cfg Config) *Engine {
	e := &Engine{cfg: cfg, engCfg: engCfg}
	e.rules = make([]rule.AnalysisRule, len(rules))
	copy(e.rules, rules)
	return e
}

// RuleNames returns the names of all rules loaded in this engine.
func (e *Engine) RuleNames() []string {
	names := make([]string, len(e.rules))
	for i, r := range e.rules {
		names[i] = r.Name()
	}
	return names
}

// Clone returns a new Engine with the same rules and rule config but a different simulation Config.
func (e *Engine) Clone(cfg Config) *Engine {
	clone := &Engine{cfg: cfg, engCfg: e.engCfg}
	clone.rules = make([]rule.AnalysisRule, len(e.rules))
	copy(clone.rules, e.rules)
	return clone
}

// Run replays all rules over bars (must be in ascending time order).
//
// For each bar from WarmupBars onward:
//  1. Builds an AnalysisContext with all bars up to and including that bar.
//  2. Runs the rule engine to collect signals.
//  3. For each non-NEUTRAL signal, simulates a trade using ATR-based TP/SL.
//
// ruleFilter: if non-empty, only outcomes from that rule are included.
func (e *Engine) Run(symbol, timeframe, ruleFilter string, bars []models.OHLCV) BacktestResult {
	result := BacktestResult{
		Symbol:    symbol,
		Timeframe: timeframe,
		Bars:      len(bars),
	}

	if e.cfg.WarmupBars >= len(bars)-1 {
		return result
	}

	// Build the live engine once; reuse across bar iterations.
	eng := engine.New(e.engCfg)
	for _, r := range e.rules {
		eng.Register(r)
	}

	for i := e.cfg.WarmupBars; i < len(bars)-1; i++ {
		ctx := buildContext(symbol, timeframe, bars[:i+1])
		signals := eng.Run(ctx)

		for _, sig := range signals {
			if sig.Direction == "NEUTRAL" {
				continue
			}
			if ruleFilter != "" && sig.Rule != ruleFilter {
				continue
			}

			atr := ctx.Indicators[timeframe+":ATR_14"]
			if atr <= 0 {
				continue
			}

			entry := bars[i].Close
			var tp, sl float64
			if sig.Direction == "LONG" {
				tp = entry + atr*e.cfg.TPATRMultiplier
				sl = entry - atr*e.cfg.SLATRMultiplier
			} else {
				tp = entry - atr*e.cfg.TPATRMultiplier
				sl = entry + atr*e.cfg.SLATRMultiplier
			}

			outcome := TradeOutcome{
				EntryTime:  bars[i].OpenTime,
				EntryPrice: entry,
				Direction:  sig.Direction,
				Rule:       sig.Rule,
				Score:      sig.Score,
				TP:         tp,
				SL:         sl,
			}

			limit := i + 1 + e.cfg.MaxExitBars
			if limit > len(bars) {
				limit = len(bars)
			}

			for j := i + 1; j < limit; j++ {
				bar := bars[j]
				exitBars := j - i

				if sig.Direction == "LONG" {
					if bar.High >= tp {
						outcome.Win, outcome.ExitPrice, outcome.ExitBars = true, tp, exitBars
						outcome.PnLPct = (tp - entry) / entry * 100
						break
					}
					if bar.Low <= sl {
						outcome.ExitPrice, outcome.ExitBars = sl, exitBars
						outcome.PnLPct = (sl - entry) / entry * 100
						break
					}
				} else {
					if bar.Low <= tp {
						outcome.Win, outcome.ExitPrice, outcome.ExitBars = true, tp, exitBars
						outcome.PnLPct = (entry - tp) / entry * 100
						break
					}
					if bar.High >= sl {
						outcome.ExitPrice, outcome.ExitBars = sl, exitBars
						outcome.PnLPct = (entry - sl) / entry * 100
						break
					}
				}

				// Timeout: exit at the last bar's close.
				if j == limit-1 {
					outcome.ExitPrice, outcome.ExitBars = bar.Close, exitBars
					if sig.Direction == "LONG" {
						outcome.PnLPct = (bar.Close - entry) / entry * 100
					} else {
						outcome.PnLPct = (entry - bar.Close) / entry * 100
					}
					outcome.Win = outcome.PnLPct > 0
				}
			}

			if outcome.ExitBars > 0 {
				result.Outcomes = append(result.Outcomes, outcome)
			}
		}
	}

	result.Trades = len(result.Outcomes)
	result.Stats = ComputeStats(result.Outcomes)
	result.RegimeStats = computeRegimeStats(result.Outcomes, bars)
	return result
}

// buildContext constructs an AnalysisContext from a window of bars.
func buildContext(symbol, timeframe string, bars []models.OHLCV) models.AnalysisContext {
	tfs := map[string][]models.OHLCV{timeframe: bars}
	return models.AnalysisContext{
		Symbol:     symbol,
		Timeframes: tfs,
		Indicators: indicator.Compute(tfs),
	}
}

// ── ATR percentile regime classification ─────────────────────────────────────

const (
	atrPeriod     = 14
	atrHistoryLen = 90
	lowVolPct     = 25.0
	highVolPct    = 75.0
)

// btATRPercentile computes the percentile rank (0-100) of the ATR at the given
// bar index within bars (ascending order). It mirrors the logic in
// internal/pipeline/regime.go but is local to avoid circular imports.
//
// Returns -1 when there is insufficient data.
func btATRPercentile(bars []models.OHLCV, barIdx int) float64 {
	// We need atrHistoryLen bars ending at barIdx (inclusive).
	if barIdx < atrHistoryLen-1 {
		return -1
	}
	window := bars[barIdx-atrHistoryLen+1 : barIdx+1] // ascending, length = atrHistoryLen

	if atrHistoryLen < atrPeriod+1 {
		return -1
	}

	tr := func(i int) float64 {
		hl := window[i].High - window[i].Low
		hc := math.Abs(window[i].High - window[i-1].Close)
		lc := math.Abs(window[i].Low - window[i-1].Close)
		return math.Max(hl, math.Max(hc, lc))
	}

	// Seed: simple average of the first atrPeriod true ranges.
	var seed float64
	for i := 1; i <= atrPeriod; i++ {
		seed += tr(i)
	}
	seed /= float64(atrPeriod)

	numATR := atrHistoryLen - atrPeriod
	if numATR <= 0 {
		return -1
	}
	atrHistory := make([]float64, 0, numATR)
	cur := seed
	atrHistory = append(atrHistory, cur)
	for i := atrPeriod + 1; i < atrHistoryLen; i++ {
		cur = (cur*float64(atrPeriod-1) + tr(i)) / float64(atrPeriod)
		atrHistory = append(atrHistory, cur)
	}

	currentATR := atrHistory[len(atrHistory)-1]

	count := 0
	for _, v := range atrHistory {
		if v <= currentATR {
			count++
		}
	}
	return float64(count) / float64(len(atrHistory)) * 100.0
}

// classifyRegime returns "LOW_VOL", "NORMAL", or "HIGH_VOL" for a given
// ATR percentile value. Returns "" when percentile is invalid (< 0).
func classifyRegime(pct float64) string {
	if pct < 0 {
		return ""
	}
	if pct < lowVolPct {
		return "LOW_VOL"
	}
	if pct > highVolPct {
		return "HIGH_VOL"
	}
	return "NORMAL"
}

// computeRegimeStats classifies each trade outcome by ATR percentile regime
// at its entry time and computes per-regime statistics.
func computeRegimeStats(outcomes []TradeOutcome, bars []models.OHLCV) []RegimeStats {
	if len(outcomes) == 0 || len(bars) == 0 {
		return nil
	}

	// Build a time-to-index lookup for fast bar matching.
	timeIdx := make(map[time.Time]int, len(bars))
	for i, b := range bars {
		timeIdx[b.OpenTime] = i
	}

	// Group outcomes by regime.
	groups := map[string][]TradeOutcome{}
	for _, o := range outcomes {
		idx, ok := timeIdx[o.EntryTime]
		if !ok {
			continue
		}
		pct := btATRPercentile(bars, idx)
		regime := classifyRegime(pct)
		if regime == "" {
			continue
		}
		groups[regime] = append(groups[regime], o)
	}

	// Compute stats for each regime.
	order := []string{"LOW_VOL", "NORMAL", "HIGH_VOL"}
	var result []RegimeStats
	for _, regime := range order {
		trades := groups[regime]
		if len(trades) == 0 {
			continue
		}
		s := ComputeStats(trades)
		result = append(result, RegimeStats{
			Regime:       regime,
			Trades:       len(trades),
			WinRate:      s.WinRate,
			AvgRR:        s.AvgRR,
			ProfitFactor: s.ProfitFactor,
			TotalReturn:  s.TotalReturnPct,
		})
	}
	return result
}
