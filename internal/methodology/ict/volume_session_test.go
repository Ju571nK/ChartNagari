package ict

import (
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestNoVolumeConfirmsPriceOnlyICT(t *testing.T) {
	fvg := makeCtx("EURUSD")
	fvg.VolumeQuality = models.VolumeNone
	fvg.Timeframes["1H"] = []models.OHLCV{
		makeBarWithVolume(95, 100, 93, 99, 0),
		makeBarWithVolume(101, 112, 100, 111, 0),
		makeBarWithVolume(110, 115, 108, 113, 0),
		makeBarWithVolume(108, 110, 101, 104, 0),
	}
	fvg.Indicators["1H:ATR_14"] = 5
	sweep := makeCtx("EURUSD")
	sweep.VolumeQuality = models.VolumeNone
	sweep.Timeframes["1H"] = []models.OHLCV{makeBarWithVolume(103, 110, 95, 108, 0)}
	sweep.Indicators["1H:SWING_LOW"] = 100

	for _, tc := range []struct {
		name    string
		ctx     models.AnalysisContext
		analyze func(models.AnalysisContext) (*models.Signal, error)
	}{
		{"fvg", fvg, (&ICTFairValueGapRule{}).Analyze},
		{"sweep", sweep, (&ICTLiquiditySweepRule{}).Analyze},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sig, err := tc.analyze(tc.ctx)
			if err != nil || sig == nil || !sig.VolumeUnconfirmed {
				t.Fatalf("expected unconfirmed price signal, got %+v, %v", sig, err)
			}
		})
	}
}

func TestKillZoneDSTBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, instant, session string }{
		{"winter London", "2026-01-15T07:30:00Z", "London"},
		{"summer London", "2026-07-15T06:30:00Z", "London"},
		{"spring after shift", "2026-03-09T06:30:00Z", "London"},
		{"autumn after shift", "2026-11-02T07:30:00Z", "London"},
		{"winter same UTC outside", "2026-01-15T06:30:00Z", ""},
		{"summer same UTC active", "2026-07-15T07:30:00Z", "London"},
		{"evening Asia", "2026-07-15T00:30:00Z", "Asia"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instant, _ := time.Parse(time.RFC3339, tc.instant)
			sig, err := (&ICTKillZoneRule{now: func() time.Time { return instant }}).Analyze(makeCtx("EURUSD"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.session == "" {
				if sig != nil {
					t.Fatalf("expected no session, got %+v", sig)
				}
				return
			}
			if sig == nil || sig.MessageParams["session"] != tc.session || sig.MessageKey != "signal.ict.kill_zone" || strings.Contains(sig.Message, "활성") {
				t.Fatalf("unexpected kill-zone signal: %+v", sig)
			}
		})
	}
}

func TestAMDDoesNotInventAsianRangeFromFourHourBars(t *testing.T) {
	ctx := makeCtx("EURUSD")
	// A four-hour candle opening in Asia can include prices from outside the
	// exact hourly range. Historical 4H-only replay must leave it unavailable.
	ctx.Timeframes["4H"] = []models.OHLCV{
		makeBarAtTime(0, 105, 110, 100, 105),
		makeBarAtTime(4, 105, 110, 100, 105),
		makeBarAtTime(8, 103, 107, 97, 101),
		makeBarAtTime(12, 102, 112, 101, 110),
	}
	if sig, err := (&ICTAMDSessionRule{}).Analyze(ctx); err != nil || sig != nil {
		t.Fatalf("4H-only Asian range must stay unavailable: signal=%+v err=%v", sig, err)
	}
}
