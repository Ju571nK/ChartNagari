package notifier

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

type fakeMacroStore struct {
	events []MacroEvent
	err    error
	calls  int
	window time.Duration
}

func (f *fakeMacroStore) ImminentHighImpact(window time.Duration) ([]MacroEvent, error) {
	f.calls++
	f.window = window
	return f.events, f.err
}

func TestDefaultForexMacroWindowAndLegacyAssets(t *testing.T) {
	for _, tc := range []struct {
		name, symbol string
		asset        models.AssetClass
		offset       time.Duration
		want         bool
	}{
		{"FX before 42m", "EURUSD", models.AssetForex, 42 * time.Minute, true},
		{"FX after 42m", "EURUSD", models.AssetForex, -42 * time.Minute, true},
		{"stock before 42m", "SPY", models.AssetStock, 42 * time.Minute, false},
		{"crypto after 42m", "BTCUSDT", models.AssetCrypto, -42 * time.Minute, false},
		{"stock before 25m", "SPY", models.AssetStock, 25 * time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := newNotifier(5, time.Hour)
			sender := &mockSender{}
			n.Register(sender)
			store := &fakeMacroStore{events: []MacroEvent{{EventTime: time.Now().Add(tc.offset), Country: "US", Event: "CPI"}}}
			n.WithMacroStore(store, 30*time.Minute)
			sig := makeSig(tc.symbol, "macro", "LONG", 10)
			sig.AssetClass = tc.asset
			n.Notify(context.Background(), []models.Signal{sig})
			if len(sender.calls) != 1 || (sender.calls[0].MacroNote != "") != tc.want {
				t.Fatalf("note=%q want warning=%t", noteOf(sender), tc.want)
			}
			if store.window != time.Hour {
				t.Fatalf("storage superset window=%v want 1h", store.window)
			}
		})
	}
}

func TestConfigurableForexMacroWindow(t *testing.T) {
	n := newNotifier(5, time.Hour)
	sender := &mockSender{}
	n.Register(sender)
	n.WithMacroStore(&fakeMacroStore{events: []MacroEvent{{EventTime: time.Now().Add(42 * time.Minute), Country: "US", Event: "CPI"}}}, 30*time.Minute)
	n.WithForexMacroWindow(45 * time.Minute)
	sig := makeSig("EURUSD", "macro", "LONG", 10)
	sig.AssetClass = models.AssetForex
	n.Notify(context.Background(), []models.Signal{sig})
	if len(sender.calls) != 1 || sender.calls[0].MacroNote == "" {
		t.Fatalf("configured FX warning missing: %q", noteOf(sender))
	}
}

// When a high-impact event is imminent, the dispatched signal carries a MacroNote.
func TestNotifier_MacroAnnotation_Appended(t *testing.T) {
	n := newNotifier(5.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)
	store := &fakeMacroStore{events: []MacroEvent{
		{EventTime: time.Now().Add(25 * time.Minute), Country: "US", Event: "CPI (MoM)"},
	}}
	n.WithMacroStore(store, 30*time.Minute)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 10)})

	if len(ms.calls) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(ms.calls))
	}
	note := ms.calls[0].MacroNote
	if !strings.Contains(note, "CPI (MoM)") || !strings.Contains(note, "High-impact macro event") {
		t.Fatalf("expected macro note with event name, got %q", note)
	}
}

// A lookup error must never block the alert — the signal still dispatches, unannotated.
func TestNotifier_MacroAnnotation_FailOpen(t *testing.T) {
	n := newNotifier(5.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)
	store := &fakeMacroStore{err: errors.New("db down")}
	n.WithMacroStore(store, 30*time.Minute)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 10)})

	if len(ms.calls) != 1 {
		t.Fatalf("fail-open: expected alert to still dispatch, got %d", len(ms.calls))
	}
	if ms.calls[0].MacroNote != "" {
		t.Errorf("expected empty note on lookup error, got %q", ms.calls[0].MacroNote)
	}
}

// No imminent events → no annotation.
func TestNotifier_MacroAnnotation_NoEvents(t *testing.T) {
	n := newNotifier(5.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)
	n.WithMacroStore(&fakeMacroStore{}, 30*time.Minute)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 10)})

	if len(ms.calls) != 1 || ms.calls[0].MacroNote != "" {
		t.Fatalf("expected dispatch with empty note, got %d calls note=%q", len(ms.calls), noteOf(ms))
	}
}

