package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// fakeWatchlistSource is an in-memory WatchlistSource for tests.
type fakeWatchlistSource struct{ cfg appconfig.WatchlistConfig }

func (f *fakeWatchlistSource) Watchlist() appconfig.WatchlistConfig { return f.cfg }

func TestListWatchlist_RendersMarkdownTable(t *testing.T) {
	src := &fakeWatchlistSource{cfg: appconfig.WatchlistConfig{
		Symbols: struct {
			Crypto  []appconfig.SymbolEntry `yaml:"crypto"`
			Stocks  []appconfig.SymbolEntry `yaml:"stocks"`
			Indices []appconfig.SymbolEntry `yaml:"indices"`
		}{
			Crypto: []appconfig.SymbolEntry{{Symbol: "BTCUSDT", Exchange: "BINANCE", Enabled: true}},
			Stocks: []appconfig.SymbolEntry{{Symbol: "AAPL", Exchange: "NASDAQ", Enabled: true}},
		},
	}}
	tool := NewListWatchlist(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "BTCUSDT") || !strings.Contains(text, "AAPL") {
		t.Errorf("missing symbols in output: %q", text)
	}
	if !strings.Contains(text, "| Symbol | Exchange |") {
		t.Errorf("missing table header: %q", text)
	}
	if !strings.Contains(text, "2 symbols") {
		t.Errorf("missing count summary: %q", text)
	}
}

func TestListWatchlist_EmptyWatchlist(t *testing.T) {
	src := &fakeWatchlistSource{}
	tool := NewListWatchlist(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "0 symbols") {
		t.Errorf("empty watchlist summary missing: %q", text)
	}
}

func TestListWatchlist_MetaFields(t *testing.T) {
	tool := NewListWatchlist(&fakeWatchlistSource{})
	if tool.Name() != "list_watchlist" {
		t.Errorf("name: %s", tool.Name())
	}
	if !strings.Contains(tool.Description(), "watchlist") {
		t.Errorf("description missing 'watchlist': %s", tool.Description())
	}
	if tool.InputSchema() != SchemaListWatchlist {
		t.Error("schema mismatch")
	}
}

type fakeSignalSource struct {
	byKey map[string][]models.Signal // key = "SYM:TF"
	price float64
}

func (f *fakeSignalSource) GetSignalsFiltered(symbol, direction string, limit int) ([]models.Signal, error) {
	var out []models.Signal
	for k, sigs := range f.byKey {
		if !strings.HasPrefix(k, symbol+":") {
			continue
		}
		out = append(out, sigs...)
	}
	return out, nil
}

func (f *fakeSignalSource) LatestClose(symbol string) (float64, error) {
	return f.price, nil
}

