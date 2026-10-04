package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
	"github.com/rs/zerolog/log"
)

// YahooFXSymbol maps a canonical pair to a Yahoo instrument. Metals are
// futures proxies and must be labelled as such by the UI and notifier.
func YahooFXSymbol(pair string) (string, bool) {
	switch pair {
	case "XAUUSD":
		return "GC=F", true
	case "XAGUSD":
		return "SI=F", true
	default:
		return pair + "=X", false
	}
}

// NY4HBucket is aligned to the NY 17:00 trading-day boundary. The UTC offset
// follows America/New_York DST, unlike time.Time.Truncate(4*time.Hour).
func NY4HBucket(t time.Time) time.Time {
	ny, _ := time.LoadLocation("America/New_York")
	local := t.In(ny)
	start := time.Date(local.Year(), local.Month(), local.Day(), 17, 0, 0, 0, ny)
	if local.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start.Add(time.Duration(int(t.Sub(start)/(4*time.Hour))) * 4 * time.Hour).UTC()
}

// RebuildFX4H emits only complete, consecutive four-hour buckets. Yahoo's
// native daily/weekly candles are retained as supplied by Yahoo.
func RebuildFX4H(symbol string, hourly []models.OHLCV) []models.OHLCV {
	sort.Slice(hourly, func(i, j int) bool { return hourly[i].OpenTime.Before(hourly[j].OpenTime) })
	var out []models.OHLCV
	for i := 0; i < len(hourly); {
		bucket := NY4HBucket(hourly[i].OpenTime)
		j := i
		for j < len(hourly) && NY4HBucket(hourly[j].OpenTime).Equal(bucket) {
			j++
		}
		complete := j-i == 4
		if complete {
			for k := 0; k < 4; k++ {
				if !hourly[i+k].OpenTime.Equal(bucket.Add(time.Duration(k) * time.Hour)) {
					complete = false
					break
				}
			}
		}
		if complete {
			bar := models.OHLCV{Symbol: symbol, Timeframe: "4H", OpenTime: bucket, Open: hourly[i].Open, High: hourly[i].High, Low: hourly[i].Low, Close: hourly[j-1].Close}
			for k := i; k < j; k++ {
				if hourly[k].High > bar.High {
					bar.High = hourly[k].High
				}
				if hourly[k].Low < bar.Low {
					bar.Low = hourly[k].Low
				}
				bar.Volume += hourly[k].Volume
			}
			out = append(out, bar)
		}
		i = j
	}
	return out
}

type YahooFXCollector struct {
	db         *storage.DB
	symbols    []config.SymbolEntry
	timeframes []string
	interval   time.Duration
	client     *http.Client
	baseURL    string
	onSuccess  func()
	onError    func(error)
	readiness  forexReadiness
	onStatus   func(map[string]string)
	now        func() time.Time
}

func (c *YahooFXCollector) OnSuccess(f func())                 { c.onSuccess = f }
func (c *YahooFXCollector) SetBaseURL(url string)              { c.baseURL = url }
func (c *YahooFXCollector) OnError(f func(error))              { c.onError = f }
func (c *YahooFXCollector) OnStatus(f func(map[string]string)) { c.onStatus = f }
func (c *YahooFXCollector) Ready(symbol string) bool           { return c.readiness.Ready(symbol) }
func (c *YahooFXCollector) Issues() map[string]string          { return c.readiness.Issues() }

func NewYahooFXCollector(db *storage.DB, symbols []config.SymbolEntry, timeframes []string, interval time.Duration) *YahooFXCollector {
	return &YahooFXCollector{db: db, symbols: symbols, timeframes: timeframes, interval: interval, client: &http.Client{Timeout: 15 * time.Second}, baseURL: "https://query1.finance.yahoo.com", now: time.Now}
}

func (c *YahooFXCollector) Start(ctx context.Context) {
	if c.interval <= 0 {
		c.interval = time.Minute
	}
	c.client = generationClient(c.client, ctx)
	c.poll(ctx, true)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.poll(ctx, false)
		}
	}
}

func (c *YahooFXCollector) poll(ctx context.Context, initial bool) {
	now := c.now()
	saved := false
	issues := c.readiness.Issues()
	if issues == nil {
		issues = map[string]string{}
		for _, entry := range c.symbols {
			if entry.Enabled {
				for _, tf := range c.timeframes {
					issues[entry.Symbol+"/"+tf] = "pending"
				}
			}
		}
	}
	for _, entry := range c.symbols {
		if !entry.Enabled {
			continue
		}
		for _, tf := range c.timeframes {
			if ctx.Err() != nil {
				return
			}
			if !initial && !market.IsForexOpen(now) && tf != "1D" && tf != "1W" {
				continue
			}
			key := entry.Symbol + "/" + tf
			bars, err := c.Fetch(ctx, entry, tf)
			if err != nil {
				log.Error().Err(err).Str("symbol", entry.Symbol).Str("tf", tf).Msg("Yahoo FX fetch failed")
				issues[key] = err.Error()
				continue
			}
			bars = completeFXBars(bars, tf, now)
			if len(bars) == 0 {
				issues[key] = "no fresh complete candles"
				continue
			}
			if err = c.db.SaveOHLCVBatch(bars, "yahoo_fx"); err != nil {
				log.Error().Err(err).Msg("Yahoo FX save failed")
				issues[key] = err.Error()
			} else {
				saved = true
				if seriesFresh(bars, tf, now) {
					delete(issues, key)
				} else {
					issues[key] = "no fresh complete candles"
				}
			}
		}
	}
	c.readiness.set(issues)
	if c.onStatus != nil {
		c.onStatus(c.Issues())
	}
	if err := issuesError(issues); err != nil && c.onError != nil {
		c.onError(err)
	}
	if saved && len(issues) == 0 && c.onSuccess != nil {
		c.onSuccess()
	}
}

func (c *YahooFXCollector) Fetch(ctx context.Context, entry config.SymbolEntry, tf string) ([]models.OHLCV, error) {
	params, ok := yahooTFParams[tf]
	if !ok {
		return nil, fmt.Errorf("unsupported timeframe %s", tf)
	}
	providerSymbol, _ := YahooFXSymbol(entry.Symbol)
	if entry.ProviderSymbol != "" {
		providerSymbol = entry.ProviderSymbol
	}
	u := fmt.Sprintf("%s/v8/finance/chart/%s?interval=%s&range=%s", strings.TrimRight(c.baseURL, "/"), url.PathEscape(providerSymbol), params[0], params[1])
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Yahoo FX HTTP %d", resp.StatusCode)
	}
	// Parse as 1H for 4H so the generic UTC reconstruction is bypassed.
	parseTF := tf
	if tf == "4H" {
		parseTF = "1H"
	}
	bars, err := parseYahooResponse(entry.Symbol, parseTF, resp.Body)
	if err != nil {
		return nil, err
	}
	for i := range bars {
		bars[i].Volume = 0
		bars[i].Symbol = entry.Symbol
	}
	if tf == "4H" {
		bars = RebuildFX4H(entry.Symbol, bars)
	}
	return completeFXBars(bars, tf, c.now()), nil
}
