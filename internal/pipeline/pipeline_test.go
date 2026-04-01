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
	interp := interpreter.New("", 12.0, "en") // disabled — no API key
	notif := notifier.New(notifier.DefaultConfig(), zerolog.Nop())
	p := New(
		DefaultConfig(),
		db,
		eng,
		interp,
		notif,
		symbols,
		[]string{"1H", "4H"},
		zerolog.Nop(),
	)
	// Mark all symbols as crypto so tests run regardless of market hours.
	p.SetCryptoSymbols(symbols)
	return p
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
	interp := interpreter.New("", 12.0, "en")
	notif := notifier.New(notifier.DefaultConfig(), zerolog.Nop())
	p := New(DefaultConfig(), db, eng, interp, notif, symbols, tfs, zerolog.Nop())
	// Mark all symbols as crypto so tests run regardless of market hours.
	p.SetCryptoSymbols(symbols)

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

// ── HTF Context Filter tests ────────────────────────────────────────────────

func TestHTFContext_Uptrend(t *testing.T) {
	indicators := map[string]float64{
		"1D:EMA_50":  150.0,
		"1D:EMA_200": 130.0, // EMA_50 > EMA_200
	}
	bars := map[string][]models.OHLCV{
		"1D": {{Close: 160.0}}, // price > EMA_50
	}
	trend := htfContext(indicators, "1D", bars)
	if trend != "LONG" {
		t.Errorf("expected LONG uptrend, got %q", trend)
	}
}

func TestHTFContext_Downtrend(t *testing.T) {
	indicators := map[string]float64{
		"1D:EMA_50":  120.0,
		"1D:EMA_200": 140.0, // EMA_50 < EMA_200
	}
	bars := map[string][]models.OHLCV{
		"1D": {{Close: 110.0}}, // price < EMA_50
	}
	trend := htfContext(indicators, "1D", bars)
	if trend != "SHORT" {
		t.Errorf("expected SHORT downtrend, got %q", trend)
	}
}

func TestHTFContext_Ranging(t *testing.T) {
	indicators := map[string]float64{
		"1D:EMA_50":  135.0,
		"1D:EMA_200": 130.0, // EMA_50 > EMA_200
	}
	bars := map[string][]models.OHLCV{
		"1D": {{Close: 125.0}}, // but price < EMA_50 → ambiguous
	}
	trend := htfContext(indicators, "1D", bars)
	if trend != "" {
		t.Errorf("expected empty (ranging), got %q", trend)
	}
}

func TestHTFContext_NoData(t *testing.T) {
	indicators := map[string]float64{}
	bars := map[string][]models.OHLCV{}
	trend := htfContext(indicators, "1D", bars)
	if trend != "" {
		t.Errorf("expected empty (no data), got %q", trend)
	}
}

func TestFilterHTFContext_UptrendFiltersShorts(t *testing.T) {
	indicators := map[string]float64{
		"1D:EMA_50": 150.0, "1D:EMA_200": 130.0,
	}
	bars := map[string][]models.OHLCV{
		"1D": {{Close: 160.0}},
	}
	signals := []models.Signal{
		{Direction: "LONG", Timeframe: "1H", Rule: "ema_cross"},
		{Direction: "SHORT", Timeframe: "1H", Rule: "rsi_overbought"},  // should be filtered
		{Direction: "SHORT", Timeframe: "4H", Rule: "ict_order_block"}, // should be filtered
		{Direction: "LONG", Timeframe: "4H", Rule: "ict_fair_value_gap"},
		{Direction: "SHORT", Timeframe: "1D", Rule: "smc_choch"},       // HTF signal — kept
		{Direction: "NEUTRAL", Timeframe: "1H", Rule: "test"},          // NEUTRAL — kept
	}
	result := filterHTFContext(signals, indicators, bars)
	if len(result) != 4 {
		t.Fatalf("expected 4 signals after filter, got %d: %+v", len(result), result)
	}
	for _, s := range result {
		if (s.Timeframe == "1H" || s.Timeframe == "4H") && s.Direction == "SHORT" {
			t.Errorf("SHORT LTF signal should have been filtered: %+v", s)
		}
	}
}

func TestFilterHTFContext_RangingKeepsAll(t *testing.T) {
	indicators := map[string]float64{
		"1D:EMA_50": 135.0, "1D:EMA_200": 130.0,
	}
	bars := map[string][]models.OHLCV{
		"1D": {{Close: 125.0}}, // ranging
	}
	signals := []models.Signal{
		{Direction: "LONG", Timeframe: "1H"},
		{Direction: "SHORT", Timeframe: "1H"},
		{Direction: "SHORT", Timeframe: "4H"},
	}
	result := filterHTFContext(signals, indicators, bars)
	if len(result) != 3 {
		t.Errorf("ranging should keep all signals, got %d", len(result))
	}
}

func TestFilterHTFContext_FallsBackToWeekly(t *testing.T) {
	indicators := map[string]float64{
		// No 1D EMAs, but 1W shows downtrend
		"1W:EMA_50": 100.0, "1W:EMA_200": 120.0,
	}
	bars := map[string][]models.OHLCV{
		"1W": {{Close: 90.0}}, // price < EMA_50 → downtrend
	}
	signals := []models.Signal{
		{Direction: "LONG", Timeframe: "1H"},  // should be filtered
		{Direction: "SHORT", Timeframe: "1H"}, // aligned — kept
	}
	result := filterHTFContext(signals, indicators, bars)
	if len(result) != 1 || result[0].Direction != "SHORT" {
		t.Errorf("expected only SHORT to survive 1W downtrend filter, got %+v", result)
	}
}
