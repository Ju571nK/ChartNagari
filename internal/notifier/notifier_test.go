package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── mock sender ──────────────────────────────────────────────────────────────

type mockSender struct {
	calls []models.Signal
	err   error
}

func (m *mockSender) Send(_ context.Context, sig models.Signal) error {
	m.calls = append(m.calls, sig)
	return m.err
}
func (m *mockSender) Name() string { return "mock" }

// ── helpers ───────────────────────────────────────────────────────────────────

func nopLog() zerolog.Logger { return zerolog.Nop() }

func makeSig(symbol, rule, dir string, score float64) models.Signal {
	return models.Signal{
		Symbol:    symbol,
		Timeframe: "1H",
		Rule:      rule,
		Direction: dir,
		Score:     score,
		Message:   "test message",
		CreatedAt: time.Now(),
	}
}

func newNotifier(threshold float64, cooldown time.Duration) *Notifier {
	return New(Config{ScoreThreshold: threshold, CooldownDur: cooldown}, nopLog())
}

// ── tests ─────────────────────────────────────────────────────────────────────

// Test 1: signal below threshold is never forwarded to senders.
func TestNotifier_FilterBelowThreshold(t *testing.T) {
	n := newNotifier(5.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 4.9)})

	if len(ms.calls) != 0 {
		t.Fatalf("expected 0 sends, got %d", len(ms.calls))
	}
}

// Test 2: signal at or above threshold is forwarded.
func TestNotifier_FilterAboveThreshold(t *testing.T) {
	n := newNotifier(5.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 5.0)})

	if len(ms.calls) != 1 {
		t.Fatalf("expected 1 send, got %d", len(ms.calls))
	}
}

// Test 3: second Notify within cooldown window → not resent.
func TestNotifier_CooldownPreventsResend(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	sig := makeSig("BTCUSDT", "rsi", "LONG", 2.0)
	n.Notify(context.Background(), []models.Signal{sig})
	n.Notify(context.Background(), []models.Signal{sig})

	if len(ms.calls) != 1 {
		t.Fatalf("expected 1 send (cooldown), got %d", len(ms.calls))
	}
}

// Test 4: after cooldown expires the signal is sent again.
func TestNotifier_CooldownExpired(t *testing.T) {
	n := newNotifier(1.0, time.Millisecond)
	ms := &mockSender{}
	n.Register(ms)

	sig := makeSig("BTCUSDT", "rsi", "LONG", 2.0)
	n.Notify(context.Background(), []models.Signal{sig})

	// Advance the cooldown clock past the duration.
	future := time.Now().Add(time.Second)
	n.cooldown.setClock(func() time.Time { return future })

	n.Notify(context.Background(), []models.Signal{sig})

	if len(ms.calls) != 2 {
		t.Fatalf("expected 2 sends (cooldown expired), got %d", len(ms.calls))
	}
}

// Test 5: different symbols have independent cooldown windows.
func TestNotifier_DifferentSymbolNoCooldown(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 2.0)})
	n.Notify(context.Background(), []models.Signal{makeSig("ETHUSDT", "rsi", "LONG", 2.0)})

	if len(ms.calls) != 2 {
		t.Fatalf("expected 2 sends (different symbols), got %d", len(ms.calls))
	}
}

// Test 6: same symbol, different rule → independent cooldowns.
func TestNotifier_DifferentRuleNoCooldown(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 2.0)})
	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "ema_cross", "LONG", 2.0)})

	if len(ms.calls) != 2 {
		t.Fatalf("expected 2 sends (different rules), got %d", len(ms.calls))
	}
}

// Test 7: signal is sent to all registered senders.
func TestNotifier_MultipleSenders(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	a, b := &mockSender{}, &mockSender{}
	n.Register(a)
	n.Register(b)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 2.0)})

	if len(a.calls) != 1 || len(b.calls) != 1 {
		t.Fatalf("expected 1 send each, got a=%d b=%d", len(a.calls), len(b.calls))
	}
}

// Test 8: error from one sender does not stop other senders.
func TestNotifier_SenderError_OtherSendersStillRun(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	bad := &mockSender{err: errors.New("network error")}
	good := &mockSender{}
	n.Register(bad)
	n.Register(good)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 2.0)})

	if len(good.calls) != 1 {
		t.Fatalf("expected good sender to still receive signal, got %d", len(good.calls))
	}
}

// Test 9: LONG and SHORT for the same rule share the cooldown key.
func TestNotifier_CooldownKeyIgnoresDirection(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 2.0)})
	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "SHORT", 2.0)})

	if len(ms.calls) != 1 {
		t.Fatalf("expected 1 send (LONG and SHORT share cooldown), got %d", len(ms.calls))
	}
}

// Test 10: empty senders list — Notify must not panic.
func TestNotifier_NoSenders_NoPanic(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 2.0)})
}

// Test 11: empty signal list — no sends.
func TestNotifier_EmptySignals(t *testing.T) {
	n := newNotifier(1.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), nil)

	if len(ms.calls) != 0 {
		t.Fatalf("expected 0 sends for empty input, got %d", len(ms.calls))
	}
}

