package paper

import (
	"math"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
	"github.com/rs/zerolog"
)

// ── mock store ────────────────────────────────────────────────────────────────

type mockStore struct {
	positions []PaperPosition
	nextID    int64
	closed    []PaperPosition
}

func (m *mockStore) SavePaperPosition(pos PaperPosition) (int64, error) {
	m.nextID++
	pos.ID = m.nextID
	pos.Status = "OPEN"
	m.positions = append(m.positions, pos)
	return m.nextID, nil
}

func (m *mockStore) GetOpenPositions(symbol string) ([]PaperPosition, error) {
	var out []PaperPosition
	for _, p := range m.positions {
		if p.Symbol == symbol && p.Status == "OPEN" {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *mockStore) GetAllOpenPositions() ([]PaperPosition, error) {
	var out []PaperPosition
	for _, p := range m.positions {
		if p.Status == "OPEN" {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *mockStore) ClosePaperPosition(id int64, exitPrice float64, status string, pnlPct float64) error {
	for i := range m.positions {
		if m.positions[i].ID == id {
			m.positions[i].Status = status
			m.positions[i].ExitPrice = exitPrice
			m.positions[i].PnLPct = pnlPct
			m.closed = append(m.closed, m.positions[i])
		}
	}
	return nil
}

func (m *mockStore) ClosePaperPositionWithPips(id int64, exitPrice float64, status string, pnlPct, pnlPips float64) error {
	if err := m.ClosePaperPosition(id, exitPrice, status, pnlPct); err != nil {
		return err
	}
	for i := range m.positions {
		if m.positions[i].ID == id {
			m.positions[i].PnLPips = pnlPips
		}
	}
	for i := range m.closed {
		if m.closed[i].ID == id {
			m.closed[i].PnLPips = pnlPips
		}
	}
	return nil
}

func (m *mockStore) GetClosedPositions(limit int) ([]PaperPosition, error) {
	return m.closed, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func nopLog() zerolog.Logger { return zerolog.Nop() }

func makeSig(symbol, dir, rule string, entry, tp, sl float64) models.Signal {
	return models.Signal{
		Symbol:     symbol,
		Timeframe:  "1H",
		Rule:       rule,
		Direction:  dir,
		Score:      10.0,
		EntryPrice: entry,
		TP:         tp,
		SL:         sl,
		CreatedAt:  time.Now(),
	}
}

func makeBar(symbol, tf string, openTime time.Time, high, low, close_ float64) models.OHLCV {
	return models.OHLCV{
		Symbol:    symbol,
		Timeframe: tf,
		OpenTime:  openTime,
		Open:      close_,
		High:      high,
		Low:       low,
		Close:     close_,
		Volume:    1000,
	}
}

func TestForexPaperSpreadPipsLongShort(t *testing.T) {
	for _, tc := range []struct {
		direction               string
		tp, sl, high, low, exit float64
	}{
		{"LONG", 1.1020, 1.0980, 1.1021, 1.0990, 1.1020},
		{"SHORT", 1.0980, 1.1020, 1.1010, 1.0979, 1.0980},
	} {
		t.Run(tc.direction, func(t *testing.T) {
			store := &mockStore{}
			tr := New(store, nopLog())
			tr.SetForexSources(map[string]string{"EURUSD": "yahoo:public:EURUSD=X"})
			sig := makeSig("EURUSD", tc.direction, "test", 1.1000, tc.tp, tc.sl)
			sig.AssetClass = models.AssetForex
			sig.SourceIdentity = "yahoo:public:EURUSD=X"
			sig.DataProvider = "yahoo"
			tr.OnSignals([]models.Signal{sig})
			bars := map[string][]models.OHLCV{"1H": {makeBar("EURUSD", "1H", sig.CreatedAt.Add(time.Hour), tc.high, tc.low, tc.exit)}}
			bars["1H"][0].Source = "yahoo_fx"
			tr.CheckPositions("EURUSD", bars)
			if len(store.closed) != 1 {
				t.Fatalf("expected close, got %d", len(store.closed))
			}
			got := store.closed[0]
			if math.Abs(got.PnLPips-19) > 1e-8 || got.SpreadPips != 1 {
				t.Fatalf("expected 19 net pips, got %+v", got)
			}
			if got.PnLPct <= 0 {
				t.Fatalf("expected positive net percent, got %+v", got)
			}
			summary := Summary(store.closed, 0)
			if math.Abs(summary.NetPips-19) > 1e-8 || math.Abs(summary.AvgPips-19) > 1e-8 {
				t.Fatalf("wrong stats: %+v", summary)
			}
		})
	}
}

func TestForexPaperExplicitZeroSpread(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())
	tr.SetForexSources(map[string]string{"EURUSD": "yahoo:public:EURUSD=X"})
	sig := makeSig("EURUSD", "LONG", "test", 1.1000, 1.1020, 1.0980)
	sig.AssetClass = models.AssetForex
	sig.SourceIdentity = "yahoo:public:EURUSD=X"
	sig.DataProvider = "yahoo"
	sig.SpreadPipsSet = true
	sig.SpreadPips = 0
	tr.OnSignals([]models.Signal{sig})
	bar := makeBar("EURUSD", "1H", sig.CreatedAt.Add(time.Hour), 1.1021, 1.0990, 1.1020)
	bar.Source = "yahoo_fx"
	tr.CheckPositions("EURUSD", map[string][]models.OHLCV{"1H": {bar}})
	if len(store.closed) != 1 || math.Abs(store.closed[0].PnLPips-20) > 1e-8 {
		t.Fatalf("explicit zero spread lost: %+v", store.closed)
	}
}

func TestPaperChecksFirstHistoricalExit(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())
	tr.SetForexSources(map[string]string{"EURUSD": "yahoo:public:EURUSD=X"})
	sig := makeSig("EURUSD", "LONG", "test", 1.1000, 1.1020, 1.0980)
	sig.AssetClass = models.AssetForex
	sig.SourceIdentity = "yahoo:public:EURUSD=X"
	sig.DataProvider = "yahoo"
	tr.OnSignals([]models.Signal{sig})
	newer := makeBar("EURUSD", "1H", sig.CreatedAt.Add(2*time.Hour), 1.1030, 1.0990, 1.1020)
	older := makeBar("EURUSD", "1H", sig.CreatedAt.Add(time.Hour), 1.1010, 1.0970, 1.0980)
	newer.Source, older.Source = "yahoo_fx", "yahoo_fx"
	tr.CheckPositions("EURUSD", map[string][]models.OHLCV{"1H": {newer, older}})
	if len(store.closed) != 1 || store.closed[0].Status != "CLOSED_SL" || store.closed[0].PnLPips >= 0 {
		t.Fatalf("first exit should be stop, got %+v", store.closed)
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// Test 1: OnSignals opens a new position for a LONG signal.
func TestOnSignals_OpensLongPosition(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())

	tr.OnSignals([]models.Signal{makeSig("BTCUSDT", "LONG", "rsi", 65000, 67000, 64000)})

	open, _ := store.GetOpenPositions("BTCUSDT")
	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if open[0].Direction != "LONG" || open[0].EntryPrice != 65000 {
		t.Errorf("position fields incorrect: %+v", open[0])
	}
}

// Test 2: OnSignals does not open duplicate positions for the same symbol.
func TestOnSignals_NoDuplicateForSymbol(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())

	tr.OnSignals([]models.Signal{makeSig("BTCUSDT", "LONG", "rsi", 65000, 67000, 64000)})
	tr.OnSignals([]models.Signal{makeSig("BTCUSDT", "SHORT", "ema", 65000, 63000, 66000)})

	open, _ := store.GetOpenPositions("BTCUSDT")
	if len(open) != 1 {
		t.Fatalf("expected 1 position (no duplicate), got %d", len(open))
	}
}

// Test 3: OnSignals ignores signals with EntryPrice = 0.
func TestOnSignals_IgnoresZeroEntry(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())

	sig := makeSig("BTCUSDT", "LONG", "rsi", 0, 0, 0) // EntryPrice=0
	tr.OnSignals([]models.Signal{sig})

	open, _ := store.GetOpenPositions("BTCUSDT")
	if len(open) != 0 {
		t.Fatalf("expected 0 positions for zero-entry signal, got %d", len(open))
	}
}

// Test 4: CheckPositions closes LONG position on TP hit.
func TestCheckPositions_LongTPHit(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())

	tr.OnSignals([]models.Signal{makeSig("BTCUSDT", "LONG", "rsi", 65000, 67000, 64000)})

	entryTime := store.positions[0].EntryTime
	future := entryTime.Add(time.Hour)

	allBars := map[string][]models.OHLCV{
		"1H": {makeBar("BTCUSDT", "1H", future, 68000, 65000, 67500)}, // High > TP
	}
	tr.CheckPositions("BTCUSDT", allBars)

	open, _ := store.GetOpenPositions("BTCUSDT")
	if len(open) != 0 {
		t.Fatalf("expected 0 open positions after TP hit, got %d", len(open))
	}
	if len(store.closed) != 1 || store.closed[0].Status != "CLOSED_TP" {
		t.Errorf("expected CLOSED_TP, got: %+v", store.closed)
	}
	if store.closed[0].PnLPct <= 0 {
		t.Errorf("expected positive PnL for TP hit, got %f", store.closed[0].PnLPct)
	}
}

// Test 5: CheckPositions closes SHORT position on SL hit.
func TestCheckPositions_ShortSLHit(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())

	tr.OnSignals([]models.Signal{makeSig("BTCUSDT", "SHORT", "ema", 65000, 63000, 66500)})

	entryTime := store.positions[0].EntryTime
	future := entryTime.Add(time.Hour)

	allBars := map[string][]models.OHLCV{
		"1H": {makeBar("BTCUSDT", "1H", future, 67000, 65000, 66000)}, // High > SL=66500
	}
	tr.CheckPositions("BTCUSDT", allBars)

	if len(store.closed) != 1 || store.closed[0].Status != "CLOSED_SL" {
		t.Errorf("expected CLOSED_SL, got: %+v", store.closed)
	}
	if store.closed[0].PnLPct >= 0 {
		t.Errorf("expected negative PnL for SL hit, got %f", store.closed[0].PnLPct)
	}
}

