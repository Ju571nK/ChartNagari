package collector

import (
	"context"
	"encoding/json"
	"github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestForex15MProvidersDropOpenFutureAndIncompleteCandles(t *testing.T) {
	now := time.Date(2026, 10, 5, 10, 7, 0, 0, time.UTC)
	stamps := []time.Time{now.Truncate(time.Hour).Add(-30 * time.Minute), now.Truncate(time.Hour).Add(-15 * time.Minute), now.Truncate(time.Hour), now.Truncate(time.Hour).Add(15 * time.Minute)}
	for _, provider := range []string{"yahoo", "oanda"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if provider == "yahoo" {
					if r.URL.Query().Get("interval") != "15m" || r.URL.Query().Get("range") != "60d" {
						t.Errorf("Yahoo query %s", r.URL.RawQuery)
					}
					ts := []int64{}
					for _, stamp := range stamps {
						ts = append(ts, stamp.Unix())
					}
					json.NewEncoder(w).Encode(map[string]any{"chart": map[string]any{"result": []any{map[string]any{"timestamp": ts, "indicators": map[string]any{"quote": []any{map[string]any{"open": []float64{150, 150, 150, 150}, "high": []float64{151, 151, 151, 151}, "low": []float64{149, 149, 149, 149}, "close": []float64{150, 150, 150, 150}}}}}}}})
				} else {
					if r.URL.Query().Get("granularity") != "M15" {
						t.Errorf("OANDA query %s", r.URL.RawQuery)
					}
					candles := []any{}
					for _, stamp := range stamps {
						candles = append(candles, map[string]any{"complete": true, "time": stamp.Format(time.RFC3339), "volume": 10, "mid": map[string]string{"o": "150", "h": "151", "l": "149", "c": "150"}})
					}
					candles = append(candles, map[string]any{"complete": false, "time": stamps[1].Format(time.RFC3339), "mid": map[string]string{"o": "150", "h": "151", "l": "149", "c": "150"}})
					json.NewEncoder(w).Encode(map[string]any{"candles": candles})
				}
			}))
			defer server.Close()
			entry := config.SymbolEntry{Symbol: "USDJPY", Enabled: true}
			var bars []models.OHLCV
			var err error
			if provider == "yahoo" {
				c := NewYahooFXCollector(nil, nil, nil, time.Minute)
				c.SetBaseURL(server.URL)
				c.now = func() time.Time { return now }
				bars, err = c.Fetch(context.Background(), entry, "15M")
			} else {
				c := NewOANDACollector(nil, nil, nil, time.Minute, "test-token", "practice")
				c.SetBaseURL(server.URL)
				c.now = func() time.Time { return now }
				bars, err = c.Fetch(context.Background(), entry, "15M")
			}
			if err != nil || len(bars) != 2 {
				t.Fatalf("bars=%+v error=%v", bars, err)
			}
			if bars[1].Timeframe != "15M" || !bars[1].OpenTime.Equal(stamps[1]) {
				t.Fatal(bars)
			}
			if !seriesFresh(bars, "15M", now) || seriesFresh(bars, "15M", now.Truncate(time.Hour).Add(15*time.Minute)) {
				t.Fatal("quarter-hour freshness incorrect")
			}
		})
	}
}

func TestForex15MWeekendAndSundayFirstClose(t *testing.T) {
	friday := nyFXTime(t, "2026-10-09", 16).Add(45 * time.Minute)
	bars := []models.OHLCV{{OpenTime: friday, Open: 150, High: 151, Low: 149, Close: 150}}
	for _, now := range []time.Time{nyFXTime(t, "2026-10-10", 12), nyFXTime(t, "2026-10-11", 17).Add(14 * time.Minute)} {
		if !seriesFresh(bars, "15M", now) || len(completeFXBars(bars, "15M", now)) != 1 {
			t.Fatalf("Friday16:45 quarter stale/rejected at %s", now)
		}
	}
	if seriesFresh(bars, "15M", nyFXTime(t, "2026-10-11", 17).Add(15*time.Minute)) {
		t.Fatal("Friday candle remained fresh after Sunday first quarter close")
	}
}

func TestForex15MDoesNotExpandOtherAssetCollection(t *testing.T) {
	want := []string{"1W", "1D", "4H", "1H"}
	if got := LegacyTimeframes(append([]string{"15M"}, want...)); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if _, ok := yahooTFParams["15M"]; ok {
		t.Fatal("15M leaked into legacy Yahoo stock intervals")
	}
}
