// Package calendar fetches economic events from Finnhub or Financial Modeling Prep and caches them locally.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
)

const (
	finnhubBase    = "https://finnhub.io/api/v1/calendar/economic"
	fmpBase        = "https://financialmodelingprep.com/stable/economics-calendar"
	fetchInterval  = 6 * time.Hour
	fetchLookahead = 14 * 24 * time.Hour // fetch next 14 days
)

// Store is the subset of storage.DB used by the Fetcher.
type Store interface {
	UpsertEconomicEvents(events []storage.EconomicEvent) error
	GetEconomicEvents(from, to time.Time) ([]storage.EconomicEvent, error)
}

// Fetcher periodically fetches economic events and caches them.
// Uses FMP if fmpKey is set, otherwise falls back to Finnhub.
type Fetcher struct {
	finnhubKey string
	fmpKey     string
	store      Store
	log        zerolog.Logger
	client     *http.Client
}

// New creates a Fetcher. FMP is preferred when both keys are set.
// If neither key is set, Run is a no-op.
func New(finnhubKey, fmpKey string, store Store, log zerolog.Logger) *Fetcher {
	return &Fetcher{
		finnhubKey: finnhubKey,
		fmpKey:     fmpKey,
		store:      store,
		log:        log,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// Run starts the periodic fetch loop. Fetches immediately on start, then every 6 hours.
func (f *Fetcher) Run(ctx context.Context) {
	if f.fmpKey == "" && f.finnhubKey == "" {
		f.log.Info().Msg("calendar: no API key configured — economic calendar disabled")
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

func (f *Fetcher) fetch(ctx context.Context) {
	if f.fmpKey != "" {
		f.fetchFMP(ctx)
	} else {
		f.fetchFinnhub(ctx)
	}
}

// ── Finnhub ───────────────────────────────────────────────────────────────────

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

func (f *Fetcher) fetchFinnhub(ctx context.Context) {
	now := time.Now().UTC()
	from := now.Format("2006-01-02")
	to := now.Add(fetchLookahead).Format("2006-01-02")

	url := fmt.Sprintf("%s?from=%s&to=%s&token=%s", finnhubBase, from, to, f.finnhubKey)
	body, status, err := f.doGet(ctx, url)
	if err != nil {
		f.log.Error().Err(err).Msg("calendar: Finnhub fetch failed")
		return
	}
	if status != http.StatusOK {
		f.log.Error().Int("status", status).Msg("calendar: Finnhub returned error")
		return
	}

	var result finnhubResponse
	if err := json.Unmarshal(body, &result); err != nil {
		f.log.Error().Err(err).Msg("calendar: Finnhub failed to parse response")
		return
	}

	var events []storage.EconomicEvent
	for _, e := range result.EconomicCalendar {
		if e.Country != "US" {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", e.Time)
		if err != nil {
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

	f.upsert(events, from, to, "finnhub")
}

// ── Financial Modeling Prep ───────────────────────────────────────────────────

type fmpEvent struct {
	Date     string   `json:"date"`     // "2026-03-21 12:30:00" or "2026-03-21"
	Country  string   `json:"country"`  // "US"
	Event    string   `json:"event"`
	Impact   string   `json:"impact"`   // "High" | "Medium" | "Low"
	Actual   *float64 `json:"actual"`
	Estimate *float64 `json:"estimate"`
	Previous *float64 `json:"previous"`
	Unit     string   `json:"unit"`
}

func (f *Fetcher) fetchFMP(ctx context.Context) {
	now := time.Now().UTC()
	from := now.Format("2006-01-02")
	to := now.Add(fetchLookahead).Format("2006-01-02")

	url := fmt.Sprintf("%s?from=%s&to=%s&apikey=%s", fmpBase, from, to, f.fmpKey)
	body, status, err := f.doGet(ctx, url)
	if err != nil {
		f.log.Error().Err(err).Msg("calendar: FMP fetch failed")
		return
	}
	if status != http.StatusOK {
		f.log.Error().Int("status", status).Msg("calendar: FMP returned error")
		return
	}

	var result []fmpEvent
	if err := json.Unmarshal(body, &result); err != nil {
		f.log.Error().Err(err).Msg("calendar: FMP failed to parse response")
		return
	}

	fmtNum := func(v *float64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'f', -1, 64)
	}

	var events []storage.EconomicEvent
	for _, e := range result {
		if e.Country != "US" {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", e.Date)
		if err != nil {
			t, err = time.Parse("2006-01-02", e.Date)
			if err != nil {
				continue
			}
		}
		events = append(events, storage.EconomicEvent{
			EventTime: t.UTC(),
			Country:   e.Country,
			Event:     e.Event,
			Impact:    strings.ToLower(e.Impact), // normalize "High" → "high"
			Actual:    fmtNum(e.Actual),
			Forecast:  fmtNum(e.Estimate),
			Previous:  fmtNum(e.Previous),
			Unit:      e.Unit,
		})
	}

	f.upsert(events, from, to, "fmp")
}

// ── shared helpers ────────────────────────────────────────────────────────────

func (f *Fetcher) doGet(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func (f *Fetcher) upsert(events []storage.EconomicEvent, from, to, provider string) {
	if len(events) == 0 {
		f.log.Debug().Str("provider", provider).Msg("calendar: no US events in response")
		return
	}
	if err := f.store.UpsertEconomicEvents(events); err != nil {
		f.log.Error().Err(err).Str("provider", provider).Msg("calendar: failed to cache events")
		return
	}
	f.log.Info().
		Int("events", len(events)).
		Str("from", from).
		Str("to", to).
		Str("provider", provider).
		Msg("calendar: events cached")
}
