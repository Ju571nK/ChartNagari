package market

import "strings"

type InstrumentSpec struct {
	PipSize           float64 `json:"pip_size"`
	Precision         int     `json:"precision"`
	DefaultSpreadPips float64 `json:"default_spread_pips"`
}

// Instrument returns FX quote conventions. Callers may override spread from a
// symbol profile; these values are the baseline for backtests and display.
func Instrument(symbol string) InstrumentSpec {
	switch symbol {
	case "XAUUSD":
		return InstrumentSpec{0.1, 2, 3}
	case "XAGUSD":
		return InstrumentSpec{0.01, 3, 3}
	}
	if strings.HasSuffix(symbol, "JPY") {
		return InstrumentSpec{0.01, 3, spread(symbol)}
	}
	return InstrumentSpec{0.0001, 5, spread(symbol)}
}

func spread(symbol string) float64 {
	switch symbol {
	case "EURUSD", "GBPUSD", "USDJPY", "USDCHF", "AUDUSD", "USDCAD", "NZDUSD":
		return 1
	default:
		return 2
	}
}
