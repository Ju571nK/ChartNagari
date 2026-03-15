package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

const tiingoBaseURL = "https://api.tiingo.com"

// tiingoMinInterval defines the minimum time between fetches per timeframe.
// Keeps request count within Tiingo free-tier limits (50 req/hr).
//
//	1H / 4H : 15 min  → 2 symbols × 2 TF × 4 polls/hr = 16 req/hr
//	1D      :  6 hr   → 2 symbols × 1 TF × (1/6) poll/hr ≈ 0.3 req/hr
//	1W      : 24 hr   → ~0 req/hr
var tiingoMinInterval = map[string]time.Duration{
	"1H": 15 * time.Minute,
	"4H": 15 * time.Minute,
	"1D": 6 * time.Hour,
	"1W": 24 * time.Hour,
}

// tiingoState is persisted to disk so restart doesn't lose fetch history.
type tiingoState struct {
	LastFetched map[string]time.Time `json:"last_fetched"`
	RateLimited time.Time            `json:"rate_limited"`
}

// TiingoCollector polls Tiingo REST API for stock OHLCV data.
// Requires a free API key from tiingo.com → set TIINGO_API_KEY in .env.
//
// Endpoint usage:
//   - 1D, 1W → /tiingo/daily/{symbol}/prices  (free plan)
//   - 1H, 4H → /iex/{symbol}/prices?resampleFreq=1hour  (free, ~30 days)
type TiingoCollector struct {
	apiKey       string
	db           *storage.DB
	symbols      []string
	timeframes   []string
	pollInterval time.Duration
	httpClient   *http.Client
	stateFile    string

	mu          sync.Mutex
	lastFetched map[string]time.Time // key: "SYMBOL:TF"
	rateLimited time.Time            // pause all requests until this time on 429
}

// NewTiingoCollector creates a TiingoCollector.
func NewTiingoCollector(apiKey string, db *storage.DB, symbols, timeframes []string, pollInterval time.Duration) *TiingoCollector {
	return &TiingoCollector{
		apiKey:       apiKey,
		db:           db,
		symbols:      symbols,
		timeframes:   timeframes,
		pollInterval: pollInterval,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		lastFetched:  make(map[string]time.Time),
	}
}

// SetStateFile sets the path for persisting fetch timestamps across restarts.
func (c *TiingoCollector) SetStateFile(path string) {
	c.stateFile = path
}

// Start begins polling. Blocks until ctx is cancelled.
func (c *TiingoCollector) Start(ctx context.Context) {
	c.loadState()

	log.Info().
		Strs("symbols", c.symbols).
		Dur("poll_interval", c.pollInterval).
		Msg("[Tiingo] collector started")

	c.fetchAll()

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("[Tiingo] collector stopped")
			return
		case <-ticker.C:
			if !isMarketOpen() {
				c.fetchForTimeframes([]string{"1D", "1W"})
				continue
			}
			c.fetchAll()
		}
	}
}

func (c *TiingoCollector) fetchAll() {
	c.fetchForTimeframes(c.timeframes)
}

