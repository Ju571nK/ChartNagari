package storage

import (
	"github.com/Ju571nK/Chatter/internal/paper"
	"github.com/Ju571nK/Chatter/pkg/models"
	"github.com/rs/zerolog"
	"path/filepath"
	"testing"
	"time"
)

func TestForexProviderTransitionPurgesOnlyFX(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if changed, err := db.EnsureForexProvider("yahoo", []string{"EURUSD"}); err != nil || changed {
		t.Fatalf("first provider %v %v", changed, err)
	}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ sym, source string }{{"EURUSD", "yahoo_fx"}, {"AAPL", "yahoo"}} {
		if err := db.SaveOHLCV(models.OHLCV{Symbol: tc.sym, Timeframe: "1D", OpenTime: at, Close: 1}, tc.source); err != nil {
			t.Fatal(err)
		}
	}
	prior, _ := db.GetOHLCV("EURUSD", "1D", 1)
	if len(prior) != 1 || prior[0].VolumeQuality != models.VolumeNone || prior[0].Source != "yahoo_fx" {
		t.Fatalf("source quality: %+v", prior)
	}
	if changed, err := db.EnsureForexProvider("oanda", []string{"EURUSD"}); err != nil || !changed {
		t.Fatalf("transition %v %v", changed, err)
	}
	fx, _ := db.GetOHLCV("EURUSD", "1D", 10)
	stock, _ := db.GetOHLCV("AAPL", "1D", 10)
	if len(fx) != 0 || len(stock) != 1 {
		t.Fatalf("purge fx=%d stock=%d", len(fx), len(stock))
	}
}

