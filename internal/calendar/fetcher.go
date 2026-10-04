// Package calendar fetches economic events from Finnhub or Financial Modeling Prep and caches them locally.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
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

// retryDelays defines fixed backoff between fetch attempts (2 retries after first failure).
var retryDelays = []time.Duration{time.Minute, 5 * time.Minute}

// Store is the subset of storage.DB used by the Fetcher.
type Store interface {
	UpsertEconomicEvents(events []storage.EconomicEvent) error
	GetEconomicEvents(from, to time.Time) ([]storage.EconomicEvent, error)
}

// Fetcher periodically fetches economic events and caches them.
// Uses FMP if fmpKey is set, otherwise falls back to Finnhub.
type Fetcher struct {
	mu             sync.RWMutex
	countries      map[string]bool
	status         CollectionStatus
	finnhubKey     string
	fmpKey         string
	store          Store
	log            zerolog.Logger
	client         *http.Client
	refresh        chan struct{} // coalesces watchlist changes until Run can fetch
	finnhubBaseURL string        // overridable for tests; defaults to finnhubBase
	fmpBaseURL     string        // overridable for tests; defaults to fmpBase
}

// SetForexSymbols updates the countries retained by future fetches. With no FX
// symbols the historical US-only calendar behaviour is preserved.
func (f *Fetcher) SetForexSymbols(symbols []string) {
	countries := CountriesForSymbols(symbols)
	f.mu.Lock()
	changed := len(countries) != len(f.countries)
	if !changed {
		for country := range countries {
			if !f.countries[country] {
				changed = true
				break
			}
		}
	}
	f.countries = countries
	f.mu.Unlock()
	if changed && f.refresh != nil {
		select {
		case f.refresh <- struct{}{}:
		default:
		}
	}
}

func (f *Fetcher) acceptsCountry(country string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.countries == nil {
		return normalizeCountry(country) == "US"
	}
	return f.countries[normalizeCountry(country)]
}

func normalizeCountry(country string) string {
	country = strings.ToUpper(strings.TrimSpace(country))
	switch country {
	case "UNITED STATES", "UNITED STATES OF AMERICA", "USA":
		return "US"
	case "EURO AREA", "EUROZONE", "EUROPEAN UNION":
		return "EU"
	case "GERMANY":
		return "DE"
	case "FRANCE":
		return "FR"
	case "ITALY":
		return "IT"
	case "UNITED KINGDOM", "UK":
		return "GB"
	case "JAPAN":
		return "JP"
	case "SWITZERLAND":
		return "CH"
	case "CANADA":
		return "CA"
	case "AUSTRALIA":
		return "AU"
	case "NEW ZEALAND":
		return "NZ"
	case "CHINA":
		return "CN"
	}
	return country
}

var currencyCountries = map[string][]string{
	"USD": {"US"}, "EUR": {"EU", "DE", "FR", "IT"},
	"GBP": {"GB"}, "JPY": {"JP"}, "CHF": {"CH"},
	"CAD": {"CA"}, "AUD": {"AU"}, "NZD": {"NZ"}, "CNY": {"CN"},
}

// CountriesForSymbols returns the relevant event countries, including US for
// existing stock and crypto alerts. Unknown or malformed FX symbols are ignored.
func CountriesForSymbols(symbols []string) map[string]bool {
	countries := map[string]bool{"US": true}
	for _, symbol := range symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if len(symbol) != 6 {
			continue
		}
		for _, currency := range []string{symbol[:3], symbol[3:]} {
			for _, country := range currencyCountries[currency] {
				countries[country] = true
			}
		}
	}
	return countries
}

// New creates a Fetcher. FMP is preferred when both keys are set.
// If neither key is set, Run is a no-op.
func New(finnhubKey, fmpKey string, store Store, log zerolog.Logger) *Fetcher {
	return &Fetcher{
		finnhubKey:     finnhubKey,
		fmpKey:         fmpKey,
		store:          store,
		log:            log,
		client:         &http.Client{Timeout: 15 * time.Second},
		refresh:        make(chan struct{}, 1),
		finnhubBaseURL: finnhubBase,
		fmpBaseURL:     fmpBase,
	}
}

