package storage

import (
	"testing"
	"time"
)

// GetImminentHighImpact is the read-only, non-consuming query that backs alert
// annotation. It must return only high-impact events inside the window, soonest
// first, ignoring medium/low impact, past events, and the alerted flag.
func TestGetImminentHighImpact(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	events := []EconomicEvent{
		makeEvent(10, "high"),   // in window
		makeEvent(25, "high"),   // in window, later
		makeEvent(12, "medium"), // excluded: not high impact
		makeEvent(600, "high"),  // excluded: outside 30m window
		makeEvent(-10, "high"),  // excluded: already past
	}
	if err := db.UpsertEconomicEvents(events); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := db.GetImminentHighImpact(30 * time.Minute)
	if err != nil {
		t.Fatalf("GetImminentHighImpact: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 imminent high-impact events, got %d", len(got))
	}
	if !got[0].EventTime.Before(got[1].EventTime) {
		t.Errorf("expected soonest-first ordering, got %v then %v", got[0].EventTime, got[1].EventTime)
	}
	for _, e := range got {
		if e.Impact != "high" {
			t.Errorf("non-high event leaked: impact=%q", e.Impact)
		}
		if !e.EventTime.After(time.Now().Add(-time.Minute)) {
			t.Errorf("past event leaked: %v", e.EventTime)
		}
	}
}

// An alerted high-impact event must still surface for annotation — annotation is
// orthogonal to the watcher's one-shot pre-event alert.
func TestGetImminentHighImpact_IgnoresAlertedFlag(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	if err := db.UpsertEconomicEvents([]EconomicEvent{makeEvent(10, "high")}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	pending, err := db.GetUpcomingAlerts(30 * time.Minute)
	if err != nil || len(pending) != 1 {
		t.Fatalf("setup GetUpcomingAlerts: got %d (err=%v)", len(pending), err)
	}
	if err := db.MarkEventAlerted(pending[0].ID); err != nil {
		t.Fatalf("MarkEventAlerted: %v", err)
	}

	got, err := db.GetImminentHighImpact(30 * time.Minute)
	if err != nil {
		t.Fatalf("GetImminentHighImpact: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected alerted event to still be returned for annotation, got %d", len(got))
	}
}