// Test 6: CheckPositions ignores bars older than entry time.
func TestCheckPositions_IgnoresOldBars(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())

	tr.OnSignals([]models.Signal{makeSig("BTCUSDT", "LONG", "rsi", 65000, 67000, 64000)})

	entryTime := store.positions[0].EntryTime
	past := entryTime.Add(-time.Hour) // bar BEFORE entry

	allBars := map[string][]models.OHLCV{
		"1H": {makeBar("BTCUSDT", "1H", past, 70000, 60000, 67000)},
	}
	tr.CheckPositions("BTCUSDT", allBars)

	open, _ := store.GetOpenPositions("BTCUSDT")
	if len(open) != 1 {
		t.Fatalf("position should remain open for pre-entry bars, got %d open", len(open))
	}
}

// Test 7: Summary calculates win rate and PnL correctly.
func TestSummary_KnownPositions(t *testing.T) {
	closed := []PaperPosition{
		{PnLPct: 4.0},
		{PnLPct: 2.0},
		{PnLPct: -1.5},
		{PnLPct: -2.0},
	}
	s := Summary(closed, 1)
	if s.WinRate != 0.5 {
		t.Errorf("WinRate: want 0.5, got %f", s.WinRate)
	}
	expected := 4.0 + 2.0 - 1.5 - 2.0
	if s.TotalPnLPct != expected {
		t.Errorf("TotalPnLPct: want %f, got %f", expected, s.TotalPnLPct)
	}
}

