package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/storage"
)

type fakeCalendarStore struct {
	events []storage.EconomicEvent
	err    error
}

func (f fakeCalendarStore) GetEconomicEvents(_, _ time.Time) ([]storage.EconomicEvent, error) {
	return f.events, f.err
}

// GET /api/calendar returns the events the store yields.
func TestGetCalendar_OK(t *testing.T) {
	srv := setupTest(t)
	now := time.Now()
	srv.WithCalendarStore(fakeCalendarStore{events: []storage.EconomicEvent{
		{ID: 1, EventTime: now.Add(time.Hour), Country: "US", Event: "CPI (MoM)", Impact: "high"},
		{ID: 2, EventTime: now.Add(2 * time.Hour), Country: "US", Event: "NFP", Impact: "high"},
	}})

	w := do(t, srv, "GET", "/api/calendar?from=2026-05-01&to=2026-05-31", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body)
	}
	var got []storage.EconomicEvent
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	if got[0].Event != "CPI (MoM)" {
		t.Errorf("unexpected first event: %q", got[0].Event)
	}
}

// An empty result must serialize as [] (not null) so the frontend can map over it.
func TestGetCalendar_EmptyReturnsArray(t *testing.T) {
	srv := setupTest(t)
	srv.WithCalendarStore(fakeCalendarStore{events: nil})

	w := do(t, srv, "GET", "/api/calendar", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body := strings.TrimSpace(w.Body.String()); body != "[]" {
		t.Fatalf("want empty JSON array, got %q", body)
	}
}

// A store error surfaces as 500.
func TestGetCalendar_StoreError(t *testing.T) {
	srv := setupTest(t)
	srv.WithCalendarStore(fakeCalendarStore{err: errors.New("calendar store unavailable")})

	w := do(t, srv, "GET", "/api/calendar", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
