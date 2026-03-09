package interpreter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// anthropicResponse returns a minimal Anthropic Messages API JSON response.
func anthropicResponse(text string) []byte {
	resp := map[string]interface{}{
		"id":    "msg_test",
		"type":  "message",
		"role":  "assistant",
		"model": "claude-opus-4-6",
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
		"stop_reason": "end_turn",
		"usage":       map[string]interface{}{"input_tokens": 100, "output_tokens": 50},
	}
	b, _ := json.Marshal(resp)
	return b
}

func makeSignal(symbol, rule, direction string, score float64) models.Signal {
	return models.Signal{
		Symbol:    symbol,
		Timeframe: "4H",
		Rule:      rule,
		Direction: direction,
		Score:     score,
		Message:   "테스트 신호",
		CreatedAt: time.Now(),
	}
}

// ── disabled (no API key) ────────────────────────────────────────────────────

func TestEnrich_Disabled(t *testing.T) {
	interp := New("", 12.0)
	sig := makeSignal("BTCUSDT", "rsi_overbought_oversold", "LONG", 15.0)
	groups := []SignalGroup{{Symbol: "BTCUSDT", Signals: []models.Signal{sig}}}

	out := interp.Enrich(context.Background(), groups)

	if len(out) != 1 {
		t.Fatalf("want 1 signal, got %d", len(out))
	}
	if out[0].AIInterpretation != "" {
		t.Error("disabled interpreter must not populate AIInterpretation")
	}
}

// ── below minScore ───────────────────────────────────────────────────────────

func TestEnrich_BelowMinScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("API must not be called when score is below threshold")
		w.WriteHeader(500)
	}))
	defer srv.Close()

	interp := New("test-key", 12.0, option.WithBaseURL(srv.URL))
	sig := makeSignal("BTCUSDT", "ema_cross", "LONG", 5.0)
	groups := []SignalGroup{{Symbol: "BTCUSDT", Signals: []models.Signal{sig}}}

	out := interp.Enrich(context.Background(), groups)

	if len(out) != 1 {
		t.Fatalf("want 1 signal, got %d", len(out))
	}
	if out[0].AIInterpretation != "" {
		t.Error("below-threshold group must not be enriched")
	}
}

// ── successful API call ──────────────────────────────────────────────────────

func TestEnrich_Success(t *testing.T) {
	const aiText = "RSI 과매도 + OB 재진입 신호. 강세 반전 가능성 높음. 손절: 전저점 아래."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(anthropicResponse(aiText))
	}))
	defer srv.Close()

	interp := New("test-key", 12.0, option.WithBaseURL(srv.URL))
	sig1 := makeSignal("NVDA", "rsi_overbought_oversold", "LONG", 8.0)
	sig2 := makeSignal("NVDA", "ict_order_block", "LONG", 6.0)
	groups := []SignalGroup{
		{Symbol: "NVDA", Signals: []models.Signal{sig1, sig2}},
	}

	out := interp.Enrich(context.Background(), groups)

	if len(out) != 2 {
		t.Fatalf("want 2 signals, got %d", len(out))
	}
	for _, s := range out {
		if s.AIInterpretation != aiText {
			t.Errorf("want AIInterpretation=%q, got %q", aiText, s.AIInterpretation)
		}
	}
}

// ── API error → graceful degradation ────────────────────────────────────────

func TestEnrich_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	interp := New("test-key", 12.0, option.WithBaseURL(srv.URL))
	sig := makeSignal("AAPL", "wyckoff_spring", "LONG", 15.0)
	groups := []SignalGroup{{Symbol: "AAPL", Signals: []models.Signal{sig}}}

	out := interp.Enrich(context.Background(), groups)

	if len(out) != 1 {
		t.Fatalf("want 1 signal, got %d", len(out))
	}
	if out[0].AIInterpretation != "" {
		t.Error("API error must not leave partial AIInterpretation")
	}
}

// ── empty groups ─────────────────────────────────────────────────────────────

