package candlestick

import (
	"testing"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestBullishEngulfingSupports15MOnly(t *testing.T) {
	ctx := models.AnalysisContext{Symbol: "EURUSD", Timeframes: map[string][]models.OHLCV{
		"15M": {
			{Open: 1.1020, High: 1.1025, Low: 1.1005, Close: 1.1010},
			{Open: 1.1008, High: 1.1030, Low: 1.1000, Close: 1.1025},
		},
	}}
	sig, err := (&BullishEngulfingRule{}).Analyze(ctx)
	if err != nil || sig == nil || sig.Timeframe != "15M" || sig.Direction != "LONG" {
		t.Fatalf("15M-only context should emit bullish engulfing: sig=%+v err=%v", sig, err)
	}
}
