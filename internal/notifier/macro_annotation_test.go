package notifier

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

type fakeMacroStore struct {
	events []MacroEvent
	err    error
	calls  int
}

func (f *fakeMacroStore) ImminentHighImpact(window time.Duration) ([]MacroEvent, error) {
	f.calls++
	return f.events, f.err
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