func TestProxyPositionSuspendsAcrossSourceSwitchAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fx.db")
	db, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	yahoo := "yahoo:public:GC=F"
	oanda := "oanda:practice:XAU_USD"
	if _, err := db.EnsureForexSources("yahoo", map[string]string{"XAUUSD": yahoo}); err != nil {
		t.Fatal(err)
	}
	entry := time.Now().Add(-2 * time.Hour)
	trader := paper.New(db, zerolog.Nop())
	trader.SetForexSources(map[string]string{"XAUUSD": yahoo})
	trader.OnSignals([]models.Signal{{Symbol: "XAUUSD", Timeframe: "1H", Rule: "test", Direction: "LONG", EntryPrice: 2000, TP: 2010, SL: 1990, CreatedAt: entry, AssetClass: models.AssetForex, DataProvider: "yahoo", ProviderSymbol: "GC=F", SourceIdentity: yahoo, DataProxy: true}})
	open, err := db.GetOpenPositions("XAUUSD")
	if err != nil || len(open) != 1 || open[0].Suspended || !open[0].DataProxy {
		t.Fatalf("before switch: %+v %v", open, err)
	}
	if changed, err := db.EnsureForexSources("oanda", map[string]string{"XAUUSD": oanda}); err != nil || !changed {
		t.Fatalf("transition: %v %v", changed, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	trader = paper.New(db, zerolog.Nop())
	trader.SetForexSources(map[string]string{"XAUUSD": oanda})
	trader.CheckPositions("XAUUSD", map[string][]models.OHLCV{"1H": {{Symbol: "XAUUSD", Timeframe: "1H", OpenTime: entry.Add(time.Hour), High: 2020, Low: 1980, Source: "oanda"}}})
	open, err = db.GetOpenPositions("XAUUSD")
	if err != nil || len(open) != 1 || !open[0].Suspended || open[0].SuspensionReason != "source_mismatch" || open[0].SourceIdentity != yahoo {
		t.Fatalf("after restart: %+v %v", open, err)
	}
	legacy, err := db.SavePaperPosition(paper.PaperPosition{Symbol: "XAUUSD", Timeframe: "1H", Rule: "legacy", Direction: "SHORT", EntryPrice: 2000, TP: 1990, SL: 2010, EntryTime: entry, AssetClass: models.AssetForex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnsureForexSources("oanda", map[string]string{"XAUUSD": oanda}); err != nil {
		t.Fatal(err)
	}
	open, _ = db.GetOpenPositions("XAUUSD")
	for _, pos := range open {
		if pos.ID == legacy && (!pos.Suspended || pos.SuspensionReason != "unknown_origin") {
			t.Fatalf("legacy unsafe: %+v", pos)
		}
	}
}

func TestEnvironmentAndMappingChangesInvalidateOnlyAffectedBars(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	initial := map[string]string{"EURUSD": "oanda:practice:EUR_USD", "GBPUSD": "oanda:practice:GBP_USD"}
	if _, err := db.EnsureForexSources("oanda", initial); err != nil {
		t.Fatal(err)
	}
	for symbol := range initial {
		if err := db.SaveOHLCV(models.OHLCV{Symbol: symbol, Timeframe: "1H", OpenTime: time.Now().Add(-time.Hour), Close: 1}, "oanda"); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := db.EnsureForexSources("oanda", map[string]string{"EURUSD": "oanda:practice:EUR_USD_ALT", "GBPUSD": "oanda:practice:GBP_USD"})
	if err != nil || !changed {
		t.Fatalf("mapping: %v %v", changed, err)
	}
	eur, _ := db.GetOHLCV("EURUSD", "1H", 1)
	gbp, _ := db.GetOHLCV("GBPUSD", "1H", 1)
	if len(eur) != 0 || len(gbp) != 1 {
		t.Fatalf("mapping purge EUR=%d GBP=%d", len(eur), len(gbp))
	}
	changed, err = db.EnsureForexSources("oanda", map[string]string{"EURUSD": "oanda:live:EUR_USD_ALT", "GBPUSD": "oanda:live:GBP_USD"})
	if err != nil || !changed {
		t.Fatalf("environment: %v %v", changed, err)
	}
	gbp, _ = db.GetOHLCV("GBPUSD", "1H", 1)
	if len(gbp) != 0 {
		t.Fatal("practice bars survived live switch")
	}
}

func TestForexSignalMetadataPersists(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sig := models.Signal{Symbol: "EURUSD", Timeframe: "1H", Rule: "test", Direction: "LONG", Score: 1, CreatedAt: time.Now(), AssetClass: models.AssetForex, VolumeQuality: models.VolumeNone, VolumeUnconfirmed: true, MessageKey: "signal.test", MessageParams: map[string]string{"session": "London"}, EntryPrice: 1.1, TP: 1.2, SL: 1.0, TPPips: 100, SLPips: 100, SpreadPipsSet: true}
	if _, err := db.SaveSignal(sig); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSignals("EURUSD", 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("read: %+v %v", got, err)
	}
	if got[0].AssetClass != models.AssetForex || got[0].VolumeQuality != models.VolumeNone || !got[0].VolumeUnconfirmed || got[0].MessageParams["session"] != "London" || !got[0].SpreadPipsSet || got[0].TP != 1.2 {
		t.Fatalf("metadata lost: %+v", got[0])
	}
}

func TestPaperPipsPersist(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "paper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := db.SavePaperPosition(paper.PaperPosition{Symbol: "EURUSD", Timeframe: "1H", Rule: "test", Direction: "LONG", EntryPrice: 1.1, TP: 1.2, SL: 1.0, EntryTime: time.Now(), AssetClass: models.AssetForex, PipSize: .0001, SpreadPips: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	open, err := db.GetOpenPositions("EURUSD")
	if err != nil || len(open) != 1 || open[0].SpreadPips != 1.5 || open[0].PipSize != .0001 {
		t.Fatalf("open: %+v %v", open, err)
	}
	if open[0].PriceBasis != "mid" || open[0].EntryExecutionPrice <= open[0].EntryPrice {
		t.Fatalf("execution entry: %+v", open[0])
	}
	if err := db.ClosePaperPositionWithPips(id, 1.12, "CLOSED_TP", 1.5, 198.5); err != nil {
		t.Fatal(err)
	}
	closed, err := db.GetClosedPositions(1)
	if err != nil || len(closed) != 1 || closed[0].PnLPips != 198.5 || closed[0].AssetClass != models.AssetForex {
		t.Fatalf("closed: %+v %v", closed, err)
	}
	if closed[0].ExitExecutionPrice >= closed[0].ExitPrice {
		t.Fatalf("execution exit: %+v", closed[0])
	}
}
