package backtest

import "github.com/Ju571nK/Chatter/pkg/models"

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