func (c *TiingoCollector) fetchForTimeframes(timeframes []string) {
	// Global 429 backoff: skip entire cycle if still in cooldown.
	c.mu.Lock()
	if time.Now().Before(c.rateLimited) {
		remaining := time.Until(c.rateLimited).Truncate(time.Second)
		log.Warn().Dur("resume_in", remaining).Msg("[Tiingo] rate limited — skipping")
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	for _, sym := range c.symbols {
		for _, tf := range timeframes {
			if !c.shouldFetch(sym, tf) {
				continue
			}

			bars, err := c.fetchOHLCV(sym, tf)
			if err != nil {
				if is429(err) {
					// Back off for 30 minutes on rate-limit.
					c.mu.Lock()
					c.rateLimited = time.Now().Add(30 * time.Minute)
					c.mu.Unlock()
					c.saveState()
					log.Warn().Msg("[Tiingo] hourly request limit exceeded — retrying in 30 minutes")
					return
				}
				log.Error().Err(err).Str("symbol", sym).Str("tf", tf).Msg("[Tiingo] data fetch failed")
				continue
			}

			if err := c.db.SaveOHLCVBatch(bars, "tiingo"); err != nil {
				log.Error().Err(err).Msg("[Tiingo] save failed")
				continue
			}
			c.markFetched(sym, tf)
			log.Debug().Str("symbol", sym).Str("tf", tf).Int("bars", len(bars)).Msg("[Tiingo] data saved")
		}
	}
}

// shouldFetch returns true if enough time has passed since the last fetch
// for this symbol+timeframe combination.
func (c *TiingoCollector) shouldFetch(sym, tf string) bool {
	minInterval, ok := tiingoMinInterval[tf]
	if !ok {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	last := c.lastFetched[sym+":"+tf]
	return time.Since(last) >= minInterval
}

func (c *TiingoCollector) markFetched(sym, tf string) {
	c.mu.Lock()
	c.lastFetched[sym+":"+tf] = time.Now()
	c.mu.Unlock()
	c.saveState()
}

func (c *TiingoCollector) loadState() {
	if c.stateFile == "" {
		return
	}
	data, err := os.ReadFile(c.stateFile)
	if err != nil {
		return // ignore missing file (first run)
	}
	var s tiingoState
	if err := json.Unmarshal(data, &s); err != nil {
		log.Warn().Err(err).Msg("[Tiingo] state file parse failed — resetting")
		return
	}
	c.mu.Lock()
	c.lastFetched = s.LastFetched
	c.rateLimited = s.RateLimited
	c.mu.Unlock()
	if time.Now().Before(s.RateLimited) {
		log.Info().Time("resume_at", s.RateLimited).Msg("[Tiingo] previous rate limit restored — waiting after restart")
	}
	log.Info().Int("entries", len(s.LastFetched)).Msg("[Tiingo] fetch state restored")
}

func (c *TiingoCollector) saveState() {
	if c.stateFile == "" {
		return
	}
	c.mu.Lock()
	s := tiingoState{
		LastFetched: c.lastFetched,
		RateLimited: c.rateLimited,
	}
	c.mu.Unlock()
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := os.WriteFile(c.stateFile, data, 0644); err != nil {
		log.Warn().Err(err).Msg("[Tiingo] state file save failed")
	}
}

func (c *TiingoCollector) fetchOHLCV(symbol, tf string) ([]models.OHLCV, error) {
	switch tf {
	case "1D", "1W":
		return c.fetchDailyOHLCV(symbol, tf)
	case "1H", "4H":
		return c.fetchIntradayOHLCV(symbol, tf)
	default:
		return nil, fmt.Errorf("unsupported timeframe: %s", tf)
	}
}

// fetchDailyOHLCV uses /tiingo/daily/{symbol}/prices (free on all plans).
func (c *TiingoCollector) fetchDailyOHLCV(symbol, tf string) ([]models.OHLCV, error) {
	startDate := time.Now().AddDate(-2, 0, 0).Format("2006-01-02")
	url := fmt.Sprintf(
		"%s/tiingo/daily/%s/prices?startDate=%s&token=%s",
		tiingoBaseURL, strings.ToLower(symbol), startDate, c.apiKey,
	)
	body, err := c.doGet(url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return parseTiingoDailyResponse(symbol, tf, body)
}

// fetchIntradayOHLCV uses /iex/{symbol}/prices (free, recent ~30 days at 1-hour resolution).
func (c *TiingoCollector) fetchIntradayOHLCV(symbol, tf string) ([]models.OHLCV, error) {
	startDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	url := fmt.Sprintf(
		"%s/iex/%s/prices?startDate=%s&resampleFreq=1hour&token=%s",
		tiingoBaseURL, strings.ToLower(symbol), startDate, c.apiKey,
	)
	body, err := c.doGet(url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return parseTiingoIntradayResponse(symbol, tf, body)
}

// errRateLimit is a sentinel for HTTP 429 responses.
type errRateLimit struct{ msg string }

func (e *errRateLimit) Error() string { return e.msg }

func is429(err error) bool {
	_, ok := err.(*errRateLimit)
	return ok
}

func (c *TiingoCollector) doGet(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		return nil, &errRateLimit{msg: string(b)}
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return resp.Body, nil
}

// ── response types ────────────────────────────────────────────────────────────

type tiingoDailyBar struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type tiingoIntradayBar struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// ── parsers ───────────────────────────────────────────────────────────────────

func parseTiingoDailyResponse(symbol, tf string, body io.Reader) ([]models.OHLCV, error) {
	var raw []tiingoDailyBar
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}
	sym := strings.ToUpper(symbol)
	bars := make([]models.OHLCV, 0, len(raw))
	for _, b := range raw {
		t, err := time.Parse(time.RFC3339, b.Date)
		if err != nil {
			continue
		}
		bars = append(bars, models.OHLCV{
			Symbol: sym, Timeframe: "1D", OpenTime: t.UTC(),
			Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	if tf == "1W" {
		return aggregateBars(sym, "1W", bars, 7*24*time.Hour), nil
	}
	return bars, nil
}

func parseTiingoIntradayResponse(symbol, tf string, body io.Reader) ([]models.OHLCV, error) {
	var raw []tiingoIntradayBar
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}
	sym := strings.ToUpper(symbol)
	bars := make([]models.OHLCV, 0, len(raw))
	for _, b := range raw {
		t, err := time.Parse(time.RFC3339, b.Date)
		if err != nil {
			continue
		}
		bars = append(bars, models.OHLCV{
			Symbol: sym, Timeframe: "1H", OpenTime: t.UTC(),
			Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	if tf == "4H" {
		rebuilt := RebuildHigherTF(sym, bars)
		return rebuilt["4H"], nil
	}
	return bars, nil
}