func TestGetAnalysis_RendersFourTimeframes(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	src := &fakeSignalSource{
		price: 58432.10,
		byKey: map[string][]models.Signal{
			"BTCUSDT:1W": {{Symbol: "BTCUSDT", Timeframe: "1W", Rule: "wyckoff.accumulation_phase_C", Direction: "LONG", Score: 12.0, CreatedAt: now}},
			"BTCUSDT:1D": {{Symbol: "BTCUSDT", Timeframe: "1D", Rule: "ict.order_block_bullish", Direction: "LONG", Score: 14.5, CreatedAt: now, EntryPrice: 57800}},
			"BTCUSDT:4H": {{Symbol: "BTCUSDT", Timeframe: "4H", Rule: "ta.macd_bullish_cross", Direction: "LONG", Score: 11.0, CreatedAt: now}},
			// 1H intentionally missing
		},
	}
	watchSrc := &fakeWatchlistSource{cfg: appconfig.WatchlistConfig{
		Symbols: struct {
			Crypto  []appconfig.SymbolEntry `yaml:"crypto"`
			Stocks  []appconfig.SymbolEntry `yaml:"stocks"`
			Indices []appconfig.SymbolEntry `yaml:"indices"`
		}{
			Crypto: []appconfig.SymbolEntry{{Symbol: "BTCUSDT", Enabled: true}},
		},
	}}

	tool := NewGetAnalysis(watchSrc, src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	for _, tf := range []string{"1W", "1D", "4H", "1H"} {
		if !strings.Contains(text, "| "+tf+" |") {
			t.Errorf("missing TF row %s in: %q", tf, text)
		}
	}
	if !strings.Contains(text, "BTCUSDT") || !strings.Contains(text, "58432") {
		t.Errorf("missing header: %q", text)
	}
	if !strings.Contains(text, "ict.order_block_bullish") {
		t.Errorf("missing rule name: %q", text)
	}
}

func TestGetAnalysis_UnknownSymbolReturnsError(t *testing.T) {
	src := &fakeSignalSource{}
	watchSrc := &fakeWatchlistSource{}
	tool := NewGetAnalysis(watchSrc, src)
	_, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"NOPE"}`))
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	var mcpErr *Error
	if !errors.As(err, &mcpErr) {
		t.Fatalf("want *Error, got %T", err)
	}
	if mcpErr.Code != ErrCodeInvalidParams {
		t.Fatalf("want InvalidParams, got %d", mcpErr.Code)
	}
	if hint, ok := mcpErr.Data.(map[string]string); !ok || hint["hint"] == "" {
		t.Errorf("missing hint in Data: %+v", mcpErr.Data)
	}
}

func TestGetAnalysis_MissingSymbolParam(t *testing.T) {
	tool := NewGetAnalysis(&fakeWatchlistSource{}, &fakeSignalSource{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != ErrCodeInvalidParams {
		t.Fatalf("want InvalidParams, got %v", err)
	}
}

func TestGetSignalHistory_RendersTable(t *testing.T) {
	// Use time.Now() so signals stay within the tool's 7-day window
	// regardless of when the test runs. (Hardcoded 2026-04-22 was a time-bomb.)
	now := time.Now().UTC()
	src := &fakeSignalSource{
		byKey: map[string][]models.Signal{
			"BTCUSDT:1H": {
				{Symbol: "BTCUSDT", Timeframe: "1H", Rule: "ict.order_block_bullish", Direction: "LONG", Score: 14.0, CreatedAt: now.Add(-48 * time.Hour)},
				{Symbol: "BTCUSDT", Timeframe: "4H", Rule: "wyckoff.distribution_phase_D", Direction: "SHORT", Score: 11.5, CreatedAt: now.Add(-72 * time.Hour)},
			},
		},
	}
	tool := NewGetSignalHistory(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "ict.order_block_bullish") {
		t.Errorf("missing rule: %q", text)
	}
	if !strings.Contains(text, "BTCUSDT") {
		t.Errorf("missing symbol: %q", text)
	}
}

func TestGetSignalHistory_NoAlerts(t *testing.T) {
	tool := NewGetSignalHistory(&fakeSignalSource{})
	res, _ := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT"}`))
	if !strings.Contains(res.Content[0].Text, "0 alerts") {
		t.Errorf("no-alerts summary missing: %q", res.Content[0].Text)
	}
}

func TestGetSignalHistory_LimitClamp(t *testing.T) {
	tool := NewGetSignalHistory(&fakeSignalSource{})
	res, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT","limit":9999}`))
	if err != nil {
		t.Fatalf("limit clamp should silently cap, got err: %v", err)
	}
	_ = res
}

type fakeOHLCVSource struct{ rows []models.OHLCV }

func (f *fakeOHLCVSource) GetOHLCV(symbol, tf string, limit int) ([]models.OHLCV, error) {
	if limit > len(f.rows) {
		limit = len(f.rows)
	}
	return f.rows[:limit], nil
}

func TestGetOHLCV_ReturnsJSON(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	src := &fakeOHLCVSource{rows: []models.OHLCV{
		{Symbol: "BTCUSDT", Timeframe: "1H", OpenTime: now, Open: 58500, High: 58600, Low: 58400, Close: 58432, Volume: 123.45},
	}}
	tool := NewGetOHLCV(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT","timeframe":"1H"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	var js map[string]any
	if err := json.Unmarshal([]byte(text), &js); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, text)
	}
	if js["symbol"] != "BTCUSDT" {
		t.Errorf("symbol wrong: %v", js["symbol"])
	}
	candles, _ := js["candles"].([]any)
	if len(candles) != 1 {
		t.Errorf("want 1 candle, got %d", len(candles))
	}
}

