package pipeline

import (
	"context"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/indicator"
	"github.com/Ju571nK/Chatter/internal/interpreter"
	"github.com/Ju571nK/Chatter/internal/notifier"
	"github.com/Ju571nK/Chatter/pkg/models"
	"github.com/rs/zerolog"
	"testing"
	"time"
)

type orderProbe struct {
	seen       []models.OHLCV
	indicators map[string]float64
}

func (*orderProbe) Name() string                 { return "order_probe" }
func (*orderProbe) RequiredIndicators() []string { return nil }
func (p *orderProbe) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	p.seen = ctx.Timeframes["1H"]
	p.indicators = ctx.Indicators
	return nil, nil
}

func TestLiveIndicatorsMatchChronologicalBacktest(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	asc := make([]models.OHLCV, 60)
	desc := make([]models.OHLCV, 60)
	for i := range asc {
		price := 1.0 + float64(i)*0.01
		asc[i] = models.OHLCV{Symbol: "EURUSD", Timeframe: "1H", OpenTime: now.Add(time.Duration(i) * time.Hour), Open: price, High: price + 0.005, Low: price - 0.005, Close: price, Volume: 10}
		desc[len(asc)-i-1] = asc[i]
	}
	db := &mockDB{bars: map[string][]models.OHLCV{"EURUSD|1H": desc}}
	eng := engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{"order_probe": {Enabled: true, Timeframe: "1H", Weight: 1}}})
	probe := &orderProbe{}
	eng.Register(probe)
	p := New(DefaultConfig(), db, eng, interpreter.New("", 12, "en"), notifier.New(notifier.DefaultConfig(), zerolog.Nop()), []string{"EURUSD"}, []string{"1H"}, zerolog.Nop())
	p.SetForexSymbols([]string{"EURUSD"}, models.VolumeNone, false)
	p.analyzeSymbol(context.Background(), "EURUSD")
	want := indicator.Compute(map[string][]models.OHLCV{"1H": asc})
	for _, key := range []string{"1H:RSI_14", "1H:EMA_20", "1H:ATR_14"} {
		if got := probe.indicators[key]; got != want[key] {
			t.Fatalf("%s live=%v backtest=%v", key, got, want[key])
		}
	}
	if probe.indicators["1H:RSI_14"] < 90 {
		t.Fatalf("rising prices gave RSI %v", probe.indicators["1H:RSI_14"])
	}
}

func TestRuleReceivesChronologicalCopy(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	newest := models.OHLCV{Symbol: "EURUSD", Timeframe: "1H", OpenTime: now.Add(2 * time.Hour), Close: 3}
	middle := models.OHLCV{Symbol: "EURUSD", Timeframe: "1H", OpenTime: now.Add(time.Hour), Close: 2}
	oldest := models.OHLCV{Symbol: "EURUSD", Timeframe: "1H", OpenTime: now, Close: 1}
	db := &mockDB{bars: map[string][]models.OHLCV{"EURUSD|1H": {newest, middle, oldest}}}
	eng := engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{"order_probe": {Enabled: true, Timeframe: "1H", Weight: 1}}})
	probe := &orderProbe{}
	eng.Register(probe)
	p := New(DefaultConfig(), db, eng, interpreter.New("", 12, "en"), notifier.New(notifier.DefaultConfig(), zerolog.Nop()), []string{"EURUSD"}, []string{"1H"}, zerolog.Nop())
	p.SetForexSymbols([]string{"EURUSD"}, models.VolumeNone, false)
	p.analyzeSymbol(context.Background(), "EURUSD")
	if len(probe.seen) != 3 || probe.seen[0].Close != 1 || probe.seen[2].Close != 3 {
		t.Fatalf("rule order: %+v", probe.seen)
	}
	if db.bars["EURUSD|1H"][0].Close != 3 {
		t.Fatal("mutated storage slice")
	}
}

func TestAssetClassResolver(t *testing.T) {
	p := newTestPipeline(&mockDB{bars: map[string][]models.OHLCV{}}, []string{"BTCUSDT"})
	p.SetForexSymbols([]string{"EURUSD"}, models.VolumeNone, false)
	p.SetIndexSymbols([]string{"DX-Y.NYB"})
	for _, tc := range []struct {
		symbol string
		want   models.AssetClass
	}{{"BTCUSDT", models.AssetCrypto}, {"EURUSD", models.AssetForex}, {"DX-Y.NYB", models.AssetIndex}, {"AAPL", models.AssetStock}, {"UNKNOWN", models.AssetStock}} {
		if got := p.assetClass(tc.symbol); got != tc.want {
			t.Errorf("%s got %s want %s", tc.symbol, got, tc.want)
		}
	}
}

func TestForexReadinessIsPerSymbol(t *testing.T) {
	p := newTestPipeline(&mockDB{bars: map[string][]models.OHLCV{}}, []string{"EURUSD", "GBPUSD"})
	p.SetForexSymbols([]string{"EURUSD", "GBPUSD"}, models.VolumeNone, false)
	p.SetForexReady(func() bool { return false }) // aggregate degraded
	p.SetForexSymbolReady(func(symbol string) bool { return symbol == "GBPUSD" })
	if p.forexAnalysisReady("EURUSD") || !p.forexAnalysisReady("GBPUSD") {
		t.Fatal("aggregate status hid the healthy pair")
	}
}
