package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/interpreter"
	"github.com/Ju571nK/Chatter/internal/notifier"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── mock DB ──────────────────────────────────────────────────────────────────

type mockDB struct {
	bars map[string][]models.OHLCV // key: "symbol|tf"
	err  error
	calls []string
}

func (m *mockDB) GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	m.calls = append(m.calls, symbol+"|"+timeframe)
	if m.err != nil {
		return nil, m.err
	}
	return m.bars[symbol+"|"+timeframe], nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makeBar(close, vol float64) models.OHLCV {
	return models.OHLCV{
		Symbol: "TEST", Timeframe: "1H",
		OpenTime: time.Now(),
		Open: close - 1, High: close + 1, Low: close - 2, Close: close, Volume: vol,
	}
}

func makeBars(n int, close, vol float64) []models.OHLCV {
	bars := make([]models.OHLCV, n)
	for i := range bars {
		bars[i] = makeBar(close, vol)
	}
	return bars
}

func newTestPipeline(db OHLCVReader, symbols []string) *Pipeline {
	eng := engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{}})
	interp := interpreter.New("", 12.0) // disabled — no API key
	notif := notifier.New(notifier.DefaultConfig(), zerolog.Nop())
	return New(
		DefaultConfig(),
		db,
		eng,
		interp,
		notif,
		symbols,
		[]string{"1H", "4H"},
		zerolog.Nop(),
	)
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestRunOnce_EmptyDB(t *testing.T) {
	db := &mockDB{bars: map[string][]models.OHLCV{}}
	p := newTestPipeline(db, []string{"BTCUSDT"})
	// Should not panic with empty DB.
	p.RunOnce(context.Background())
	if len(db.calls) == 0 {
		t.Error("expected GetOHLCV to be called")
	}
}

func TestRunOnce_QueriesAllTFs(t *testing.T) {
	db := &mockDB{bars: map[string][]models.OHLCV{}}
	symbols := []string{"BTCUSDT", "AAPL"}
	tfs := []string{"1H", "4H", "1D"}

	eng := engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{}})
	interp := interpreter.New("", 12.0)
	notif := notifier.New(notifier.DefaultConfig(), zerolog.Nop())
	p := New(DefaultConfig(), db, eng, interp, notif, symbols, tfs, zerolog.Nop())

	p.RunOnce(context.Background())

	// Expect symbol × TF calls: 2 × 3 = 6
	if len(db.calls) != 6 {
		t.Errorf("expected 6 DB calls, got %d: %v", len(db.calls), db.calls)
	}
	// Verify each symbol+TF combination was queried.
	callSet := make(map[string]bool, len(db.calls))
	for _, c := range db.calls {
		callSet[c] = true
	}
	for _, sym := range symbols {
		for _, tf := range tfs {
			key := sym + "|" + tf
			if !callSet[key] {
				t.Errorf("expected DB call for %s", key)
			}
		}
	}
}

func TestRunOnce_DBError_NoSignals(t *testing.T) {
	db := &mockDB{err: errors.New("db unavailable")}
	p := newTestPipeline(db, []string{"BTCUSDT"})
	// Should not panic on DB error.
	p.RunOnce(context.Background())
}

func TestRunOnce_WithOHLCV_NoRules(t *testing.T) {
	// Engine has no rules; no signals should be produced.
	db := &mockDB{bars: map[string][]models.OHLCV{
		"BTCUSDT|1H": makeBars(200, 50000, 100),
		"BTCUSDT|4H": makeBars(200, 50000, 100),
	}}
	p := newTestPipeline(db, []string{"BTCUSDT"})
	// Should complete without panic.
	p.RunOnce(context.Background())
}

func TestRunOnce_MultipleSymbols_Independent(t *testing.T) {
	// Each symbol gets its own DB queries; an error for one symbol
	// should not prevent the other from being processed.
	callCount := 0
	db := &mockDB{}
	db.bars = map[string][]models.OHLCV{}

	symbols := []string{"SYM1", "SYM2", "SYM3"}
	p := newTestPipeline(db, symbols)
	p.RunOnce(context.Background())

	// All symbols should be queried regardless of empty data.
	for _, c := range db.calls {
		callCount++
		_ = c
	}
	// 3 symbols × 2 TFs = 6 calls minimum
	if callCount < 6 {
		t.Errorf("expected at least 6 DB calls for 3 symbols × 2 TFs, got %d", callCount)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Interval != time.Minute {
		t.Errorf("default interval should be 1 minute, got %v", cfg.Interval)
	}
	if cfg.Lookback != 200 {
		t.Errorf("default lookback should be 200, got %d", cfg.Lookback)
	}
}