func TestGetOHLCV_InvalidTimeframe(t *testing.T) {
	tool := NewGetOHLCV(&fakeOHLCVSource{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT","timeframe":"2H"}`))
	if err == nil {
		t.Fatal("want error for invalid tf")
	}
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != ErrCodeInvalidParams {
		t.Fatalf("want InvalidParams, got %v", err)
	}
}

func TestGetOHLCV_LimitClamp(t *testing.T) {
	tool := NewGetOHLCV(&fakeOHLCVSource{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT","timeframe":"1H","limit":9999}`))
	if err != nil {
		t.Fatalf("limit clamp should not error: %v", err)
	}
}

type fakeCalendarSource struct{ events []storage.EconomicEvent }

func (f *fakeCalendarSource) GetEconomicEvents(from, to time.Time) ([]storage.EconomicEvent, error) {
	var out []storage.EconomicEvent
	for _, e := range f.events {
		if e.EventTime.Before(from) || e.EventTime.After(to) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func TestGetEconomicCalendar_Basic(t *testing.T) {
	ts := time.Date(2026, 4, 23, 12, 30, 0, 0, time.UTC)
	src := &fakeCalendarSource{events: []storage.EconomicEvent{
		{Country: "US", Event: "US CPI YoY", Impact: "high", EventTime: ts},
	}}
	tool := NewGetEconomicCalendar(src)
	raw := json.RawMessage(`{"start":"2026-04-22T00:00:00Z","end":"2026-04-29T00:00:00Z"}`)
	res, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(res.Content[0].Text, "US CPI YoY") {
		t.Errorf("missing event: %q", res.Content[0].Text)
	}
}

func TestGetEconomicCalendar_StartAfterEnd(t *testing.T) {
	tool := NewGetEconomicCalendar(&fakeCalendarSource{})
	raw := json.RawMessage(`{"start":"2026-05-01T00:00:00Z","end":"2026-04-01T00:00:00Z"}`)
	_, err := tool.Call(context.Background(), raw)
	if err == nil {
		t.Fatal("want error: start > end")
	}
}

func TestGetEconomicCalendar_ImpactFilter(t *testing.T) {
	ts := time.Date(2026, 4, 23, 12, 30, 0, 0, time.UTC)
	src := &fakeCalendarSource{events: []storage.EconomicEvent{
		{Event: "Low impact", Impact: "low", EventTime: ts},
		{Event: "High impact", Impact: "high", EventTime: ts},
	}}
	tool := NewGetEconomicCalendar(src)
	raw := json.RawMessage(`{"start":"2026-04-22T00:00:00Z","end":"2026-04-29T00:00:00Z","impact_min":"high"}`)
	res, _ := tool.Call(context.Background(), raw)
	if strings.Contains(res.Content[0].Text, "Low impact") {
		t.Error("low-impact event should be filtered out")
	}
	if !strings.Contains(res.Content[0].Text, "High impact") {
		t.Error("high-impact event missing")
	}
}

func TestGetSignalHistory_RealDB_AllDirections(t *testing.T) {
	// Regression test: ensure MCP tools pass "ALL" (not "") for direction
	// so the SQL wildcard match in storage.signals works. Was returning
	// zero signals silently in production due to "" never matching the
	// 'ALL' wildcard in SQL.
	dbPath := t.TempDir() + "/mcp.db"
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer db.Close()

	// Insert two signals: one LONG and one SHORT.
	now := time.Now().UTC()
	for _, dir := range []string{"LONG", "SHORT"} {
		sig := models.Signal{
			Symbol: "BTCUSDT", Timeframe: "1H", Rule: "ict.order_block_bullish",
			Direction: dir, Score: 14.0, Message: "test",
			CreatedAt: now.Add(-1 * time.Hour),
		}
		if err := db.SaveSignal(sig); err != nil {
			t.Fatalf("SaveSignal: %v", err)
		}
	}

	tool := NewGetSignalHistory(db)
	res, err := tool.Call(context.Background(), json.RawMessage(`{"symbol":"BTCUSDT"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "LONG") || !strings.Contains(text, "SHORT") {
		t.Errorf("expected both LONG and SHORT in output (direction wildcard broken?), got: %q", text)
	}
}