// Test 12: Cooldown.Allow — first call always returns true.
func TestCooldown_FirstCallAllows(t *testing.T) {
	c := NewCooldown(time.Hour)
	if !c.Allow("SYM", "rule") {
		t.Fatal("first call should be allowed")
	}
}

// Test 13: Cooldown.Allow — immediate second call blocked.
func TestCooldown_SecondCallBlocked(t *testing.T) {
	c := NewCooldown(time.Hour)
	c.Allow("SYM", "rule")
	if c.Allow("SYM", "rule") {
		t.Fatal("second call within cooldown should be blocked")
	}
}

// Test 14: formatTelegram contains expected fields.
func TestFormatTelegram_ContainsFields(t *testing.T) {
	sig := makeSig("AAPL", "ema_cross", "LONG", 8.5)
	msg := formatTelegram(sig)

	for _, want := range []string{"LONG", "AAPL", "ema_cross", "8.50"} {
		if !strings.Contains(msg, want) {
			t.Errorf("formatTelegram: expected %q in message, got:\n%s", want, msg)
		}
	}
}

// Test 14b: formatTelegram includes entry/TP/SL when EntryPrice > 0.
func TestFormatTelegram_WithLevels(t *testing.T) {
	sig := makeSig("BTCUSDT", "rsi", "LONG", 10.0)
	sig.EntryPrice = 65000.0
	sig.TP = 67000.0
	sig.SL = 64000.0
	msg := formatTelegram(sig)

	for _, want := range []string{"65000", "67000", "64000", "진입"} {
		if !strings.Contains(msg, want) {
			t.Errorf("formatTelegram with levels: expected %q in message, got:\n%s", want, msg)
		}
	}
}

// Test 14c: formatTelegram omits level line when EntryPrice == 0.
func TestFormatTelegram_NoLevelsWhenZero(t *testing.T) {
	sig := makeSig("BTCUSDT", "rsi", "LONG", 10.0)
	msg := formatTelegram(sig)
	if strings.Contains(msg, "진입") {
		t.Errorf("formatTelegram: should not show 진입 line when EntryPrice=0, got:\n%s", msg)
	}
}

// Test 15: TelegramSender returns error when token is empty.
func TestTelegramSender_EmptyToken_Error(t *testing.T) {
	s := NewTelegramSender("", "12345")
	err := s.Send(context.Background(), makeSig("BTC", "rsi", "LONG", 5.0))
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

// Test 16: TelegramSender sends correct JSON payload to a mock HTTP server.
func TestTelegramSender_SendsCorrectPayload(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		buf.ReadFrom(r.Body)
		received = buf.Bytes()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &TelegramSender{
		token:  "TOKEN",
		chatID: "CHAT",
		client: srv.Client(),
	}
	// Override the URL by pointing client at the test server via transport override.
	// We use a round-tripper that rewrites requests to the test server.
	s.client.Transport = &rewriteTransport{base: srv.URL}

	sig := makeSig("BTCUSDT", "rsi_overbought_oversold", "SHORT", 9.0)
	err := s.Send(context.Background(), sig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("could not unmarshal payload: %v", err)
	}
	if payload["chat_id"] != "CHAT" {
		t.Errorf("chat_id mismatch: %q", payload["chat_id"])
	}
	if payload["parse_mode"] != "HTML" {
		t.Errorf("parse_mode mismatch: %q", payload["parse_mode"])
	}
	if !strings.Contains(payload["text"], "SHORT") {
		t.Errorf("text does not contain direction: %q", payload["text"])
	}
}

// Test 17: DiscordSender returns error when webhookURL is empty.
func TestDiscordSender_EmptyURL_Error(t *testing.T) {
	s := NewDiscordSender("")
	err := s.Send(context.Background(), makeSig("BTC", "rsi", "LONG", 5.0))
	if err == nil {
		t.Fatal("expected error for empty webhookURL")
	}
}

// Test 18: DiscordSender sends embed payload with correct color.
func TestDiscordSender_SendsEmbedPayload(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		buf.ReadFrom(r.Body)
		received = buf.Bytes()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &DiscordSender{webhookURL: srv.URL, client: srv.Client()}

	sig := makeSig("ETHUSDT", "ict_order_block", "LONG", 12.0)
	if err := s.Send(context.Background(), sig); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("could not unmarshal payload: %v", err)
	}
	embeds, ok := payload["embeds"].([]interface{})
	if !ok || len(embeds) == 0 {
		t.Fatal("expected embeds array in payload")
	}
	embed := embeds[0].(map[string]interface{})
	if color, ok := embed["color"].(float64); !ok || int(color) != discordColor("LONG") {
		t.Errorf("embed color mismatch: got %v, want %d", embed["color"], discordColor("LONG"))
	}
}

// ── rewriteTransport ─────────────────────────────────────────────────────────
// Redirects all outbound HTTP requests to a fixed base URL (test server).
type rewriteTransport struct {
	base string
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = strings.TrimPrefix(rt.base, "http://")
	return http.DefaultTransport.RoundTrip(req2)
}
