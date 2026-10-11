package api

import (
	"encoding/json"
	"github.com/Ju571nK/Chatter/internal/backtest"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTimeframeOptInPreservesSymbolsAndRestartsGeneration(t *testing.T) {
	s := setupTest(t)
	before, err := s.readWatchlist()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.WithWatchlistChanged(func(w appconfig.WatchlistConfig) { calls++ })
	rr := do(t, s, "PUT", "/api/watchlist/timeframes", map[string]any{"timeframes": []string{"1H", "4H", "1D", "1W", "15M"}})
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	after, err := s.readWatchlist()
	if err != nil || !reflect.DeepEqual(before.Symbols.Crypto, after.Symbols.Crypto) || !reflect.DeepEqual(before.Symbols.Stocks, after.Symbols.Stocks) || len(before.Symbols.Indices) != len(after.Symbols.Indices) || len(before.Symbols.Forex) != len(after.Symbols.Forex) || calls != 1 || len(after.Timeframes) != 5 {
		t.Fatalf("preservation/callback: %+v %v calls%d", after, err, calls)
	}
	rr = do(t, s, "GET", "/api/watchlist/timeframes", nil)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "15M") {
		t.Fatal(rr.Body.String())
	}
	for _, bad := range []string{`{"timeframes":["15M"]}`, `{"timeframes":["1H","1H"]}`, `{"timeframes":["5M"]}`, `{"timeframes":null}`, `{"timeframes":["1H"],"symbols":[]}`, `{"timeframes":["1H"]}{}`} {
		rr = httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest("PUT", "/api/watchlist/timeframes", strings.NewReader(bad)))
		if rr.Code != 400 {
			t.Fatalf("bad%s code%d", bad, rr.Code)
		}
	}
	if calls != 1 {
		t.Fatal("invalid request restarted runtime")
	}
	s.apiToken = "test-auth"
	rr = do(t, s, "PUT", "/api/watchlist/timeframes", map[string]any{"timeframes": []string{"1H", "4H"}})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated mutation %d", rr.Code)
	}
}

func TestForex15MChartReadinessAndBacktestRoundTrip(t *testing.T) {
	s := setupTest(t)
	if rr := do(t, s, "POST", "/api/symbols", map[string]string{"symbol": "USDJPY", "type": "forex"}); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	db, err := storage.New(filepath.Join(t.TempDir(), "fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var bars []models.OHLCV
	for i := 0; i < 224; i++ {
		bars = append(bars, models.OHLCV{Symbol: "USDJPY", Timeframe: "15M", OpenTime: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * 15 * time.Minute), Open: 150, High: 151, Low: 149, Close: 150, VolumeQuality: models.VolumeNone})
	}
	if err = db.SaveOHLCVBatch(bars, "yahoo_fx"); err != nil {
		t.Fatal(err)
	}
	s.WithChartStore(db)
	cfg := backtest.DefaultConfig()
	cfg.AssetClass = models.AssetForex
	s.WithBacktestRunner(backtest.NewRunner(db, backtest.New(nil, engine.RuleConfig{}, cfg)))
	rr := do(t, s, "GET", "/api/ohlcv/USDJPY/15M", nil)
	var got []OHLCVBar
	if rr.Code != 200 {
		t.Fatalf("chart%d %s", rr.Code, rr.Body.String())
	}
	if err = json.Unmarshal(rr.Body.Bytes(), &got); err != nil || len(got) != 200 || got[1].Time-got[0].Time != 900 {
		t.Fatalf("chart duration/shape:%v %v", len(got), err)
	}
	rr = do(t, s, "GET", "/api/backtest/readiness?symbol=USDJPY&timeframe=15M", nil)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"ready":true`) {
		t.Fatal(rr.Body.String())
	}
	rr = do(t, s, "POST", "/api/backtest", map[string]string{"symbol": "USDJPY", "timeframe": "15M"})
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"timeframe":"15M"`) {
		t.Fatal(rr.Body.String())
	}
	for _, path := range []string{"/api/ohlcv/AAPL/15M", "/api/backtest/readiness?symbol=AAPL&timeframe=15M", "/api/backtest/rules?symbol=AAPL&timeframe=15M"} {
		rr = do(t, s, "GET", path, nil)
		if rr.Code != 400 {
			t.Fatalf("non-FX admitted %s:%d", path, rr.Code)
		}
	}
}

func TestForex15MOverrideIsExplicitAndLegacy4HPreserved(t *testing.T) {
	s := setupTest(t)
	if rr := do(t, s, "POST", "/api/symbols", map[string]string{"symbol": "USDJPY", "type": "forex"}); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	db, err := storage.New(filepath.Join(t.TempDir(), "overrides.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.overrideStore = storage.NewSymbolOverrideStore(db)
	s.profileHolder = appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{})
	wl, err := s.readWatchlist()
	if err != nil {
		t.Fatal(err)
	}
	s.profileHolder.SetWatchlist(wl)
	rr := do(t, s, "PUT", "/api/symbol-overrides/USDJPY", map[string]any{"timeframes": []string{"15M", "4H"}})
	if rr.Code != 200 {
		t.Fatalf("FXoverride%d %s", rr.Code, rr.Body.String())
	}
	rr = do(t, s, "PUT", "/api/symbol-overrides/AAPL", map[string]any{"timeframes": []string{"4H"}})
	if rr.Code != 200 {
		t.Fatalf("legacy4H rejected%d %s", rr.Code, rr.Body.String())
	}
	rr = do(t, s, "PUT", "/api/symbol-overrides/AAPL", map[string]any{"timeframes": []string{"15M"}})
	if rr.Code != 400 {
		t.Fatalf("non-FX15Moverride accepted%d", rr.Code)
	}
}
