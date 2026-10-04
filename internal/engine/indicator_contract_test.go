package engine_test

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/indicator"
	generalta "github.com/Ju571nK/Chatter/internal/methodology/general_ta"
	"github.com/Ju571nK/Chatter/internal/methodology/ict"
	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func contractBars() []models.OHLCV {
	bars := make([]models.OHLCV, 30)
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		bars[i] = models.OHLCV{OpenTime: start.Add(time.Duration(i) * time.Hour),
			Open: 100, High: 101, Low: 99, Close: 100, Volume: 1000}
	}
	return bars
}

func contractContext(bars []models.OHLCV, quality models.VolumeQuality, asset models.AssetClass) models.AnalysisContext {
	if quality == models.VolumeNone {
		for i := range bars {
			bars[i].Volume = 0
		}
	}
	tfs := map[string][]models.OHLCV{"1H": bars}
	return models.AnalysisContext{Symbol: "EURUSD", AssetClass: asset, VolumeQuality: quality,
		Timeframes: tfs, Indicators: indicator.Compute(tfs)}
}

func runRealRule(impl rule.AnalysisRule, ctx models.AnalysisContext) []models.Signal {
	e := engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{
		impl.Name(): {Enabled: true, Timeframe: "1H", Weight: 1},
	}})
	e.Register(impl)
	return e.Run(ctx)
}

func TestRegisteredFVGAndVSAWithComputedIndicators(t *testing.T) {
	for _, quality := range []models.VolumeQuality{models.VolumeNone, models.VolumeTick, models.VolumeReal} {
		t.Run(string(quality), func(t *testing.T) {
			fvg := contractBars()
			fvg[26].Open, fvg[26].High, fvg[26].Low, fvg[26].Close = 99, 100, 98, 99
			fvg[27].Open, fvg[27].High, fvg[27].Low, fvg[27].Close = 100, 112, 100, 110
			fvg[28].Open, fvg[28].High, fvg[28].Low, fvg[28].Close = 108, 114, 105, 110
			fvg[29].Open, fvg[29].High, fvg[29].Low, fvg[29].Close = 104, 107, 101, 103
			ctx := contractContext(fvg, quality, models.AssetForex)
			if _, ok := ctx.Indicators["1H:ATR_14"]; !ok {
				t.Fatal("fixture lacks computed ATR")
			}
			// An absent volume MA must not suppress the price-only FX setup.
			if quality == models.VolumeNone {
				delete(ctx.Indicators, "1H:VOLUME_MA_20")
			}
			signals := runRealRule(&ict.ICTFairValueGapRule{}, ctx)
			if len(signals) != 1 || signals[0].Direction != "LONG" || signals[0].VolumeUnconfirmed != (quality == models.VolumeNone) {
				t.Fatalf("FVG quality=%s: %+v", quality, signals)
			}

			vsa := contractBars()
			for i := range vsa {
				price := 120 - float64(i)*0.5
				vsa[i].Open, vsa[i].High, vsa[i].Low, vsa[i].Close = price, price+0.5, price-0.5, price
			}
			vsa[29].Open, vsa[29].High, vsa[29].Low, vsa[29].Close, vsa[29].Volume = 104.1, 105.3, 103.8, 104, 5000
			signals = runRealRule(&generalta.VSAEffortCandleRule{}, contractContext(vsa, quality, models.AssetForex))
			if quality == models.VolumeNone {
				if len(signals) != 0 {
					t.Fatalf("volume-only rule fired without volume: %+v", signals)
				}
			} else if len(signals) != 1 || signals[0].Direction != "LONG" {
				t.Fatalf("VSA quality=%s: %+v", quality, signals)
			}
		})
	}
}

func TestRegisteredOrderBlockAndOTEWithComputedIndicators(t *testing.T) {
	ob := contractBars()
	ob[26].Open, ob[26].High, ob[26].Low, ob[26].Close = 101, 102, 98, 99
	ob[27].Open, ob[27].High, ob[27].Low, ob[27].Close = 99, 111, 99, 110
	ob[28].Open, ob[28].High, ob[28].Low, ob[28].Close = 110, 113, 109, 112
	ob[29].Open, ob[29].High, ob[29].Low, ob[29].Close = 100, 102, 98, 100
	obCtx := contractContext(ob, models.VolumeReal, models.AssetStock)
	if sig := runRealRule(&ict.ICTOrderBlockRule{}, obCtx); len(sig) != 1 || sig[0].Direction != "LONG" {
		t.Fatalf("registered OB did not run: %+v", sig)
	}
	delete(obCtx.Indicators, "1H:ATR_14")
	if sig := runRealRule(&ict.ICTOrderBlockRule{}, obCtx); len(sig) != 1 || sig[0].Direction != "LONG" {
		t.Fatalf("price-only OB was improperly gated: %+v", sig)
	}

	ote := contractBars()
	ote[12].Low = 80
	ote[20].High = 120
	ote[29].Open, ote[29].High, ote[29].Low, ote[29].Close = 92, 93, 91, 92
	ctx := contractContext(ote, models.VolumeReal, models.AssetCrypto)
	if _, ok := ctx.Indicators["1H:SWING_HIGH"]; !ok {
		t.Fatal("fixture lacks computed swing high")
	}
	if sig := runRealRule(&ict.ICTOTERule{}, ctx); len(sig) != 1 || sig[0].Direction != "LONG" {
		t.Fatalf("registered OTE did not run: %+v", sig)
	}
	delete(ctx.Indicators, "1H:SWING_HIGH")
	delete(ctx.Indicators, "1H:SWING_LOW")
	if sig := runRealRule(&ict.ICTOTERule{}, ctx); len(sig) != 1 || sig[0].Direction != "LONG" {
		t.Fatalf("OTE's own swing scan was improperly gated: %+v", sig)
	}
}
