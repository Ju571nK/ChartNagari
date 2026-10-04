package wyckoff

import (
	"testing"

	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestVolumeNoneSkipsConfirmationButPreservesPricePatterns(t *testing.T) {
	acc := makeCtx("EURUSD")
	acc.VolumeQuality = models.VolumeNone
	acc.Timeframes["1H"] = fillBars(20, 100, 102, 98, 99, 0)
	acc.Indicators["1H:EMA_50"] = 110

	dist := makeCtx("EURUSD")
	dist.VolumeQuality = models.VolumeNone
	dist.Timeframes["1H"] = fillBars(20, 100, 102, 98, 101, 0)
	dist.Indicators["1H:EMA_50"] = 90

	spring := makeCtx("EURUSD")
	spring.VolumeQuality = models.VolumeNone
	spring.Timeframes["1H"] = []models.OHLCV{
		makeBar(105, 108, 103, 106, 0), makeBar(104, 107, 102, 105, 0),
		makeBar(103, 106, 101, 104, 0), makeBar(102, 105, 98, 103, 0),
		makeBar(103, 106, 101, 104, 0),
	}
	spring.Indicators["1H:SWING_LOW"] = 100

	upthrust := makeCtx("EURUSD")
	upthrust.VolumeQuality = models.VolumeNone
	upthrust.Timeframes["1H"] = []models.OHLCV{
		makeBar(95, 99, 93, 97, 0), makeBar(96, 99, 94, 98, 0),
		makeBar(97, 99, 95, 98, 0), makeBar(98, 102, 96, 99, 0),
		makeBar(99, 100, 96, 97, 0),
	}
	upthrust.Indicators["1H:SWING_HIGH"] = 100

	for _, tc := range []struct {
		name      string
		impl      rule.AnalysisRule
		ctx       models.AnalysisContext
		direction string
	}{
		{"accumulation", &WyckoffAccumulationRule{}, acc, "LONG"},
		{"distribution", &WyckoffDistributionRule{}, dist, "SHORT"},
		{"spring", &WyckoffSpringRule{}, spring, "LONG"},
		{"upthrust", &WyckoffUpthrustRule{}, upthrust, "SHORT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sig, err := tc.impl.Analyze(tc.ctx)
			if err != nil || sig == nil {
				t.Fatalf("expected price-pattern signal, got signal=%+v err=%v", sig, err)
			}
			if sig.Direction != tc.direction || !sig.VolumeUnconfirmed {
				t.Fatalf("wrong no-volume signal: %+v", sig)
			}
		})
	}
}

func TestVolumeAnomalyNeverFiresWithNoVolume(t *testing.T) {
	ctx := makeCtx("EURUSD")
	ctx.VolumeQuality = models.VolumeNone
	ctx.Timeframes["1H"] = []models.OHLCV{makeBar(100, 102, 99, 101, 0)}
	ctx.Indicators["1H:VOLUME_MA_20"] = 0
	sig, err := (&WyckoffVolumeAnomalyRule{}).Analyze(ctx)
	if err != nil || sig != nil {
		t.Fatalf("got signal=%+v err=%v", sig, err)
	}
}
