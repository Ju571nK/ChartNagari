package backtest

type Readiness struct {
	Symbol      string `json:"symbol"`
	Timeframe   string `json:"timeframe"`
	Bars        int    `json:"bars"`
	MinimumBars int    `json:"minimum_bars"`
	From        *int64 `json:"from"`
	To          *int64 `json:"to"`
	Ready       bool   `json:"ready"`
}

// Prepare reports the exact full-history input used by the engine, not the
// chart's capped 200-bar window. At least one entry and exit bar follow warmup.
func (r *Runner) Prepare(symbol, timeframe string) (*Readiness, error) {
	bars, err := r.store.GetOHLCVAll(symbol, timeframe)
	if err != nil {
		return nil, err
	}
	result := &Readiness{Symbol: symbol, Timeframe: timeframe, Bars: len(bars), MinimumBars: r.engine.cfg.WarmupBars + 2}
	result.Ready = result.Bars >= result.MinimumBars
	if len(bars) > 0 {
		first, last := bars[0].OpenTime.Unix(), bars[len(bars)-1].OpenTime.Unix()
		result.From, result.To = &first, &last
	}
	return result, nil
}