// Without a macro store wired, lookup is skipped entirely (no note, no store calls).
func TestNotifier_MacroAnnotation_Disabled(t *testing.T) {
	n := newNotifier(5.0, time.Hour)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 10)})

	if len(ms.calls) != 1 || ms.calls[0].MacroNote != "" {
		t.Fatalf("expected dispatch with empty note when disabled, got note=%q", noteOf(ms))
	}
}

// The Telegram formatter renders the MacroNote line when present.
func TestFormatTelegram_IncludesMacroNote(t *testing.T) {
	sig := makeSig("BTCUSDT", "rsi", "LONG", 10)
	sig.MacroNote = "⚠️ High-impact macro event in 24m: CPI (MoM) (US)"
	out := formatTelegram(sig)
	if !strings.Contains(out, sig.MacroNote) {
		t.Fatalf("formatTelegram should include macro note, got:\n%s", out)
	}
}

func noteOf(ms *mockSender) string {
	if len(ms.calls) == 0 {
		return "<no calls>"
	}
	return ms.calls[0].MacroNote
}

func TestForexMacroAnnotationPairCurrencies(t *testing.T) {
	n := newNotifier(5, time.Hour)
	ms := &mockSender{}
	n.Register(ms)
	n.WithMacroStore(&fakeMacroStore{events: []MacroEvent{
		{EventTime: time.Now().Add(10 * time.Minute), Country: "JP", Event: "BOJ"},
		{EventTime: time.Now().Add(30 * time.Minute), Country: "EU", Event: "ECB rate decision"},
		{EventTime: time.Now().Add(45 * time.Minute), Country: "US", Event: "CPI"},
	}}, time.Hour)
	sig := makeSig("EURUSD", "rsi", "LONG", 10)
	sig.AssetClass = models.AssetForex
	n.Notify(context.Background(), []models.Signal{sig})
	if len(ms.calls) != 1 || !strings.Contains(ms.calls[0].MacroNote, "EUR ECB rate decision in 30m") {
		t.Fatalf("wrong EURUSD warning: %q", noteOf(ms))
	}
}

func TestForexMacroAnnotationPastEventWithinWindow(t *testing.T) {
	n := newNotifier(5, time.Hour)
	ms := &mockSender{}
	n.Register(ms)
	n.WithMacroStore(&fakeMacroStore{events: []MacroEvent{{EventTime: time.Now().Add(-20 * time.Minute), Country: "US", Event: "CPI"}}}, time.Hour)
	sig := makeSig("EURUSD", "rsi", "LONG", 10)
	sig.AssetClass = models.AssetForex
	n.Notify(context.Background(), []models.Signal{sig})
	if len(ms.calls) != 1 || !strings.Contains(ms.calls[0].MacroNote, "USD CPI 20m ago") {
		t.Fatalf("wrong warning: %q", noteOf(ms))
	}
}

func TestFormatTelegramForexPipsAndProxy(t *testing.T) {
	sig := makeSig("EURUSD", "ict_fvg", "LONG", 10)
	sig.AssetClass, sig.EntryPrice, sig.TP, sig.SL = models.AssetForex, 1.08420, 1.08604, 1.08236
	sig.DataProxy = true
	out := formatTelegram(sig)
	for _, want := range []string{"1.08420", "+18.4 pips", "-18.4 pips", "proxy"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestDiscordForexPipsAndProxy(t *testing.T) {
	var payload string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payload = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	sig := makeSig("XAUUSD", "ict_fvg", "SHORT", 10)
	sig.AssetClass, sig.EntryPrice, sig.TP, sig.SL = models.AssetForex, 2650.0, 2648.0, 2652.0
	sig.DataProxy = true
	if err := NewDiscordSender(srv.URL).Send(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"20.0 pips", "proxy", "2650.00"} {
		if !strings.Contains(payload, want) {
			t.Errorf("missing %q in %s", want, payload)
		}
	}
	sig.Symbol, sig.EntryPrice, sig.TP, sig.SL, sig.DataProxy = "EURUSD", 1.08420, 1.08604, 1.08236, false
	if err := NewDiscordSender(srv.URL).Send(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, "1.08420") || !strings.Contains(payload, "18.4 pips") {
		t.Fatalf("EURUSD precision/pips missing: %s", payload)
	}
}
