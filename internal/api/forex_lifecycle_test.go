package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/collector"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/internal/paper"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

func TestSettingsSwitchDrainsPurgeBackfillAndRestart(t *testing.T) {
	lastClosedFXHour := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	for !market.IsForexOpen(lastClosedFXHour.Add(30 * time.Minute)) {
		lastClosedFXHour = lastClosedFXHour.Add(-time.Hour)
	}
	s := setupTest(t)
	settingsPath := filepath.Join(s.configDir, "settings.yaml")
	settings, err := appconfig.LoadSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	settings.Forex.Provider = "yahoo"
	if err := appconfig.SaveSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}
	s.WithSettingsFile(settingsPath)
	wl := appconfig.WatchlistConfig{}
	wl.Symbols.Forex = []appconfig.SymbolEntry{{Symbol: "XAUUSD", Enabled: true}}
	wl.Timeframes = []string{"1H"}
	data, _ := yaml.Marshal(wl)
	if err := os.WriteFile(filepath.Join(s.configDir, "watchlist.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fx.db")
	db, err := storage.New(path)
	if err != nil {
		t.Fatal(err)
	}
	yahooID := "yahoo:public:GC=F"
	if _, err := db.EnsureForexSources("yahoo", map[string]string{"XAUUSD": yahooID}); err != nil {
		t.Fatal(err)
	}
	old := models.OHLCV{Symbol: "XAUUSD", Timeframe: "1H", OpenTime: time.Now().Add(-2 * time.Hour), Close: 2000}
	if err := db.SaveOHLCV(old, "yahoo_fx"); err != nil {
		t.Fatal(err)
	}
	tr := paper.New(db, zerolog.Nop())
	tr.SetForexSources(map[string]string{"XAUUSD": yahooID})
	tr.OnSignals([]models.Signal{{Symbol: "XAUUSD", Timeframe: "1H", Rule: "test", Direction: "LONG", EntryPrice: 2000, TP: 2010, SL: 1990, CreatedAt: time.Now().Add(-3 * time.Hour), AssetClass: models.AssetForex, DataProvider: "yahoo", ProviderSymbol: "GC=F", SourceIdentity: yahooID, DataProxy: true}})
	started := make(chan struct{})
	drained := make(chan struct{})
	yahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-r.Context().Done()
		select {
		case <-drained:
		default:
			close(drained)
		}
	}))
	defer yahoo.Close()
	oanda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"candles": []any{map[string]any{"complete": true, "time": lastClosedFXHour.Format(time.RFC3339Nano), "volume": 8, "mid": map[string]string{"o": "2000", "h": "2020", "l": "1980", "c": "2005"}}}})
	}))
	defer oanda.Close()
	runtime := collector.NewRuntime(wl)
	s.WithForexRuntime(runtime.Reload, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.Run(ctx, func(workerCtx context.Context, current appconfig.WatchlistConfig) {
			loaded, e := appconfig.LoadSettings(settingsPath)
			if e != nil {
				return
			}
			provider := (appconfig.ForexConfig{Provider: loaded.Forex.Provider, OANDA: appconfig.OANDAConfig{Token: loaded.Forex.OANDA.Token, Environment: loaded.Forex.OANDA.Environment}}).EffectiveProvider()
			source := collector.SourceForForex(current.Symbols.Forex[0], provider, loaded.Forex.OANDA.Environment, loaded.Forex.OANDA.Token)
			if _, e = db.EnsureForexSources(provider, map[string]string{"XAUUSD": source.Identity}); e != nil {
				return
			}
			if provider == "yahoo" {
				c := collector.NewYahooFXCollector(db, current.Symbols.Forex, current.Timeframes, time.Hour)
				c.SetBaseURL(yahoo.URL)
				c.Start(workerCtx)
			} else {
				c := collector.NewOANDACollector(db, current.Symbols.Forex, current.Timeframes, time.Hour, loaded.Forex.OANDA.Token, loaded.Forex.OANDA.Environment)
				c.SetBaseURL(oanda.URL)
				c.Start(workerCtx)
			}
		})
	}()
	defer func() { cancel(); <-done; db.Close() }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("Yahoo request did not start")
	}
	rr := do(t, s, "PUT", "/api/settings/config", map[string]string{"FOREX_PROVIDER": "oanda", "OANDA_TOKEN": "token", "OANDA_ENVIRONMENT": "practice"})
	if rr.Code != 200 {
		t.Fatalf("settings save: %d %s", rr.Code, rr.Body.String())
	}
	select {
	case <-drained:
	case <-time.After(3 * time.Second):
		t.Fatal("old Yahoo request was not canceled")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		bars, e := db.GetOHLCV("XAUUSD", "1H", 10)
		if e == nil && len(bars) == 1 && bars[0].Source == "oanda" {
			open, e := db.GetOpenPositions("XAUUSD")
			if e != nil || len(open) != 1 || !open[0].Suspended {
				t.Fatalf("position after switch: %+v %v", open, e)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("OANDA backfill failed: %+v %v", bars, e)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	open, err := reopened.GetOpenPositions("XAUUSD")
	if err != nil || len(open) != 1 || !open[0].Suspended || open[0].SourceIdentity != yahooID || !open[0].DataProxy {
		t.Fatalf("restart position: %+v %v", open, err)
	}
}
