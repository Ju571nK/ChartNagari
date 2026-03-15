package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

const avBaseURL = "https://www.alphavantage.co/query"

// AlphaVantageCollector fetches daily bars from Alpha Vantage (free tier: 25 req/day).
// On each startup it checks the DB first and only fetches what is actually missing:
//   - No data at all       → outputsize=full  (up to 20 years)
//   - Gap ≤ 90 days        → outputsize=compact (last ~100 trading days)
//   - Data up-to-date (≤3 days stale, covers weekends/holidays) → skip
type AlphaVantageCollector struct {
	apiKey  string
	db      *storage.DB
	symbols []string
	client  *http.Client
}

// NewAlphaVantageCollector creates an AlphaVantageCollector.
func NewAlphaVantageCollector(apiKey string, db *storage.DB, symbols []string) *AlphaVantageCollector {
	return &AlphaVantageCollector{
		apiKey:  apiKey,
		db:      db,
		symbols: symbols,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Start checks for missing data and fetches only what is needed, then blocks until ctx is cancelled.
func (c *AlphaVantageCollector) Start(ctx context.Context) {
	log.Info().Strs("symbols", c.symbols).Msg("[AlphaVantage] checking data status")
	for i, sym := range c.symbols {
		outputSize, skip := c.determineOutputSize(sym)
		if skip {
			log.Info().Str("symbol", sym).Msg("[AlphaVantage] data up-to-date — skipping")
			continue
		}
		if err := c.fetchAndStore(sym, outputSize); err != nil {
			log.Error().Err(err).Str("symbol", sym).Str("outputSize", outputSize).Msg("[AlphaVantage] fetch failed")
		}
		// AV free tier: 5 req/min → 12s gap between actual requests
		if i < len(c.symbols)-1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(12 * time.Second):
			}
		}
	}
	log.Info().Msg("[AlphaVantage] fetch completed")
	<-ctx.Done()
}

// determineOutputSize checks the DB and returns the appropriate AV outputsize parameter.
// Returns ("", true) when data is already up-to-date and no fetch is needed.
func (c *AlphaVantageCollector) determineOutputSize(symbol string) (outputSize string, skip bool) {
	bars, err := c.db.GetOHLCV(symbol, "1D", 1)
	if err != nil || len(bars) == 0 {
		// No data — fetch full 20-year history
		log.Info().Str("symbol", symbol).Msg("[AlphaVantage] no data in DB → full fetch")
		return "full", false
	}

	// GetOHLCV returns DESC order; bars[0] is the most recent bar
	latest := bars[0].OpenTime
	daysSince := int(time.Since(latest).Hours() / 24)

	switch {
	case daysSince <= 3:
		// Up-to-date: ≤3 days covers weekends + US holidays
		return "", true
	case daysSince <= 90:
		// Compact covers the last ~100 trading days (~5 months calendar)
		log.Info().Str("symbol", symbol).Int("days_since", daysSince).
			Time("latest_bar", latest).Msg("[AlphaVantage] partial update → compact fetch")
		return "compact", false
	default:
		// Stale by more than 90 days — fetch the full history again
		log.Info().Str("symbol", symbol).Int("days_since", daysSince).
			Time("latest_bar", latest).Msg("[AlphaVantage] stale data → full fetch")
		return "full", false
	}
}

func (c *AlphaVantageCollector) fetchAndStore(symbol, outputSize string) error {
	url := fmt.Sprintf(
		"%s?function=TIME_SERIES_DAILY&symbol=%s&outputsize=%s&apikey=%s",
		avBaseURL, symbol, outputSize, c.apiKey,
	)
	resp, err := c.client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("response read failed: %w", err)
	}

	bars, err := parseAVResponse(symbol, body)
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return fmt.Errorf("no data: %s", symbol)
	}

	if err := c.db.SaveOHLCVBatch(bars, "alphavantage"); err != nil {
		return fmt.Errorf("save failed: %w", err)
	}
	log.Info().Str("symbol", symbol).Str("outputSize", outputSize).Int("bars", len(bars)).
		Msg("[AlphaVantage] saved to DB")
	return nil
}

type avTimeSeries struct {
	Open   string `json:"1. open"`
	High   string `json:"2. high"`
	Low    string `json:"3. low"`
	Close  string `json:"4. close"`
	Volume string `json:"5. volume"`
}

type avResponse struct {
	TimeSeries  map[string]avTimeSeries `json:"Time Series (Daily)"`
	Note        string                  `json:"Note"`
	Information string                  `json:"Information"`
}

func parseAVResponse(symbol string, data []byte) ([]models.OHLCV, error) {
	var raw avResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}
	if raw.Information != "" {
		return nil, fmt.Errorf("AV API error: %s", raw.Information)
	}
	if raw.Note != "" {
		log.Warn().Str("note", raw.Note).Msg("[AlphaVantage] API note")
	}
	if len(raw.TimeSeries) == 0 {
		return nil, fmt.Errorf("TimeSeries empty (check API key)")
	}

	sym := strings.ToUpper(symbol)
	bars := make([]models.OHLCV, 0, len(raw.TimeSeries))
	for dateStr, v := range raw.TimeSeries {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		bar := models.OHLCV{
			Symbol:    sym,
			Timeframe: "1D",
			OpenTime:  t.UTC(),
		}
		fmt.Sscanf(v.Open, "%f", &bar.Open)
		fmt.Sscanf(v.High, "%f", &bar.High)
		fmt.Sscanf(v.Low, "%f", &bar.Low)
		fmt.Sscanf(v.Close, "%f", &bar.Close)
		fmt.Sscanf(v.Volume, "%f", &bar.Volume)
		bars = append(bars, bar)
	}

	sort.Slice(bars, func(i, j int) bool {
		return bars[i].OpenTime.Before(bars[j].OpenTime)
	})
	return bars, nil
}
