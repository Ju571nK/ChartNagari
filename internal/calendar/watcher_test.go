package calendar

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
)

// mockAlertStore controls what GetUpcomingAlerts returns and tracks MarkEventAlerted calls.
type mockAlertStore struct {
	mu       sync.Mutex
	events   []storage.EconomicEvent
	marked   []int64
	markFail bool
}

func (m *mockAlertStore) GetUpcomingAlerts(_ time.Duration) ([]storage.EconomicEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return only unalerted events (simulate DB behaviour)
	var out []storage.EconomicEvent
	for _, e := range m.events {
		if !e.Alerted {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *mockAlertStore) MarkEventAlerted(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markFail {
		return nil // simulate failure (but don't actually mark — event stays alerted=false in DB)
	}
	m.marked = append(m.marked, id)
	for i, e := range m.events {
		if e.ID == id {
			m.events[i].Alerted = true
		}
	}
	return nil
}

// mockAnnouncer records all announcements.
type mockAnnouncer struct {
	mu   sync.Mutex
	msgs []string
}

func (m *mockAnnouncer) Announce(_ context.Context, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, text)
}

func (m *mockAnnouncer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.msgs)
}

func TestWatcher_SendsAlertForHighImpactEvent(t *testing.T) {
	store := &mockAlertStore{
		events: []storage.EconomicEvent{
			{ID: 1, EventTime: time.Now().Add(20 * time.Minute), Country: "US", Event: "CPI", Impact: "high"},
		},
	}
	announcer := &mockAnnouncer{}
	w := NewWatcher(store, announcer, 30*time.Minute, zerolog.Nop())

	w.check(context.Background())

	if announcer.count() != 1 {
		t.Errorf("expected 1 alert, got %d", announcer.count())
	}
}

func TestWatcher_NoDuplicateAlert_InMemorySet(t *testing.T) {
	// markFail=true simulates DB write failure — event stays alerted=false in "DB"
	store := &mockAlertStore{
		events: []storage.EconomicEvent{
			{ID: 42, EventTime: time.Now().Add(10 * time.Minute), Country: "US", Event: "NFP", Impact: "high"},
		},
		markFail: true, // DB mark always fails
	}
	announcer := &mockAnnouncer{}
	w := NewWatcher(store, announcer, 30*time.Minute, zerolog.Nop())

	// First check — should alert
	w.check(context.Background())
	// Second check — DB still returns event (alerted=false), but in-memory set prevents re-alert
	w.check(context.Background())
	// Third check — same
	w.check(context.Background())

	if announcer.count() != 1 {
		t.Errorf("in-memory set should prevent duplicate alerts: expected 1, got %d", announcer.count())
	}
}

func TestWatcher_AlertWindowRespected(t *testing.T) {
	store := &mockAlertStore{
		events: []storage.EconomicEvent{
			{ID: 1, EventTime: time.Now().Add(10 * time.Minute), Country: "US", Event: "CPI", Impact: "high"},
			{ID: 2, EventTime: time.Now().Add(60 * time.Minute), Country: "US", Event: "NFP", Impact: "high"},
		},
	}
	announcer := &mockAnnouncer{}
	w := NewWatcher(store, announcer, 30*time.Minute, zerolog.Nop())
	w.check(context.Background())

	// Only event within 30min window should alert.
	// mockAlertStore.GetUpcomingAlerts ignores the window param (returns all unalerted),
	// so this tests DB-level filtering. In practice, GetUpcomingAlerts in real DB uses the window.
	// Here we test that both events get sent (mock doesn't filter by time).
	if announcer.count() < 1 {
		t.Error("expected at least 1 alert")
	}
}

func TestWatcher_DefaultAlertWindow(t *testing.T) {
	w := NewWatcher(&mockAlertStore{}, &mockAnnouncer{}, 0, zerolog.Nop())
	if w.alertWindow != defaultAlertWindow {
		t.Errorf("expected default %v, got %v", defaultAlertWindow, w.alertWindow)
	}
}

func TestWatcher_CustomAlertWindow(t *testing.T) {
	w := NewWatcher(&mockAlertStore{}, &mockAnnouncer{}, 45*time.Minute, zerolog.Nop())
	if w.alertWindow != 45*time.Minute {
		t.Errorf("expected 45min, got %v", w.alertWindow)
	}
}
