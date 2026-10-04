package collector

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestYahooFXNY4H(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	for _, day := range []string{"2026-03-08", "2026-11-01"} {
		d, _ := time.ParseInLocation("2006-01-02", day, ny)
		start := time.Date(d.Year(), d.Month(), d.Day(), 17, 0, 0, 0, ny).UTC()
		if !NY4HBucket(start).Equal(start) || !NY4HBucket(start.Add(3*time.Hour)).Equal(start) || !NY4HBucket(start.Add(4*time.Hour)).Equal(start.Add(4*time.Hour)) {
			t.Fatal("bad bucket", day)
		}
		bars := make([]models.OHLCV, 4)
		for i := range bars {
			bars[i] = models.OHLCV{OpenTime: start.Add(time.Duration(i) * time.Hour), Open: 1, High: 2, Low: 0.5, Close: 1.5}
		}
		if got := RebuildFX4H("EURUSD", bars); len(got) != 1 || !got[0].OpenTime.Equal(start) {
			t.Fatalf("%s: %+v", day, got)
		}
		bars[2].OpenTime = bars[1].OpenTime
		if got := RebuildFX4H("EURUSD", bars); len(got) != 0 {
			t.Fatal("accepted incomplete bucket")
		}
	}
	if sym, proxy := YahooFXSymbol("XAUUSD"); sym != "GC=F" || !proxy {
		t.Fatal("gold proxy")
	}
}

func TestForexSourceIdentityIncludesEnvironmentMappingAndProxy(t *testing.T) {
	entry := config.SymbolEntry{Symbol: "XAUUSD"}
	yahoo := SourceForForex(entry, "yahoo", "")
	if yahoo.Identity != "yahoo:public:GC=F" || !yahoo.Proxy {
		t.Fatalf("Yahoo source: %+v", yahoo)
	}
	practice := SourceForForex(entry, "oanda", "practice")
	live := SourceForForex(entry, "oanda", "live")
	if practice.Identity == live.Identity || practice.Proxy || live.Proxy {
		t.Fatalf("OANDA environments: %+v %+v", practice, live)
	}
	entry.ProviderSymbol = "CUSTOM"
	if mapped := SourceForForex(entry, "oanda", "practice"); mapped.Identity == practice.Identity {
		t.Fatal("mapping did not change identity")
	}
	if SourceForForex(entry, "oanda", "practice", "token-a").Identity == SourceForForex(entry, "oanda", "practice", "token-b").Identity {
		t.Fatal("credentials did not change identity")
	}
}

func TestYahooFXPartialFailureKeepsHealthySymbolReady(t *testing.T) {
	clock := nyFXTime(t, "2026-10-01", 12)
	db, err := storage.New(filepath.Join(t.TempDir(), "fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := models.OHLCV{Symbol: "EURUSD", Timeframe: "1H", OpenTime: clock.Add(-30 * 24 * time.Hour), Close: 1}
	if err := db.SaveOHLCV(old, "yahoo_fx"); err != nil {
		t.Fatal(err)
	}
	fail := true
	empty := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "EURUSD") && fail {
			http.Error(w, "failed", 502)
			return
		}
		if strings.Contains(r.URL.Path, "EURUSD") && empty {
			json.NewEncoder(w).Encode(map[string]any{"chart": map[string]any{"result": []any{map[string]any{"timestamp": []int64{}, "indicators": map[string]any{"quote": []any{map[string]any{"close": []float64{}}}}}}}})
			return
		}
		ts := clock.Add(-time.Hour).Unix()
		json.NewEncoder(w).Encode(map[string]any{"chart": map[string]any{"result": []any{map[string]any{"timestamp": []int64{ts}, "indicators": map[string]any{"quote": []any{map[string]any{"open": []float64{1}, "high": []float64{2}, "low": []float64{0.5}, "close": []float64{1.5}, "volume": []float64{0}}}}}}}})
	}))
	defer server.Close()
	c := NewYahooFXCollector(db, []config.SymbolEntry{{Symbol: "EURUSD", Enabled: true}, {Symbol: "GBPUSD", Enabled: true}}, []string{"1H"}, time.Minute)
	c.baseURL = server.URL
	c.now = func() time.Time { return clock }
	success, failures := 0, 0
	c.OnSuccess(func() { success++ })
	c.OnError(func(err error) {
		failures++
		if !strings.Contains(err.Error(), "EURUSD/1H") {
			t.Errorf("missing affected series: %v", err)
		}
	})
	c.poll(context.Background(), true)
	if success != 0 || failures != 1 || c.Ready("EURUSD") || !c.Ready("GBPUSD") {
		t.Fatalf("ready EUR=%v GBP=%v success=%d failures=%d issues=%v", c.Ready("EURUSD"), c.Ready("GBPUSD"), success, failures, c.Issues())
	}
	stored, _ := db.GetOHLCV("EURUSD", "1H", 1)
	if len(stored) != 1 || stored[0].OpenTime.UnixMilli() != old.OpenTime.UnixMilli() {
		t.Fatal("stale fixture unexpectedly replaced")
	}
	fail = false
	empty = true
	c.poll(context.Background(), true)
	if c.Ready("EURUSD") || !c.Ready("GBPUSD") || success != 0 {
		t.Fatalf("empty candle marked ready: %v", c.Issues())
	}
	empty = false
	c.poll(context.Background(), true)
	if success != 1 || !c.Ready("EURUSD") || !c.Ready("GBPUSD") {
		t.Fatalf("recovery failed: %+v", c.Issues())
	}
}

