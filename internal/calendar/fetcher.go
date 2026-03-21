// Package calendar fetches economic events from Finnhub and caches them locally.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
)

const (
	finnhubBase    = "https://finnhub.io/api/v1/calendar/economic"
	fetchInterval  = 6 * time.Hour
	fetchLookahead = 14 * 24 * time.Hour // fetch next 14 days
)

// Store is the subset of storage.DB used by the Fetcher.
type Store interface {
	UpsertEconomicEvents(events []storage.EconomicEvent) error
	GetEconomicEvents(from, to time.Time) ([]storage.EconomicEvent, error)
}

// Fetcher periodically fetches economic events from Finnhub and caches them.
type Fetcher struct {
	apiKey string
	store  Store
	log    zerolog.Logger
	client *http.Client
}

// New creates a Fetcher. If apiKey is empty, Fetch is a no-op.
func New(apiKey string, store Store, log zerolog.Logger) *Fetcher {
	return &Fetcher{
		apiKey: apiKey,
		store:  store,
		log:    log,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Run starts the periodic fetch loop. Fetches immediately on start, then every 6 hours.
func (f *Fetcher) Run(ctx context.Context) {
	if f.apiKey == "" {
		f.log.Info().Msg("calendar: Finnhub API key not set — economic calendar disabled")
		return
	}
	f.fetch(ctx)

	ticker := time.NewTicker(fetchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			f.fetch(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// finnhubResponse matches the Finnhub economic calendar API response.
type finnhubResponse struct {
	EconomicCalendar []finnhubEvent `json:"economicCalendar"`
}

type finnhubEvent struct {
	Time     string `json:"time"`     // "2026-03-21 12:30:00"
	Country  string `json:"country"`  // "US"
	Event    string `json:"event"`
	Impact   string `json:"impact"`   // "high" | "medium" | "low"
	Actual   string `json:"actual"`
	Estimate string `json:"estimate"`
	Prev     string `json:"prev"`
	Unit     string `json:"unit"`
}

func (f *Fetcher) fetch(ctx context.Context) {
	now := time.Now().UTC()
	from := now.Format("2006-01-02")
	to := now.Add(fetchLookahead).Format("2006-01-02")

	url := fmt.Sprintf("%s?from=%s&to=%s&token=%s", finnhubBase, from, to, f.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		f.log.Error().Err(err).Msg("calendar: failed to create request")
		return
	}

	resp, err := f.client.Do(req)
	if err != nil {
		f.log.Error().Err(err).Msg("calendar: fetch failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.log.Error().Int("status", resp.StatusCode).Msg("calendar: Finnhub returned error")
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.log.Error().Err(err).Msg("calendar: failed to read response")
		return
	}

	var result finnhubResponse
	if err := json.Unmarshal(body, &result); err != nil {
		f.log.Error().Err(err).Msg("calendar: failed to parse response")
		return
	}

	var events []storage.EconomicEvent
	for _, e := range result.EconomicCalendar {
		if e.Country != "US" {
			continue // only US events for now
		}
		t, err := time.Parse("2006-01-02 15:04:05", e.Time)
		if err != nil {
			// Try date-only format
			t, err = time.Parse("2006-01-02", e.Time)
			if err != nil {
				continue
			}
		}
		events = append(events, storage.EconomicEvent{
			EventTime: t.UTC(),
			Country:   e.Country,
			Event:     e.Event,
			Impact:    e.Impact,
			Actual:    e.Actual,
			Forecast:  e.Estimate,
			Previous:  e.Prev,
			Unit:      e.Unit,
		})
	}

	if len(events) == 0 {
		f.log.Debug().Msg("calendar: no US events in response")
		return
	}

	if err := f.store.UpsertEconomicEvents(events); err != nil {
		f.log.Error().Err(err).Msg("calendar: failed to cache events")
		return
	}
	f.log.Info().Int("events", len(events)).Str("from", from).Str("to", to).Msg("calendar: events cached")
}