// Test 8: checkLevel LONG TP/SL detection.
func TestCheckLevel_Long(t *testing.T) {
	pos := PaperPosition{Direction: "LONG", EntryPrice: 100, TP: 105, SL: 98}

	bar := models.OHLCV{High: 106, Low: 99} // TP hit
	status, exit, hit := checkLevel(pos, bar)
	if !hit || status != "CLOSED_TP" || exit != 105 {
		t.Errorf("LONG TP: want CLOSED_TP 105, got %s %f %v", status, exit, hit)
	}

	bar2 := models.OHLCV{High: 103, Low: 97} // SL hit
	status2, exit2, hit2 := checkLevel(pos, bar2)
	if !hit2 || status2 != "CLOSED_SL" || exit2 != 98 {
		t.Errorf("LONG SL: want CLOSED_SL 98, got %s %f %v", status2, exit2, hit2)
	}

	bar3 := models.OHLCV{High: 103, Low: 99} // neither
	_, _, hit3 := checkLevel(pos, bar3)
	if hit3 {
		t.Error("expected no hit when price between SL and TP")
	}
}

// Test 9: checkLevel SHORT TP/SL detection.
func TestCheckLevel_Short(t *testing.T) {
	pos := PaperPosition{Direction: "SHORT", EntryPrice: 100, TP: 95, SL: 103}

	bar := models.OHLCV{High: 101, Low: 94} // TP hit
	status, exit, hit := checkLevel(pos, bar)
	if !hit || status != "CLOSED_TP" || exit != 95 {
		t.Errorf("SHORT TP: want CLOSED_TP 95, got %s %f %v", status, exit, hit)
	}

	bar2 := models.OHLCV{High: 104, Low: 97} // SL hit
	status2, exit2, hit2 := checkLevel(pos, bar2)
	if !hit2 || status2 != "CLOSED_SL" || exit2 != 103 {
		t.Errorf("SHORT SL: want CLOSED_SL 103, got %s %f %v", status2, exit2, hit2)
	}
}

// Test 10: OnSignals opens positions for multiple different symbols simultaneously.
func TestOnSignals_MultipleSymbols(t *testing.T) {
	store := &mockStore{}
	tr := New(store, nopLog())

	tr.OnSignals([]models.Signal{
		makeSig("BTCUSDT", "LONG", "rsi", 65000, 67000, 64000),
		makeSig("ETHUSDT", "SHORT", "ema", 3000, 2900, 3100),
	})

	btcOpen, _ := store.GetOpenPositions("BTCUSDT")
	ethOpen, _ := store.GetOpenPositions("ETHUSDT")
	if len(btcOpen) != 1 || len(ethOpen) != 1 {
		t.Errorf("expected 1 open position each; BTC=%d ETH=%d", len(btcOpen), len(ethOpen))
	}
}