func TestOANDAPartialFailureAndAuthStop(t *testing.T) {
	clock := nyFXTime(t, "2026-10-01", 12)
	db, err := storage.New(filepath.Join(t.TempDir(), "fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	status := 502
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "EUR_USD") {
			w.WriteHeader(status)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"candles": []any{map[string]any{"complete": true, "time": clock.Add(-time.Hour).Format(time.RFC3339Nano), "volume": 10, "mid": map[string]string{"o": "1", "h": "2", "l": "0.5", "c": "1.5"}}}})
	}))
	defer server.Close()
	c := NewOANDACollector(db, []config.SymbolEntry{{Symbol: "EURUSD", Enabled: true}, {Symbol: "GBPUSD", Enabled: true}}, []string{"1H"}, time.Minute, "token", "practice")
	c.baseURL = server.URL
	c.now = func() time.Time { return clock }
	success := 0
	c.OnSuccess(func() { success++ })
	if err := c.poll(context.Background(), true); err == nil || !strings.Contains(err.Error(), "EURUSD/1H") {
		t.Fatalf("partial error: %v", err)
	}
	if success != 0 || c.Ready("EURUSD") || !c.Ready("GBPUSD") {
		t.Fatalf("partial readiness: %v", c.Issues())
	}
	status = 401
	if err := c.poll(context.Background(), true); !errors.Is(err, ErrOANDATokenRejected) || c.Ready("GBPUSD") {
		t.Fatalf("auth rejection: %v %v", err, c.Issues())
	}
}

func TestOANDAFetch(t *testing.T) {
	status := 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/instruments/EUR_USD/candles" || r.URL.Query().Get("granularity") != "H4" || r.URL.Query().Get("price") != "M" || r.URL.Query().Get("dailyAlignment") != "17" || r.URL.Query().Get("alignmentTimezone") != "America/New_York" || r.URL.Query().Get("weeklyAlignment") != "Friday" || r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("bad request: %s", r.URL.String())
		}
		w.WriteHeader(status)
		if status == 200 {
			json.NewEncoder(w).Encode(map[string]any{"candles": []any{map[string]any{"complete": true, "time": "2026-10-01T21:00:00.000000000Z", "volume": 47, "mid": map[string]string{"o": "1.1000", "h": "1.2000", "l": "1.0000", "c": "1.1500"}}, map[string]any{"complete": false, "time": "2026-10-02T01:00:00Z", "volume": 3, "mid": map[string]string{"o": "1", "h": "1", "l": "1", "c": "1"}}}})
		}
	}))
	defer server.Close()
	c := NewOANDACollector(nil, nil, nil, time.Minute, "token", "practice")
	c.baseURL = server.URL
	bars, err := c.Fetch(context.Background(), config.SymbolEntry{Symbol: "EURUSD"}, "4H")
	if err != nil || len(bars) != 1 || bars[0].Volume != 47 || bars[0].Close != 1.15 {
		t.Fatalf("bars=%+v err=%v", bars, err)
	}
	status = 401
	if _, err = c.Fetch(context.Background(), config.SymbolEntry{Symbol: "EURUSD"}, "4H"); !errors.Is(err, ErrOANDATokenRejected) {
		t.Fatal(err)
	}
	status = 429
	if _, err = c.Fetch(context.Background(), config.SymbolEntry{Symbol: "EURUSD"}, "4H"); !errors.Is(err, ErrOANDARateLimited) {
		t.Fatal(err)
	}
}
