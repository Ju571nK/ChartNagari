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

// RunBacktest loads historical bars from storage and runs the backtest engine.
// ruleFilter: empty string = all rules; otherwise only that rule's trades.
func (r *Runner) RunBacktest(symbol, timeframe, ruleFilter string) (*BacktestResult, error) {
	bars, err := r.store.GetOHLCVAll(symbol, timeframe)
	if err != nil {
		return nil, err
	}
	res := r.engine.Run(symbol, timeframe, ruleFilter, bars)
	return &res, nil
}
