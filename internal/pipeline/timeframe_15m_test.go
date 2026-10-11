package pipeline

import (
	"testing"

	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/methodology/general_ta"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestSupplemental15MScanEmitsAlongsideExistingHourlySignal(t *testing.T) {
	rule := &general_ta.RSIOverboughtOversoldRule{}
	eng := engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{
		rule.Name(): {Enabled: true, Timeframe: "1H", Weight: 1},
	}})
	eng.Register(rule)
	ctx := models.AnalysisContext{
		Symbol:     "EURUSD",
		AssetClass: models.AssetForex,
		Timeframes: map[string][]models.OHLCV{"1H": {{}}, "15M": {{}}},
		// The 15M value is more extreme, so a mixed scan would replace the
		// legacy 1H signal unless the primary pass excludes 15M.
		Indicators: map[string]float64{"1H:RSI_14": 20, "15M:RSI_14": 0},
	}
	primary := eng.Run(primaryFXContext(ctx))
	if len(primary) != 1 || primary[0].Timeframe != "1H" {
		t.Fatalf("primary scan should preserve hourly priority: %+v", primary)
	}
	seen := map[string]struct{}{signalIdentity(primary[0]): {}}
	additional := supplemental15MSignals(eng, ctx, seen)
	if len(additional) != 1 || additional[0].Timeframe != "15M" {
		t.Fatalf("supplemental FX scan should expose the 15M setup: %+v", additional)
	}
	if got := supplemental15MSignals(eng, ctx, seen); len(got) != 0 {
		t.Fatalf("duplicate supplemental scan should deduplicate: %+v", got)
	}
}
