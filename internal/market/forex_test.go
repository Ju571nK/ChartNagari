package market

import (
	"github.com/Ju571nK/Chatter/pkg/models"
	"testing"
	"time"
)

func TestForexHoursDST(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	for _, day := range []string{"2026-01-04", "2026-03-08", "2026-07-05", "2026-11-01"} {
		d, _ := time.ParseInLocation("2006-01-02", day, ny)
		for _, tc := range []struct {
			hour, minute int
			open         bool
		}{{16, 59, false}, {17, 0, true}} {
			at := time.Date(d.Year(), d.Month(), d.Day(), tc.hour, tc.minute, 0, 0, ny)
			if IsOpen(models.AssetForex, at) != tc.open {
				t.Errorf("%v expected %v", at, tc.open)
			}
		}
	}
	friday := time.Date(2026, 10, 2, 16, 59, 0, 0, ny)
	if !IsForexOpen(friday) || IsForexOpen(friday.Add(time.Minute)) {
		t.Fatal("Friday close")
	}
	if IsForexOpen(time.Date(2026, 12, 25, 12, 0, 0, 0, ny)) || IsForexOpen(time.Date(2027, 1, 1, 12, 0, 0, 0, ny)) {
		t.Fatal("holidays")
	}
	if !IsOpen(models.AssetCrypto, friday.Add(48*time.Hour)) {
		t.Fatal("crypto closed")
	}
}

func TestInstrument(t *testing.T) {
	for _, tc := range []struct {
		symbol    string
		pip       float64
		precision int
		spread    float64
	}{{"EURUSD", .0001, 5, 1}, {"USDJPY", .01, 3, 1}, {"EURJPY", .01, 3, 2}, {"XAUUSD", .1, 2, 3}, {"XAGUSD", .01, 3, 3}} {
		got := Instrument(tc.symbol)
		if got.PipSize != tc.pip || got.Precision != tc.precision || got.DefaultSpreadPips != tc.spread {
			t.Errorf("%s: %+v", tc.symbol, got)
		}
	}
}
