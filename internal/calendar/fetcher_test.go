package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
)

// mockStore records upserted events for assertion.
type mockStore struct {
	upserted []storage.EconomicEvent
}

func (m *mockStore) UpsertEconomicEvents(events []storage.EconomicEvent) error {
	m.upserted = append(m.upserted, events...)
	return nil
}

func (m *mockStore) GetEconomicEvents(_, _ time.Time) ([]storage.EconomicEvent, error) {
	return nil, nil
}

// newFMPFetcher wires a Fetcher to a test server acting as FMP.
func newFMPFetcher(srv *httptest.Server, store Store) *Fetcher {
	return &Fetcher{
		fmpKey:     "testkey",
		store:      store,
		log:        zerolog.Nop(),
		client:     srv.Client(),
		fmpBaseURL: srv.URL,
	}
}

// newFinnhubFetcher wires a Fetcher to a test server acting as Finnhub.
func newFinnhubFetcher(srv *httptest.Server, store Store) *Fetcher {
	return &Fetcher{
		finnhubKey:     "testkey",
		store:          store,
		log:            zerolog.Nop(),
		client:         srv.Client(),
		finnhubBaseURL: srv.URL,
	}
}

// ── Finnhub tests ─────────────────────────────────────────────────────────────

func TestFetchFinnhub_APIKeyInHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Finnhub-Token") != "testkey" {
			t.Errorf("expected X-Finnhub-Token header, got %q", r.Header.Get("X-Finnhub-Token"))
		}
		if r.URL.Query().Get("token") != "" {
			t.Errorf("API key must not appear in URL, got token=%q", r.URL.Query().Get("token"))
		}
		json.NewEncoder(w).Encode(finnhubResponse{})
	}))
	defer srv.Close()

	f := newFinnhubFetcher(srv, &mockStore{})
	_ = f.fetchFinnhub(context.Background())
}

func TestFetchFinnhub_FiltersNonUS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(finnhubResponse{
			EconomicCalendar: []finnhubEvent{
				{Time: "2026-03-21 12:30:00", Country: "US", Event: "CPI", Impact: "high"},
				{Time: "2026-03-21 13:00:00", Country: "EU", Event: "ECB", Impact: "high"},
				{Time: "2026-03-21 14:00:00", Country: "JP", Event: "BOJ", Impact: "high"},
			},
		})
	}))
	defer srv.Close()

	store := &mockStore{}
	f := newFinnhubFetcher(srv, store)
	if err := f.fetchFinnhub(context.Background()); err != nil {
		t.Fatalf("fetchFinnhub: %v", err)
	}
	if len(store.upserted) != 1 || store.upserted[0].Country != "US" {
		t.Errorf("expected 1 US event, got %d", len(store.upserted))
	}
}

func TestFetchFinnhub_DateOnlyFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(finnhubResponse{
			EconomicCalendar: []finnhubEvent{
				{Time: "2026-03-21", Country: "US", Event: "Holiday", Impact: "low"},
			},
		})
	}))
	defer srv.Close()

	store := &mockStore{}
	f := newFinnhubFetcher(srv, store)
	if err := f.fetchFinnhub(context.Background()); err != nil {
		t.Fatalf("date-only format should not error: %v", err)
	}
	if len(store.upserted) != 1 {
		t.Error("expected date-only event to be parsed and stored")
	}
}

func TestFetchFinnhub_Returns403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	f := newFinnhubFetcher(srv, &mockStore{})
	if err := f.fetchFinnhub(context.Background()); err == nil {
		t.Error("expected error on 403, got nil")
	}
}

// ── FMP tests ─────────────────────────────────────────────────────────────────

func TestFetchFMP_NullNumericFields(t *testing.T) {
	body := `[{"date":"2026-03-21 12:30:00","country":"US","event":"CPI","impact":"High","actual":null,"estimate":null,"previous":0.2,"unit":"%"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	store := &mockStore{}
	f := newFMPFetcher(srv, store)
	if err := f.fetchFMP(context.Background()); err != nil {
		t.Fatalf("fetchFMP: %v", err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.upserted))
	}
	ev := store.upserted[0]
	if ev.Actual != "" {
		t.Errorf("null actual should be empty string, got %q", ev.Actual)
	}
	if ev.Forecast != "" {
		t.Errorf("null estimate should be empty string, got %q", ev.Forecast)
	}
	if ev.Previous != "0.2" {
		t.Errorf("expected previous=0.2, got %q", ev.Previous)
	}
}

func TestFetchFMP_NormalizesImpactToLower(t *testing.T) {
	body := `[{"date":"2026-03-21 12:30:00","country":"US","event":"CPI","impact":"High","unit":""}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	store := &mockStore{}
	f := newFMPFetcher(srv, store)
	_ = f.fetchFMP(context.Background())

	if len(store.upserted) == 1 && store.upserted[0].Impact != "high" {
		t.Errorf("FMP 'High' should normalize to 'high', got %q", store.upserted[0].Impact)
	}
}

func TestFetchFMP_FiltersNonUS(t *testing.T) {
	body := `[
		{"date":"2026-03-21 12:30:00","country":"US","event":"CPI","impact":"High","unit":""},
		{"date":"2026-03-21 13:00:00","country":"EU","event":"ECB","impact":"High","unit":""}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	store := &mockStore{}
	f := newFMPFetcher(srv, store)
	_ = f.fetchFMP(context.Background())

	if len(store.upserted) != 1 || store.upserted[0].Country != "US" {
		t.Errorf("expected 1 US event, got %d", len(store.upserted))
	}
}

func TestFetchFMP_Returns403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	f := newFMPFetcher(srv, &mockStore{})
	if err := f.fetchFMP(context.Background()); err == nil {
		t.Error("expected error on 403, got nil")
	}
}

// ── backoff tests ─────────────────────────────────────────────────────────────

func TestFetchWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Second attempt: return empty FMP response (success)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	// Patch retryDelays to zero for fast test
	orig := retryDelays
	retryDelays = []time.Duration{0, 0}
	defer func() { retryDelays = orig }()

	store := &mockStore{}
	f := newFMPFetcher(srv, store)
	f.fetchWithRetry(context.Background())

	if attempts < 2 {
		t.Errorf("expected at least 2 attempts (1 failure + 1 retry), got %d", attempts)
	}
}

func TestFetchWithRetry_GivesUpAfterMaxRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable) // always fail
	}))
	defer srv.Close()

	orig := retryDelays
	retryDelays = []time.Duration{0, 0}
	defer func() { retryDelays = orig }()

	f := newFMPFetcher(srv, &mockStore{})
	f.fetchWithRetry(context.Background())

	// 1 initial + 2 retries = 3 total
	if attempts != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", attempts)
	}
}
