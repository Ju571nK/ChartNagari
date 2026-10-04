package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func fxFixture(direction string) []models.OHLCV {
	bars := make([]models.OHLCV, 16)
	for i := range bars {
		bars[i] = models.OHLCV{Symbol: "EURUSD", Timeframe: "1H", OpenTime: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour), Open: 1.1, High: 1.1005, Low: 1.0995, Close: 1.1, Volume: 0}
	}
	if direction == "LONG" {
		bars[15].High = 1.1011
	} else {
		bars[15].Low = 1.0989
	}
	return bars
}

func TestForexPipsAndSpreadBothDirections(t *testing.T) {
	for _, tc := range []struct {
		name string
		impl rule.AnalysisRule
	}{
		{"LONG", &alwaysLongRule{score: 1}}, {"SHORT", &alwaysShortRule{score: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{WarmupBars: 14, MaxExitBars: 1, TPATRMultiplier: 1, SLATRMultiplier: 1, AssetClass: models.AssetForex}
			eng := New([]rule.AnalysisRule{tc.impl}, engCfgFor(tc.impl.Name()), cfg)
			with := eng.Run("EURUSD", "1H", "", fxFixture(tc.name))
			zero := 0.0
			cfg.SpreadPipsOverride = &zero
			without := eng.Clone(cfg).Run("EURUSD", "1H", "", fxFixture(tc.name))
			if with.Trades != 1 || without.Trades != 1 {
				t.Fatalf("expected one trade each: %+v %+v", with, without)
			}
			gross := without.Outcomes[0]
			net := with.Outcomes[0]
			if math.Abs(gross.Pips-10) > 1e-6 || math.Abs(net.Pips-9) > 1e-6 {
				t.Fatalf("expected 10/9 pips, got %.8f/%.8f", gross.Pips, net.Pips)
			}
			if math.Abs(with.Stats.NetPips-9) > 1e-6 || math.Abs(with.Stats.AvgPips-9) > 1e-6 {
				t.Fatalf("wrong stats: %+v", with.Stats)
			}
			if math.Abs(math.Abs(net.EntryPrice-gross.EntryPrice)-0.00005) > 1e-10 {
				t.Fatalf("entry does not pay half spread: %+v %+v", net, gross)
			}
			if math.Abs(math.Abs(net.ExitPrice-gross.ExitPrice)-0.00005) > 1e-10 {
				t.Fatalf("exit does not pay half spread: %+v %+v", net, gross)
			}
		})
	}
}

func TestRunnerUsesWatchlistClassAndValidatesSpread(t *testing.T) {
	cfg := Config{WarmupBars: 14, MaxExitBars: 1, TPATRMultiplier: 1, SLATRMultiplier: 1}
	runner := NewRunner(preparationStore(fxFixture("LONG")), New([]rule.AnalysisRule{&alwaysLongRule{score: 1}}, engCfgFor("always_long"), cfg))
	stock, err := runner.RunBacktest("EURUSD", "1H", "", 0, 0)
	if err != nil || stock.Trades != 1 || stock.Outcomes[0].Pips != 0 {
		t.Fatalf("unresolved symbol should keep stock behaviour: %+v %v", stock, err)
	}
	runner.WithAssetClassResolver(func(string) models.AssetClass { return models.AssetForex })
	zero := 0.0
	fx, err := runner.RunBacktestWithSpread("EURUSD", "1H", "", 0, 0, &zero)
	if err != nil || fx.Trades != 1 || math.Abs(fx.Outcomes[0].Pips-10) > 1e-6 {
		t.Fatalf("resolver/zero-spread failed: %+v %v", fx, err)
	}
	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := runner.RunBacktestWithSpread("EURUSD", "1H", "", 0, 0, &invalid); err == nil {
			t.Errorf("accepted invalid spread %v", invalid)
		}
	}
}

type qualityCaptureRule struct{ qualities []models.VolumeQuality }

func (r *qualityCaptureRule) Name() string                 { return "quality_capture" }
func (r *qualityCaptureRule) RequiredIndicators() []string { return nil }
func (r *qualityCaptureRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	r.qualities = append(r.qualities, ctx.VolumeQuality)
	return nil, nil
}

type sessionCaptureRule struct{ sessions []models.SessionContext }

func (r *sessionCaptureRule) Name() string                 { return "session_capture" }
func (r *sessionCaptureRule) RequiredIndicators() []string { return nil }
func (r *sessionCaptureRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	r.sessions = append(r.sessions, ctx.Session)
	return nil, nil
}

func TestFourHourReplayLeavesExactAsianRangeUnavailable(t *testing.T) {
	bars := make([]models.OHLCV, 16)
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		bars[i] = models.OHLCV{OpenTime: start.Add(time.Duration(i*4) * time.Hour),
			Open: 100, High: 110, Low: 90, Close: 100}
	}
	capture := &sessionCaptureRule{}
	eng := New([]rule.AnalysisRule{capture}, engCfgFor(capture.Name()), Config{WarmupBars: 14, MaxExitBars: 1})
	eng.Run("EURUSD", "4H", "", bars)
	if len(capture.sessions) != 1 || capture.sessions[0].AsianRangeValid || capture.sessions[0].Asia.Open.IsZero() {
		t.Fatalf("4H replay should expose windows but no synthetic range: %+v", capture.sessions)
	}
}

func TestBacktestUsesCurrentPersistedVolumeQuality(t *testing.T) {
	bars := append(fxFixture("LONG"), models.OHLCV{Symbol: "EURUSD", Timeframe: "1H", OpenTime: time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC), Open: 1.1, High: 1.1005, Low: 1.0995, Close: 1.1})
	bars[14].VolumeQuality = models.VolumeNone
	bars[15].VolumeQuality = models.VolumeTick // OANDA may have zero ticks for this candle
	capture := &qualityCaptureRule{}
	cfg := Config{WarmupBars: 14, MaxExitBars: 1, TPATRMultiplier: 1, SLATRMultiplier: 1, AssetClass: models.AssetForex}
	eng := New([]rule.AnalysisRule{capture}, engCfgFor(capture.Name()), cfg)
	eng.Run("EURUSD", "1H", "", bars)
	if len(capture.qualities) != 2 || capture.qualities[0] != models.VolumeNone || capture.qualities[1] != models.VolumeTick {
		t.Fatalf("quality looked ahead or ignored source: %v", capture.qualities)
	}
}
