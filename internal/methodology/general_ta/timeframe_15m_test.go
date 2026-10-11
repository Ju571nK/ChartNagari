package general_ta

import (
	"testing"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestRSIOverboughtOversoldSupports15MAndPreserves1HPriority(t *testing.T) {
	rule := &RSIOverboughtOversoldRule{}
	ctx := models.AnalysisContext{Symbol: "EURUSD", Timeframes: map[string][]models.OHLCV{"15M": {{}}}, Indicators: map[string]float64{"15M:RSI_14": 20}}
	sig, err := rule.Analyze(ctx)
	if err != nil || sig == nil || sig.Timeframe != "15M" {
		t.Fatalf("15M-only context should produce a 15M signal: sig=%+v err=%v", sig, err)
	}
	ctx.Timeframes["1H"] = []models.OHLCV{{}}
	ctx.Indicators["1H:RSI_14"] = 20
	sig, err = rule.Analyze(ctx)
	if err != nil || sig == nil || sig.Timeframe != "1H" {
		t.Fatalf("1H must retain priority over the equal-weight 15M fallback: sig=%+v err=%v", sig, err)
	}
}
