package general_ta

import (
	"testing"

	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestPureVolumeRulesNeverFireWithNoVolume(t *testing.T) {
	ctx := makeCtx("EURUSD")
	ctx.VolumeQuality = models.VolumeNone
	ctx.Timeframes["1H"] = makeBars([]float64{100, 101, 102, 103, 104, 105})
	for i := range ctx.Timeframes["1H"] {
		ctx.Timeframes["1H"][i].Volume = 0
	}
	ctx.Indicators["1H:VOLUME_MA_20"] = 0
	ctx.Indicators["1H:ATR_14"] = 2
	for _, impl := range []rule.AnalysisRule{&VolumeSpikeRule{}, &VSAEffortCandleRule{}} {
		t.Run(impl.Name(), func(t *testing.T) {
			sig, err := impl.Analyze(ctx)
			if err != nil || sig != nil {
				t.Fatalf("got signal=%+v err=%v", sig, err)
			}
		})
	}
}

func TestTickVolumeAllowsSpike(t *testing.T) {
	ctx := makeCtx("EURUSD")
	ctx.VolumeQuality = models.VolumeTick
	ctx.Timeframes["1H"] = []models.OHLCV{{Open: 1.1, High: 1.11, Low: 1.09, Close: 1.105, Volume: 3000}}
	ctx.Indicators["1H:VOLUME_MA_20"] = 1000
	sig, err := (&VolumeSpikeRule{}).Analyze(ctx)
	if err != nil || sig == nil || sig.Direction != "LONG" {
		t.Fatalf("tick volume should permit spike: %+v %v", sig, err)
	}
}
