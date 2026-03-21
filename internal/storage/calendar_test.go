package storage

import (
	"testing"
	"time"
)

func makeEvent(minsFromNow int, impact string) EconomicEvent {
	return EconomicEvent{
		EventTime: time.Now().Add(time.Duration(minsFromNow) * time.Minute).UTC(),
		Country:   "US",
		Event:     "Test Event",
		Impact:    impact,
		Forecast:  "0.3",
		Previous:  "0.2",
		Unit:      "%",
	}
}

func TestUpsertAndGetEconomicEvents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC()
	events := []EconomicEvent{
		{EventTime: now.Add(1 * time.Hour), Country: "US", Event: "CPI (MoM)", Impact: "high"},
		{EventTime: now.Add(2 * time.Hour), Country: "US", Event: "NFP", Impact: "high"},
		{EventTime: now.Add(3 * time.Hour), Country: "EU", Event: "ECB Rate", Impact: "high"}, // non-US, but upsert doesn't filter
	}

	if err := db.UpsertEconomicEvents(events); err != nil {
		t.Fatalf("UpsertEconomicEvents: %v", err)
	}

	got, err := db.GetEconomicEvents(now, now.Add(4*time.Hour))
	if err != nil {
		t.Fatalf("GetEconomicEvents: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 events, got %d", len(got))
	}

	// Upsert again — should update, not duplicate
	events[0].Forecast = "0.4" // updated forecast
	if err := db.UpsertEconomicEvents(events); err != nil {
		t.Fatalf("UpsertEconomicEvents (update): %v", err)
	}
	got2, _ := db.GetEconomicEvents(now, now.Add(4*time.Hour))
	if len(got2) != 3 {
		t.Errorf("upsert should not duplicate: expected 3, got %d", len(got2))
	}
	if got2[0].Forecast != "0.4" {
		t.Errorf("upsert should update forecast: got %q", got2[0].Forecast)
	}
}

func TestGetEconomicEvents_DateRange(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC()
	_ = db.UpsertEconomicEvents([]EconomicEvent{
		{EventTime: now.Add(-2 * time.Hour), Country: "US", Event: "Past Event", Impact: "high"},
		{EventTime: now.Add(1 * time.Hour), Country: "US", Event: "Future Event", Impact: "high"},
	})

	// Query only future
	got, _ := db.GetEconomicEvents(now, now.Add(2*time.Hour))
	if len(got) != 1 || got[0].Event != "Future Event" {
		t.Errorf("expected only future event, got %v", got)
	}
}

func TestGetUpcomingAlerts_FiltersCorrectly(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	events := []EconomicEvent{
		makeEvent(20, "high"),   // within window — should alert
		makeEvent(40, "high"),   // outside window — skip
		makeEvent(10, "medium"), // medium impact — skip
		makeEvent(15, "low"),    // low impact — skip
	}
	_ = db.UpsertEconomicEvents(events)

	alerts, err := db.GetUpcomingAlerts(30 * time.Minute)
	if err != nil {
		t.Fatalf("GetUpcomingAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Errorf("expected 1 high-impact alert within window, got %d", len(alerts))
	}
	if alerts[0].Impact != "high" {
		t.Errorf("expected high impact, got %q", alerts[0].Impact)
	}
}

func TestMarkEventAlerted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_ = db.UpsertEconomicEvents([]EconomicEvent{makeEvent(10, "high")})

	// Before marking
	alerts, _ := db.GetUpcomingAlerts(30 * time.Minute)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 unalerted event, got %d", len(alerts))
	}

	if err := db.MarkEventAlerted(alerts[0].ID); err != nil {
		t.Fatalf("MarkEventAlerted: %v", err)
	}

	// After marking — should not appear in upcoming alerts
	alerts2, _ := db.GetUpcomingAlerts(30 * time.Minute)
	if len(alerts2) != 0 {
		t.Errorf("expected 0 alerts after marking, got %d", len(alerts2))
	}
}