func TestEnrich_EmptyGroups(t *testing.T) {
	interp := New("test-key", 12.0)
	out := interp.Enrich(context.Background(), nil)
	if len(out) != 0 {
		t.Errorf("want empty output, got %d signals", len(out))
	}
}

// ── multiple groups: only high-score group gets AI ───────────────────────────

func TestEnrich_MixedScores(t *testing.T) {
	calls := 0
	const aiText = "복합 신호 해석"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(anthropicResponse(aiText))
	}))
	defer srv.Close()

	interp := New("test-key", 12.0, option.WithBaseURL(srv.URL))

	lowSig := makeSignal("ETH", "ema_cross", "LONG", 3.0)   // total=3 < 12
	highSig := makeSignal("BTC", "ict_order_block", "LONG", 14.0) // total=14 >= 12

	groups := []SignalGroup{
		{Symbol: "ETH", Signals: []models.Signal{lowSig}},
		{Symbol: "BTC", Signals: []models.Signal{highSig}},
	}

	out := interp.Enrich(context.Background(), groups)

	if len(out) != 2 {
		t.Fatalf("want 2 signals, got %d", len(out))
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 API call, got %d", calls)
	}
	// ETH signal (low score): no interpretation
	if out[0].AIInterpretation != "" {
		t.Error("low-score signal must not have AIInterpretation")
	}
	// BTC signal (high score): has interpretation
	if out[1].AIInterpretation != aiText {
		t.Errorf("high-score signal must have AIInterpretation=%q, got %q", aiText, out[1].AIInterpretation)
	}
}

// ── prompt contains symbol and rule names ────────────────────────────────────

func TestBuildPrompt_ContainsContext(t *testing.T) {
	sig := makeSignal("TSLA", "ict_fair_value_gap", "SHORT", 8.0)
	g := SignalGroup{
		Symbol:     "TSLA",
		Signals:    []models.Signal{sig},
		Indicators: map[string]float64{"1H:RSI_14": 72.5, "4H:ATR_14": 0.8},
	}

	p := buildPrompt(g)

	for _, want := range []string{"TSLA", "ict_fair_value_gap", "1H:RSI_14", "4H:ATR_14"} {
		if !contains(p, want) {
			t.Errorf("prompt must contain %q", want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}

// ── TP/SL/R:R in prompt ──────────────────────────────────────────────────────

func TestBuildPrompt_ContainsEntryLevels(t *testing.T) {
	sig := models.Signal{
		Symbol:     "BTCUSDT",
		Timeframe:  "4H",
		Rule:       "ict_order_block",
		Direction:  "LONG",
		Score:      8.0,
		Message:    "OB 진입 신호",
		EntryPrice: 65000.0,
		TP:         67000.0,
		SL:         64000.0,
		CreatedAt:  time.Now(),
	}
	g := SignalGroup{
		Symbol:  "BTCUSDT",
		Signals: []models.Signal{sig},
	}

	p := buildPrompt(g)

	for _, want := range []string{"65000", "67000", "64000", "R:R"} {
		if !contains(p, want) {
			t.Errorf("prompt must contain %q", want)
		}
	}
}

// ── MTF confluence header ─────────────────────────────────────────────────────

func TestBuildPrompt_MTFConfluence(t *testing.T) {
	sigs := []models.Signal{
		{Symbol: "ETHUSDT", Timeframe: "1H", Rule: "ema_cross", Direction: "LONG", Score: 6.0, CreatedAt: time.Now()},
		{Symbol: "ETHUSDT", Timeframe: "4H", Rule: "ict_order_block", Direction: "LONG", Score: 8.0, CreatedAt: time.Now()},
		{Symbol: "ETHUSDT", Timeframe: "1D", Rule: "rsi_overbought_oversold", Direction: "SHORT", Score: 5.0, CreatedAt: time.Now()},
	}
	g := SignalGroup{Symbol: "ETHUSDT", Signals: sigs}

	p := buildPrompt(g)

	for _, want := range []string{"MTF 합류", "LONG 2개", "SHORT 1개", "롱 우세"} {
		if !contains(p, want) {
			t.Errorf("prompt must contain %q", want)
		}
	}
}
