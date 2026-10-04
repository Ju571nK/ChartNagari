package api

import (
	"encoding/json"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSavedFXSignalChartRoundTrip(t *testing.T) {
	s := setupTest(t)
	db, err := storage.New(filepath.Join(t.TempDir(), "chart.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.WithChartStore(db)
	sig := models.Signal{Symbol: "XAUUSD", Timeframe: "ALL", Rule: "ict_kill_zone", Direction: "NEUTRAL", Score: 1, Message: "London kill zone active (03:00 New York)", CreatedAt: time.Now(), AssetClass: models.AssetForex, VolumeQuality: models.VolumeNone, DataProvider: "yahoo", ProviderSymbol: "GC=F", SourceIdentity: "yahoo:public:GC=F", DataProxy: true, MessageKey: "signal.ict.kill_zone", MessageParams: map[string]string{"session": "London", "time": "03:00", "timezone": "America/New_York"}}
	if _, err := db.SaveSignal(sig); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/signals?symbol=XAUUSD", "/api/history?symbol=XAUUSD"} {
		rr := do(t, s, "GET", path, nil)
		if rr.Code != 200 {
			t.Fatalf("%s: %s", path, rr.Body.String())
		}
		var got []SignalBar
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Timeframe != "ALL" || got[0].Direction != "NEUTRAL" || got[0].MessageKey != sig.MessageKey || got[0].MessageParams["session"] != "London" || got[0].MessageParams["time"] != "03:00" || got[0].MessageParams["timezone"] != "America/New_York" || got[0].SourceIdentity != sig.SourceIdentity || !got[0].DataProxy {
			t.Fatalf("%s: %+v", path, got)
		}
	}
}

func TestDXYDisabledWithRetainedCandles(t *testing.T) {
	s := setupTest(t)
	db, err := storage.New(filepath.Join(t.TempDir(), "dxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.WithChartStore(db)
	if rr := do(t, s, "POST", "/api/symbols", map[string]string{"symbol": "EURUSD", "type": "forex"}); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	if err := db.SaveOHLCV(models.OHLCV{Symbol: "DX-Y.NYB", Timeframe: "1D", OpenTime: time.Now().Add(-24 * time.Hour), Close: 100}, "yahoo"); err != nil {
		t.Fatal(err)
	}
	if rr := do(t, s, "DELETE", "/api/symbols/DX-Y.NYB", nil); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	rr := do(t, s, "GET", "/api/forex/dxy?symbol=EURUSD", nil)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"state":"disabled"`) {
		t.Fatalf("DXY was still served: %d %s", rr.Code, rr.Body.String())
	}
}

func TestForexSymbolLifecycleAndDXYOptOut(t *testing.T) {
	s := setupTest(t)
	if rr := do(t, s, "POST", "/api/symbols", map[string]string{"symbol": "ZZZUSD", "type": "forex"}); rr.Code != 400 {
		t.Fatalf("invalid pair accepted: %d", rr.Code)
	}
	if rr := do(t, s, "POST", "/api/symbols", map[string]string{"symbol": "EURUSD", "type": "forex"}); rr.Code != 204 {
		t.Fatalf("add: %s", rr.Body.String())
	}
	wl, err := s.readWatchlist()
	if err != nil || len(wl.Symbols.Forex) != 1 || len(wl.Symbols.Indices) != 1 || wl.Symbols.Indices[0].Symbol != "DX-Y.NYB" {
		t.Fatalf("watchlist: %+v %v", wl, err)
	}
	if rr := do(t, s, "DELETE", "/api/symbols/DX-Y.NYB", nil); rr.Code != 204 {
		t.Fatalf("remove DXY: %d", rr.Code)
	}
	if rr := do(t, s, "POST", "/api/symbols", map[string]string{"symbol": "GBPUSD", "type": "forex"}); rr.Code != 204 {
		t.Fatalf("second pair: %d", rr.Code)
	}
	wl, _ = s.readWatchlist()
	if len(wl.Symbols.Indices) != 0 || !wl.DXYAutoInitialized {
		t.Fatalf("DXY re-added: %+v", wl)
	}
	if rr := do(t, s, "GET", "/api/symbols/validate?symbol=EURUSD", nil); rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
}

func TestForexTokenMaskedAndClearReloads(t *testing.T) {
	s := setupTest(t)
	path := filepath.Join(s.configDir, "settings.yaml")
	settings := &appconfig.SettingsYAML{}
	settings.Forex.Provider = "auto"
	settings.Forex.OANDA.Token = "secret"
	settings.Forex.OANDA.Environment = "practice"
	if err := appconfig.SaveSettings(path, settings); err != nil {
		t.Fatal(err)
	}
	s.WithSettingsFile(path)
	reloads := 0
	s.WithForexRuntime(func() { reloads++ }, nil)
	rr := do(t, s, "GET", "/api/settings/config", nil)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["OANDA_TOKEN"] != "__configured__" {
		t.Fatalf("token visible: %q", got["OANDA_TOKEN"])
	}
	if rr := do(t, s, "PUT", "/api/settings/config", map[string]string{"OANDA_TOKEN": ""}); rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads=%d", reloads)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("settings empty")
	}
	updated, err := appconfig.LoadSettings(path)
	if err != nil || updated.Forex.OANDA.Token != "" || updated.Forex.Provider != "auto" {
		t.Fatalf("update: %+v %v", updated, err)
	}
}

func TestEnablingFirstExistingUSDPairAddsDXYOnce(t *testing.T) {
	s := setupTest(t)
	content := strings.Replace(testWatchlist, "timeframes:", "  forex:\n    - symbol: EURUSD\n      enabled: false\ntimeframes:", 1)
	if err := os.WriteFile(filepath.Join(s.configDir, "watchlist.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if rr := do(t, s, "PUT", "/api/symbols/EURUSD", map[string]bool{"enabled": true}); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	if rr := do(t, s, "DELETE", "/api/symbols/DX-Y.NYB", nil); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	if rr := do(t, s, "PUT", "/api/symbols/EURUSD", map[string]bool{"enabled": false}); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	if rr := do(t, s, "PUT", "/api/symbols/EURUSD", map[string]bool{"enabled": true}); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	wl, _ := s.readWatchlist()
	if len(wl.Symbols.Indices) != 0 {
		t.Fatalf("DXY re-added: %+v", wl.Symbols.Indices)
	}
}
