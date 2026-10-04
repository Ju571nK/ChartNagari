package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func nyFXTime(t *testing.T, day string, hour int) time.Time {
	t.Helper()
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	d, err := time.ParseInLocation("2006-01-02", day, ny)
	if err != nil {
		t.Fatal(err)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, ny).UTC()
}

func TestFXFreshnessCountsOnlyExpectedTradingSessions(t *testing.T) {
	friday := nyFXTime(t, "2026-10-02", 16)
	bar := []models.OHLCV{{OpenTime: friday}}
	for _, at := range []time.Time{nyFXTime(t, "2026-10-03", 12), nyFXTime(t, "2026-10-04", 16)} {
		if !seriesFresh(bar, "1H", at) {
			t.Fatalf("Friday close should remain fresh at %s", at)
		}
	}
	if seriesFresh(bar, "1H", nyFXTime(t, "2026-10-04", 22)) {
		t.Fatal("Friday close became ready after Sunday trading resumed")
	}
	if !seriesFresh(bar, "1H", nyFXTime(t, "2026-10-04", 17).Add(time.Minute)) {
		t.Fatal("Friday close must remain ready until the first Sunday candle completes")
	}
	if seriesFresh(bar, "1H", nyFXTime(t, "2026-10-04", 18).Add(time.Minute)) {
		t.Fatal("Friday close remained ready after the first Sunday candle completed")
	}
	fourHour := []models.OHLCV{{OpenTime: nyFXTime(t, "2026-10-02", 13)}}
	if !seriesFresh(fourHour, "4H", nyFXTime(t, "2026-10-03", 12)) ||
		!seriesFresh(fourHour, "4H", nyFXTime(t, "2026-10-04", 21).Add(-time.Minute)) ||
		seriesFresh(fourHour, "4H", nyFXTime(t, "2026-10-04", 21).Add(time.Minute)) {
		t.Fatal("4H readiness did not follow the NY Sunday 17:00-21:00 bucket")
	}
	holidayEve := nyFXTime(t, "2026-12-24", 23)
	if !seriesFresh([]models.OHLCV{{OpenTime: holidayEve}}, "1H", nyFXTime(t, "2026-12-26", 12)) {
		t.Fatal("Christmas closure counted as active candle time")
	}
}

func TestYahooFXWeekendBackfillAndReopen(t *testing.T) {
	testFXWeekendBackfill(t, "yahoo_fx")
}

func TestOANDAWeekendBackfillAndReopen(t *testing.T) {
	testFXWeekendBackfill(t, "oanda")
}

func testFXWeekendBackfill(t *testing.T, source string) {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	friday := nyFXTime(t, "2026-10-02", 16)
	sunday := nyFXTime(t, "2026-10-04", 21)
	future := nyFXTime(t, "2026-10-05", 12)
	clock := nyFXTime(t, "2026-10-03", 12)
	broken := false
	includeSunday := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken && (strings.Contains(r.URL.Path, "EURUSD") || strings.Contains(r.URL.Path, "EUR_USD")) {
			http.Error(w, "upstream unavailable", 502)
			return
		}
		times := []time.Time{friday.Add(-time.Hour), friday, future}
		if includeSunday {
			times = append(times, sunday)
		}
		if source == "yahoo_fx" {
			stamp := make([]int64, len(times))
			open, high, low, close := make([]float64, len(times)), make([]float64, len(times)), make([]float64, len(times)), make([]float64, len(times))
			for i, at := range times {
				stamp[i], open[i], high[i], low[i], close[i] = at.Unix(), 1, 2, 0.5, 1.5
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"chart": map[string]any{"result": []any{map[string]any{"timestamp": stamp, "indicators": map[string]any{"quote": []any{map[string]any{"open": open, "high": high, "low": low, "close": close}}}}}}})
			return
		}
		candles := make([]any, 0, len(times)+1)
		for _, at := range times {
			candles = append(candles, map[string]any{"complete": true, "time": at.Format(time.RFC3339Nano), "volume": 10, "mid": map[string]string{"o": "1", "h": "2", "l": "0.5", "c": "1.5"}})
		}
		candles = append(candles, map[string]any{"complete": false, "time": friday.Add(time.Hour).Format(time.RFC3339Nano), "volume": 10, "mid": map[string]string{"o": "1", "h": "2", "l": "0.5", "c": "1.5"}})
		_ = json.NewEncoder(w).Encode(map[string]any{"candles": candles})
	}))
	defer server.Close()
	entries := []config.SymbolEntry{{Symbol: "EURUSD", Enabled: true}, {Symbol: "GBPUSD", Enabled: true}}
	var ready func(string) bool
	var issues func() map[string]string
	var poll func(bool) error
	if source == "yahoo_fx" {
		c := NewYahooFXCollector(db, entries, []string{"1H"}, time.Minute)
		c.SetBaseURL(server.URL)
		c.now = func() time.Time { return clock }
		ready, issues = c.Ready, c.Issues
		poll = func(initial bool) error { c.poll(context.Background(), initial); return nil }
	} else {
		c := NewOANDACollector(db, entries, []string{"1H"}, time.Minute, "token", "practice")
		c.SetBaseURL(server.URL)
		c.now = func() time.Time { return clock }
		ready, issues = c.Ready, c.Issues
		poll = func(initial bool) error { return c.poll(context.Background(), initial) }
	}
	if err := poll(true); err != nil || !ready("EURUSD") || !ready("GBPUSD") {
		t.Fatalf("Saturday backfill: err=%v issues=%v", err, issues())
	}
	stored, err := db.GetOHLCV("EURUSD", "1H", 10)
	if err != nil || len(stored) != 2 || !stored[0].OpenTime.Equal(friday) {
		t.Fatalf("historical bars lost or incomplete/future bars saved: %+v %v", stored, err)
	}
	for _, bar := range stored {
		if bar.Source != source {
			t.Fatalf("mixed source: %+v", stored)
		}
	}
	clock = nyFXTime(t, "2026-10-04", 16)
	broken = true
	if err := poll(true); err == nil && source == "oanda" {
		t.Fatal("expected per-series OANDA error")
	}
	if ready("EURUSD") || !ready("GBPUSD") {
		t.Fatalf("partial failure lost healthy readiness: %v", issues())
	}
	if err := poll(false); err == nil && source == "oanda" {
		t.Fatal("weekend skip erased prior series error")
	}
	if ready("EURUSD") || !ready("GBPUSD") {
		t.Fatalf("weekend skip erased prior series error: %v", issues())
	}
	clock = nyFXTime(t, "2026-10-04", 22)
	if err := poll(false); err == nil && source == "oanda" {
		t.Fatal("expected stale/error issue at reopen")
	}
	if ready("EURUSD") || ready("GBPUSD") {
		t.Fatalf("stale Friday candles became ready after reopen: %v", issues())
	}
	includeSunday = true
	broken = false
	if err := poll(false); err != nil || !ready("EURUSD") || !ready("GBPUSD") {
		t.Fatalf("fresh Sunday candle did not restore readiness: %v %v", err, issues())
	}
}