// Run starts the periodic fetch loop. Fetches immediately on start (with backoff retry),
// then every 6 hours.
func (f *Fetcher) Run(ctx context.Context) {
	if f.fmpKey == "" && f.finnhubKey == "" {
		f.log.Info().Msg("calendar: no API key configured — economic calendar disabled")
		return
	}
	f.fetchWithRetry(ctx)

	ticker := time.NewTicker(fetchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			f.fetchWithRetry(ctx)
		case <-f.refresh:
			f.fetchWithRetry(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// fetchWithRetry attempts a fetch; on failure retries after 1min then 5min before giving up.
func (f *Fetcher) fetchWithRetry(ctx context.Context) {
	if f.tryFetch(ctx) {
		return
	}
	for i, delay := range retryDelays {
		f.log.Warn().Dur("retry_in", delay).Int("attempt", i+1).Msg("calendar: fetch failed, will retry")
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		if f.tryFetch(ctx) {
			return
		}
	}
	f.log.Error().Msg("calendar: all fetch attempts failed, waiting for next 6h cycle")
}

// tryFetch executes a single fetch attempt. Returns true on success.
func (f *Fetcher) tryFetch(ctx context.Context) bool {
	f.mu.Lock()
	f.status.State = "fetching"
	f.status.LastAttempt = time.Now().UTC()
	f.mu.Unlock()
	var err error
	if f.fmpKey != "" {
		err = f.fetchFMP(ctx)
	} else {
		err = f.fetchFinnhub(ctx)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.State = "success"
	f.status.Failure = ""
	if err != nil {
		f.status.State = "error"
		f.status.Failure = "network"
		var failure collectionError
		if errors.As(err, &failure) {
			f.status.Failure = string(failure)
		}
	} else {
		f.status.LastSuccess = time.Now().UTC()
	}
	return err == nil
}

// ── Finnhub ───────────────────────────────────────────────────────────────────

type finnhubResponse struct {
	EconomicCalendar []finnhubEvent `json:"economicCalendar"`
}

type finnhubEvent struct {
	Time     string `json:"time"`    // "2026-03-21 12:30:00"
	Country  string `json:"country"` // "US"
	Event    string `json:"event"`
	Impact   string `json:"impact"` // "high" | "medium" | "low"
	Actual   string `json:"actual"`
	Estimate string `json:"estimate"`
	Prev     string `json:"prev"`
	Unit     string `json:"unit"`
}

func (f *Fetcher) fetchFinnhub(ctx context.Context) error {
	now := time.Now().UTC()
	from := now.Format("2006-01-02")
	to := now.Add(fetchLookahead).Format("2006-01-02")

	// Use X-Finnhub-Token header — keeps the API key out of URL/access logs.
	url := fmt.Sprintf("%s?from=%s&to=%s", f.finnhubBaseURL, from, to)
	body, status, err := f.doGet(ctx, url, map[string]string{"X-Finnhub-Token": f.finnhubKey})
	if err != nil {
		f.log.Error().Err(err).Msg("calendar: Finnhub fetch failed")
		return err
	}
	if status != http.StatusOK {
		err := httpFailure(status)
		f.log.Error().Int("status", status).Msg("calendar: Finnhub returned error")
		return err
	}

	var result finnhubResponse
	if err := json.Unmarshal(body, &result); err != nil {
		f.log.Error().Err(err).Msg("calendar: Finnhub failed to parse response")
		return collectionError("invalid_response")
	}
	if result.EconomicCalendar == nil {
		return collectionError("invalid_response")
	}

	var events []storage.EconomicEvent
	for _, e := range result.EconomicCalendar {
		if !f.acceptsCountry(e.Country) {
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
			Country:   normalizeCountry(e.Country),
			Event:     e.Event,
			Impact:    e.Impact,
			Actual:    e.Actual,
			Forecast:  e.Estimate,
			Previous:  e.Prev,
			Unit:      e.Unit,
		})
	}

	return f.upsert(events, from, to, "finnhub")
}

// ── Financial Modeling Prep ───────────────────────────────────────────────────

type fmpEvent struct {
	Date     string   `json:"date"`    // "2026-03-21 12:30:00" or "2026-03-21"
	Country  string   `json:"country"` // "US"
	Event    string   `json:"event"`
	Impact   string   `json:"impact"` // "High" | "Medium" | "Low"
	Actual   *float64 `json:"actual"`
	Estimate *float64 `json:"estimate"`
	Previous *float64 `json:"previous"`
	Unit     string   `json:"unit"`
}

func (f *Fetcher) fetchFMP(ctx context.Context) error {
	now := time.Now().UTC()
	from := now.Format("2006-01-02")
	to := now.Add(fetchLookahead).Format("2006-01-02")

	url := fmt.Sprintf("%s?from=%s&to=%s&apikey=%s", f.fmpBaseURL, from, to, f.fmpKey)
	body, status, err := f.doGet(ctx, url, nil)
	if err != nil {
		f.log.Error().Err(err).Msg("calendar: FMP fetch failed")
		return err
	}
	if status != http.StatusOK {
		err := httpFailure(status)
		f.log.Error().Int("status", status).Msg("calendar: FMP returned error")
		return err
	}

	var result []fmpEvent
	if err := json.Unmarshal(body, &result); err != nil {
		f.log.Error().Err(err).Msg("calendar: FMP failed to parse response")
		return collectionError("invalid_response")
	}
	if result == nil {
		return collectionError("invalid_response")
	}

	fmtNum := func(v *float64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'f', -1, 64)
	}

	var events []storage.EconomicEvent
	for _, e := range result {
		if !f.acceptsCountry(e.Country) {
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
			Country:   normalizeCountry(e.Country),
			Event:     e.Event,
			Impact:    strings.ToLower(e.Impact), // normalize "High" → "high"
			Actual:    fmtNum(e.Actual),
			Forecast:  fmtNum(e.Estimate),
			Previous:  fmtNum(e.Previous),
			Unit:      e.Unit,
		})
	}

	return f.upsert(events, from, to, "fmp")
}

// ── shared helpers ────────────────────────────────────────────────────────────

func (f *Fetcher) doGet(ctx context.Context, url string, headers map[string]string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		// Transport errors can embed the FMP URL (and its API key).
		return nil, 0, collectionError("network")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func (f *Fetcher) upsert(events []storage.EconomicEvent, from, to, provider string) error {
	if len(events) == 0 {
		f.log.Debug().Str("provider", provider).Msg("calendar: no relevant events in response")
		f.mu.Lock()
		f.status.EventCount = 0
		f.mu.Unlock()
		return nil
	}
	if err := f.store.UpsertEconomicEvents(events); err != nil {
		f.log.Error().Err(err).Str("provider", provider).Msg("calendar: failed to cache events")
		return collectionError("storage")
	}
	f.mu.Lock()
	f.status.EventCount = len(events)
	f.mu.Unlock()
	f.log.Info().
		Int("events", len(events)).
		Str("from", from).
		Str("to", to).
		Str("provider", provider).
		Msg("calendar: events cached")
	return nil
}
