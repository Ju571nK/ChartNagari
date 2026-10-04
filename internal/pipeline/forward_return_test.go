package pipeline

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── mock forward return DB ──────────────────────────────────────────────────

type mockFRDB struct {
	signals []storage.SignalForForwardReturn
	updated map[int64][4]float64
}

func TestForwardReturns_ForexSourceTransitionAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forward-returns.db")
	db, err := storage.New(path)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Truncate(time.Second).Add(-12 * 24 * time.Hour)
	yahoo := models.Signal{AssetClass: models.AssetForex, DataProvider: "yahoo", ProviderSymbol: "GC=F", SourceIdentity: "yahoo:public:GC=F", DataProxy: true}
	oanda := models.Signal{AssetClass: models.AssetForex, DataProvider: "oanda", ProviderSymbol: "XAU_USD", SourceIdentity: "oanda:practice:XAU_USD"}
	if _, err := db.EnsureForexSources("yahoo", map[string]string{"XAUUSD": yahoo.SourceIdentity}); err != nil {
		t.Fatal(err)
	}
	old := yahoo
	old.Symbol, old.Timeframe, old.Rule, old.CreatedAt, old.EntryPrice = "XAUUSD", "1H", "old", created, 2500
	oldID, err := db.SaveSignal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateForwardReturns(oldID, 3, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOHLCV(models.OHLCV{Symbol: "XAUUSD", Timeframe: "1D", OpenTime: created, Close: 2490}, "yahoo_fx"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnsureForexSources("oanda", map[string]string{"XAUUSD": oanda.SourceIdentity}); err != nil {
		t.Fatal(err)
	}
	for _, bar := range []models.OHLCV{
		{Symbol: "XAUUSD", Timeframe: "1D", OpenTime: created, Close: 3000},
		{Symbol: "XAUUSD", Timeframe: "1D", OpenTime: created.Add(10 * 24 * time.Hour), Close: 3300},
	} {
		if err := db.SaveOHLCV(bar, "oanda"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	needs, err := db.GetSignalsNeedingForwardReturn(5)
	if err != nil || len(needs) != 1 || needs[0].EntryPrice != 2500 || needs[0].SourceIdentity != yahoo.SourceIdentity || needs[0].FR5d != 3 {
		t.Fatalf("reload lost original source, entry, or existing return: %+v, %v", needs, err)
	}
	p := New(DefaultConfig(), db, nil, nil, nil, nil, nil, zerolog.Nop())
	p.SetForwardReturnStore(db, db)
	p.SetForexSymbols([]string{"XAUUSD"}, models.VolumeTick, false)
	p.SetForexSources(map[string]models.Signal{"XAUUSD": oanda})
	p.SetForexSymbolReady(func(string) bool { return true })
	p.RunOnce(context.Background())
	assertReturns := func(id int64, want5, want10 float64) {
		t.Helper()
		rows, err := db.GetSignalsNeedingForwardReturn(5)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ID == id {
				if math.Abs(row.FR5d-want5) > 1e-9 || math.Abs(row.FR10d-want10) > 1e-9 {
					t.Fatalf("signal %d returns: got %.3f/%.3f, want %.3f/%.3f", id, row.FR5d, row.FR10d, want5, want10)
				}
				return
			}
		}
		t.Fatalf("signal %d missing", id)
	}
	assertReturns(oldID, 3, 0) // OANDA prices must not value a Yahoo proxy signal.

	current := oanda
	current.Symbol, current.Timeframe, current.Rule, current.CreatedAt, current.EntryPrice = "XAUUSD", "1H", "current", created, 3000
	currentID, err := db.SaveSignal(current)
	if err != nil {
		t.Fatal(err)
	}
	legacy := models.Signal{Symbol: "XAUUSD", Timeframe: "1H", Rule: "legacy", CreatedAt: created, EntryPrice: 2500}
	legacyID, err := db.SaveSignal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	p.SetForexSymbolReady(nil) // Unknown health cannot verify current bars.
	p.RunOnce(context.Background())
	assertReturns(currentID, 0, 0)
	p.SetForexSymbolReady(func(string) bool { return false })
	p.RunOnce(context.Background())
	assertReturns(currentID, 0, 0)
	// A healthy provider cannot use a mislabeled bar in the current series.
	if err := db.SaveOHLCV(models.OHLCV{Symbol: "XAUUSD", Timeframe: "1D", OpenTime: created.Add(10 * 24 * time.Hour), Close: 9900}, "yahoo_fx"); err != nil {
		t.Fatal(err)
	}
	p.SetForexSymbolReady(func(string) bool { return true })
	p.RunOnce(context.Background())
	assertReturns(currentID, 0, 0)
	if err := db.SaveOHLCV(models.OHLCV{Symbol: "XAUUSD", Timeframe: "1D", OpenTime: created.Add(10 * 24 * time.Hour), Close: 3300}, "oanda"); err != nil {
		t.Fatal(err)
	}
	p.RunOnce(context.Background())
	assertReturns(currentID, 0, 10)
	assertReturns(legacyID, 0, 0)
	assertReturns(oldID, 3, 0)

	if _, err := db.EnsureForexSources("yahoo", map[string]string{"XAUUSD": yahoo.SourceIdentity}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOHLCV(models.OHLCV{Symbol: "XAUUSD", Timeframe: "1D", OpenTime: created.Add(10 * 24 * time.Hour), Close: 2750}, "yahoo_fx"); err != nil {
		t.Fatal(err)
	}
	p.SetForexSources(map[string]models.Signal{"XAUUSD": yahoo})
	p.RunOnce(context.Background())
	assertReturns(oldID, 3, 10)
	assertReturns(currentID, 0, 10) // Historical values survive later switches.
}

func TestForwardReturns_LegacyStockStillUsesHistoricalClose(t *testing.T) {
	db, err := storage.New(filepath.Join(t.TempDir(), "stock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	created := time.Now().UTC().Truncate(time.Second).Add(-12 * 24 * time.Hour)
	id, err := db.SaveSignal(models.Signal{Symbol: "AAPL", Timeframe: "1H", Rule: "old", CreatedAt: created, EntryPrice: 777})
	if err != nil {
		t.Fatal(err)
	}
	for _, bar := range []models.OHLCV{
		{Symbol: "AAPL", Timeframe: "1D", OpenTime: created, Close: 100},
		{Symbol: "AAPL", Timeframe: "1D", OpenTime: created.Add(10 * 24 * time.Hour), Close: 110},
	} {
		if err := db.SaveOHLCV(bar, "yahoo"); err != nil {
			t.Fatal(err)
		}
	}
	needs, err := db.GetSignalsNeedingForwardReturn(5)
	if err != nil || len(needs) != 1 || needs[0].EntryPrice != 100 {
		t.Fatalf("legacy stock entry: %+v, %v", needs, err)
	}
	UpdateForwardReturns(db, db, zerolog.Nop())
	rows, err := db.GetSignalsNeedingForwardReturn(5)
	if err != nil || len(rows) != 1 || rows[0].ID != id || math.Abs(rows[0].FR10d-10) > 1e-9 {
		t.Fatalf("legacy stock return: %+v, %v", rows, err)
	}
}

func TestForwardReturns_UnverifiableFXDoesNotStarveStocks(t *testing.T) {
	db, err := storage.New(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	created := time.Now().UTC().Truncate(time.Second).Add(-13 * 24 * time.Hour)
	for i := 0; i < 205; i++ {
		if _, err := db.SaveSignal(models.Signal{Symbol: "XAUUSD", Timeframe: "1H", Rule: "legacy", CreatedAt: created, AssetClass: models.AssetForex, EntryPrice: 2500}); err != nil {
			t.Fatal(err)
		}
	}
	stockTime := created.Add(24 * time.Hour)
	stockID, err := db.SaveSignal(models.Signal{Symbol: "AAPL", Timeframe: "1H", Rule: "stock", CreatedAt: stockTime})
	if err != nil {
		t.Fatal(err)
	}
	for _, bar := range []models.OHLCV{
		{Symbol: "AAPL", Timeframe: "1D", OpenTime: stockTime, Close: 100},
		{Symbol: "AAPL", Timeframe: "1D", OpenTime: stockTime.Add(10 * 24 * time.Hour), Close: 110},
	} {
		if err := db.SaveOHLCV(bar, "yahoo"); err != nil {
			t.Fatal(err)
		}
	}
	p := New(DefaultConfig(), db, nil, nil, nil, nil, nil, zerolog.Nop())
	p.SetForwardReturnStore(db, db)
	p.SetForexSymbols([]string{"XAUUSD"}, models.VolumeTick, false)
	p.SetForexSources(map[string]models.Signal{"XAUUSD": {DataProvider: "oanda", ProviderSymbol: "XAU_USD", SourceIdentity: "oanda:practice:XAU_USD"}})
	p.SetForexSymbolReady(func(string) bool { return true })
	p.RunOnce(context.Background())
	rows, err := db.GetSignalsNeedingForwardReturn(5)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == stockID {
			if math.Abs(row.FR10d-10) > 1e-9 {
				t.Fatalf("stock delayed by unverifiable FX: %+v", row)
			}
			return
		}
	}
	t.Fatal("stock signal not found")
}

func (m *mockFRDB) GetSignalsNeedingForwardReturn(minAgeDays int) ([]storage.SignalForForwardReturn, error) {
	return m.signals, nil
}

func (m *mockFRDB) UpdateForwardReturns(signalID int64, r5, r10, r20, r40 float64) error {
	if m.updated == nil {
		m.updated = make(map[int64][4]float64)
	}
	m.updated[signalID] = [4]float64{r5, r10, r20, r40}
	return nil
}

// ── mock OHLCV reader ───────────────────────────────────────────────────────

type mockFROHLCV struct {
	bars map[string][]models.OHLCV
}

func (m *mockFROHLCV) GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	return m.bars[symbol+"|"+timeframe], nil
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestUpdateForwardReturns_ComputesReturns(t *testing.T) {
	now := time.Now()
	signalTime := now.Add(-45 * 24 * time.Hour) // 45 days ago

	db := &mockFRDB{
		signals: []storage.SignalForForwardReturn{
			{
				ID:         1,
				Symbol:     "AAPL",
				Timeframe:  "1D",
				EntryPrice: 100.0,
				CreatedAt:  signalTime,
			},
		},
	}

	// Create bars at 5, 10, 20, 40 days after signal
	ohlcv := &mockFROHLCV{
		bars: map[string][]models.OHLCV{
			"AAPL|1D": {
				// DESC order: newest first
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(40 * 24 * time.Hour), Close: 120.0}, // +20%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(20 * 24 * time.Hour), Close: 115.0}, // +15%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(10 * 24 * time.Hour), Close: 110.0}, // +10%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(5 * 24 * time.Hour), Close: 105.0},  // +5%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime, Close: 100.0},
			},
		},
	}

	UpdateForwardReturns(db, ohlcv, zerolog.Nop())

	if len(db.updated) != 1 {
		t.Fatalf("expected 1 updated signal, got %d", len(db.updated))
	}

	returns := db.updated[1]
	// r5d = (105 - 100) / 100 * 100 = 5.0%
	if returns[0] < 4.9 || returns[0] > 5.1 {
		t.Errorf("expected r5d ~5.0%%, got %.2f%%", returns[0])
	}
	// r10d = (110 - 100) / 100 * 100 = 10.0%
	if returns[1] < 9.9 || returns[1] > 10.1 {
		t.Errorf("expected r10d ~10.0%%, got %.2f%%", returns[1])
	}
	// r20d = 15%
	if returns[2] < 14.9 || returns[2] > 15.1 {
		t.Errorf("expected r20d ~15.0%%, got %.2f%%", returns[2])
	}
	// r40d = 20%
	if returns[3] < 19.9 || returns[3] > 20.1 {
		t.Errorf("expected r40d ~20.0%%, got %.2f%%", returns[3])
	}
}

func TestUpdateForwardReturns_PartialData(t *testing.T) {
	now := time.Now()
	signalTime := now.Add(-8 * 24 * time.Hour) // 8 days ago — only 5d should be computed

	db := &mockFRDB{
		signals: []storage.SignalForForwardReturn{
			{
				ID:         2,
				Symbol:     "BTCUSDT",
				Timeframe:  "1D",
				EntryPrice: 50000.0,
				CreatedAt:  signalTime,
			},
		},
	}

	ohlcv := &mockFROHLCV{
		bars: map[string][]models.OHLCV{
			"BTCUSDT|1D": {
				{Symbol: "BTCUSDT", Timeframe: "1D", OpenTime: signalTime.Add(5 * 24 * time.Hour), Close: 52000.0},
				{Symbol: "BTCUSDT", Timeframe: "1D", OpenTime: signalTime, Close: 50000.0},
			},
		},
	}

	UpdateForwardReturns(db, ohlcv, zerolog.Nop())

	if len(db.updated) != 1 {
		t.Fatalf("expected 1 updated signal, got %d", len(db.updated))
	}

	returns := db.updated[2]
	// r5d = (52000 - 50000) / 50000 * 100 = 4.0%
	if returns[0] < 3.9 || returns[0] > 4.1 {
		t.Errorf("expected r5d ~4.0%%, got %.2f%%", returns[0])
	}
	// r10d, r20d, r40d should remain 0 (not enough time elapsed)
	if returns[1] != 0 || returns[2] != 0 || returns[3] != 0 {
		t.Errorf("expected r10d/r20d/r40d = 0, got %.2f/%.2f/%.2f", returns[1], returns[2], returns[3])
	}
}

func TestUpdateForwardReturns_NoEntryPrice(t *testing.T) {
	db := &mockFRDB{
		signals: []storage.SignalForForwardReturn{
			{
				ID:         3,
				Symbol:     "TSLA",
				Timeframe:  "1D",
				EntryPrice: 0, // no entry price
				CreatedAt:  time.Now().Add(-10 * 24 * time.Hour),
			},
		},
	}

	ohlcv := &mockFROHLCV{bars: map[string][]models.OHLCV{}}

	UpdateForwardReturns(db, ohlcv, zerolog.Nop())

	if len(db.updated) != 0 {
		t.Errorf("expected no updates for signal without entry price, got %d", len(db.updated))
	}
}

func TestFindCloseNearDate(t *testing.T) {
	target := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	bars := []models.OHLCV{
		{OpenTime: time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC), Close: 102.0},
		{OpenTime: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC), Close: 101.0}, // closest: 1 day off
		{OpenTime: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), Close: 99.0},
	}

	got := findCloseNearDate(bars, target)
	if got != 101.0 {
		t.Errorf("expected 101.0 (closest bar), got %.1f", got)
	}

	// Test out of tolerance
	farTarget := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	got = findCloseNearDate(bars, farTarget)
	if got != 0 {
		t.Errorf("expected 0 (no bar in tolerance), got %.1f", got)
	}
}
