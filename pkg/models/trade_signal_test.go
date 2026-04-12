package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestToTradeSignal_FieldMapping(t *testing.T) {
	src := Signal{
		Symbol:           "AAPL",
		Timeframe:        "4H",
		Rule:             "liquidity_sweep",
		Direction:        "LONG",
		Score:            14.5,
		Message:          "ignored by envelope",
		AIInterpretation: "Strong sweep with 2.3x volume",
		EntryPrice:       175.50,
		TP:               180.20,
		SL:               173.10,
		HTFTrend:         "LONG",
		ATRPercentile:    0.65,
		CreatedAt:        time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}

	ts := ToTradeSignal(src)

	if ts.Symbol != src.Symbol {
		t.Errorf("Symbol: got %q, want %q", ts.Symbol, src.Symbol)
	}
	if ts.Direction != src.Direction {
		t.Errorf("Direction: got %q, want %q", ts.Direction, src.Direction)
	}
	if ts.Timeframe != src.Timeframe {
		t.Errorf("Timeframe: got %q, want %q", ts.Timeframe, src.Timeframe)
	}
	if ts.Rule != src.Rule {
		t.Errorf("Rule: got %q, want %q", ts.Rule, src.Rule)
	}
	if ts.EntryPrice != src.EntryPrice {
		t.Errorf("EntryPrice: got %v, want %v", ts.EntryPrice, src.EntryPrice)
	}
	if ts.TakeProfit != src.TP {
		t.Errorf("TakeProfit: got %v, want %v (from Signal.TP)", ts.TakeProfit, src.TP)
	}
	if ts.StopLoss != src.SL {
		t.Errorf("StopLoss: got %v, want %v (from Signal.SL)", ts.StopLoss, src.SL)
	}
	if ts.Score != src.Score {
		t.Errorf("Score: got %v, want %v", ts.Score, src.Score)
	}
	if ts.HTFTrend != src.HTFTrend {
		t.Errorf("HTFTrend: got %q, want %q", ts.HTFTrend, src.HTFTrend)
	}
	if ts.ATRPercentile != src.ATRPercentile {
		t.Errorf("ATRPercentile: got %v, want %v", ts.ATRPercentile, src.ATRPercentile)
	}
	if ts.AIInterpretation != src.AIInterpretation {
		t.Errorf("AIInterpretation: got %q, want %q", ts.AIInterpretation, src.AIInterpretation)
	}
}

func TestToTradeSignal_EnvelopeInjection(t *testing.T) {
	src := Signal{Symbol: "AAPL", Direction: "LONG"}

	ts := ToTradeSignal(src)

	if ts.Version != TradeSignalVersion {
		t.Errorf("Version: got %q, want %q", ts.Version, TradeSignalVersion)
	}
	if ts.Version != "1.0" {
		t.Errorf("Version constant drifted: got %q", ts.Version)
	}

	if _, err := uuid.Parse(ts.ID); err != nil {
		t.Errorf("ID is not a valid UUID: %q (err: %v)", ts.ID, err)
	}

	if ts.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp not UTC: %v", ts.Timestamp.Location())
	}
	if delta := time.Since(ts.Timestamp); delta < 0 || delta > 5*time.Second {
		t.Errorf("Timestamp not ~now: got %v (delta %v)", ts.Timestamp, delta)
	}
}

func TestToTradeSignal_UniqueIDPerCall(t *testing.T) {
	src := Signal{Symbol: "AAPL"}
	a := ToTradeSignal(src)
	b := ToTradeSignal(src)
	if a.ID == b.ID {
		t.Fatalf("ToTradeSignal must generate a new UUID per call, got duplicate %q", a.ID)
	}
}

func TestInferAssetClassAndExchange(t *testing.T) {
	cases := []struct {
		symbol   string
		wantAC   string
		wantExch string
	}{
		{"AAPL", "stock", "nasdaq"},
		{"MSFT", "stock", "nasdaq"},
		{"BTCUSDT", "crypto", "binance"},
		{"ETHUSDT", "crypto", "binance"},
		{"btcusdt", "crypto", "binance"},
		{"BNBUSDC", "crypto", "binance"},
		{"ADABUSD", "crypto", "binance"},
		{"ETHBTC", "crypto", "binance"},
		{"LINKETH", "crypto", "binance"},
		{"BTC", "stock", "nasdaq"},  // exactly "BTC" alone → ambiguous, default stock
		{"USDT", "stock", "nasdaq"}, // exactly "USDT" alone → not a pair
		{"", "stock", "nasdaq"},
	}
	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			ac, ex := InferAssetClassAndExchange(tc.symbol)
			if ac != tc.wantAC || ex != tc.wantExch {
				t.Errorf("%q: got (%q,%q), want (%q,%q)", tc.symbol, ac, ex, tc.wantAC, tc.wantExch)
			}
		})
	}
}

func TestToTradeSignal_AssetClassInjection(t *testing.T) {
	cryptoTS := ToTradeSignal(Signal{Symbol: "BTCUSDT"})
	if cryptoTS.AssetClass != "crypto" || cryptoTS.Exchange != "binance" {
		t.Errorf("BTCUSDT: got (%q,%q), want (crypto,binance)", cryptoTS.AssetClass, cryptoTS.Exchange)
	}

	stockTS := ToTradeSignal(Signal{Symbol: "AAPL"})
	if stockTS.AssetClass != "stock" || stockTS.Exchange != "nasdaq" {
		t.Errorf("AAPL: got (%q,%q), want (stock,nasdaq)", stockTS.AssetClass, stockTS.Exchange)
	}
}

// TestTradeSignalJSON_Golden verifies the JSON shape against a frozen fixture.
// If this test fails, the wire format changed — bump TradeSignalVersion and
// update testdata/trade_signal_v1.json deliberately.
func TestTradeSignalJSON_Golden(t *testing.T) {
	fixed := TradeSignal{
		ID:               "11111111-2222-3333-4444-555555555555",
		Version:          "1.0",
		Timestamp:        time.Date(2026, 4, 12, 10, 30, 0, 0, time.UTC),
		Symbol:           "AAPL",
		Direction:        "LONG",
		Timeframe:        "4H",
		Rule:             "liquidity_sweep",
		EntryPrice:       175.50,
		TakeProfit:       180.20,
		StopLoss:         173.10,
		Score:            14.5,
		HTFTrend:         "LONG",
		ATRPercentile:    0.65,
		AssetClass:       "stock",
		Exchange:         "nasdaq",
		AIInterpretation: "Strong sweep with 2.3x volume",
	}

	got, err := json.MarshalIndent(fixed, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')

	goldenPath := filepath.Join("testdata", "trade_signal_v1.json")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}

	if string(got) != string(want) {
		t.Errorf("JSON shape drifted from golden %s.\n--- got ---\n%s\n--- want ---\n%s",
			goldenPath, got, want)
	}
}

func TestOrderFeedbackStatusEnum(t *testing.T) {
	// Compile-time sanity: verify the declared enum values match the spec.
	got := []string{
		OrderStatusReceived, OrderStatusSubmitted, OrderStatusFilled,
		OrderStatusPartialFill, OrderStatusRejected, OrderStatusCancelled, OrderStatusError,
	}
	want := []string{"RECEIVED", "SUBMITTED", "FILLED", "PARTIAL_FILL", "REJECTED", "CANCELLED", "ERROR"}
	if len(got) != len(want) {
		t.Fatalf("enum count: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("status[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}
