package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
	"github.com/rs/zerolog/log"
)

var ErrOANDATokenRejected = errors.New("OANDA token rejected")
var ErrOANDARateLimited = errors.New("OANDA rate limited")

var oandaTF = map[string]string{"1H": "H1", "4H": "H4", "1D": "D", "1W": "W"}

type OANDACollector struct {
	db         *storage.DB
	symbols    []config.SymbolEntry
	timeframes []string
	interval   time.Duration
	token      string
	client     *http.Client
	baseURL    string
	onError    func(error)
	onSuccess  func()
	readiness  forexReadiness
	onStatus   func(map[string]string)
	now        func() time.Time
}

func NewOANDACollector(db *storage.DB, symbols []config.SymbolEntry, timeframes []string, interval time.Duration, token, environment string) *OANDACollector {
	base := "https://api-fxpractice.oanda.com"
	if environment == "live" {
		base = "https://api-fxtrade.oanda.com"
	}
	return &OANDACollector{db: db, symbols: symbols, timeframes: timeframes, interval: interval, token: token, client: &http.Client{Timeout: 20 * time.Second}, baseURL: base, now: time.Now}
}

func (c *OANDACollector) OnError(f func(error))              { c.onError = f }
func (c *OANDACollector) OnSuccess(f func())                 { c.onSuccess = f }
func (c *OANDACollector) SetBaseURL(url string)              { c.baseURL = url }
func (c *OANDACollector) OnStatus(f func(map[string]string)) { c.onStatus = f }
func (c *OANDACollector) Ready(symbol string) bool           { return c.readiness.Ready(symbol) }
func (c *OANDACollector) Issues() map[string]string          { return c.readiness.Issues() }

func (c *OANDACollector) Start(ctx context.Context) {
	if c.interval <= 0 {
		c.interval = time.Minute
	}
	c.client = generationClient(c.client, ctx)
	backoff := time.Duration(0)
	initial := true
	for {
		err := c.poll(ctx, initial)
		initial = false
		if err != nil {
			if c.onError != nil {
				c.onError(err)
			}
			log.Error().Err(err).Msg("OANDA FX collection failed")
			if errors.Is(err, ErrOANDATokenRejected) {
				return
			}
			if backoff == 0 {
				backoff = c.interval
			}
			backoff *= 2
			if backoff > 15*time.Minute {
				backoff = 15 * time.Minute
			}
		} else {
			backoff = c.interval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (c *OANDACollector) poll(ctx context.Context, initial bool) error {
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
	var authErr error
	for _, entry := range c.symbols {
		if !entry.Enabled {
			continue
		}
		for _, tf := range c.timeframes {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !initial && !market.IsForexOpen(now) && tf != "1D" && tf != "1W" {
				continue
			}
			key := entry.Symbol + "/" + tf
			bars, err := c.Fetch(ctx, entry, tf)
			if err != nil {
				issues[key] = err.Error()
				if errors.Is(err, ErrOANDATokenRejected) {
					authErr = err
					break
				}
				continue
			}
			bars = completeFXBars(bars, tf, now)
			if len(bars) == 0 {
				issues[key] = "no fresh complete candles"
				continue
			}
			if err = c.db.SaveOHLCVBatch(bars, "oanda"); err != nil {
				issues[key] = err.Error()
				continue
			}
			saved = true
			if seriesFresh(bars, tf, now) {
				delete(issues, key)
			} else {
				issues[key] = "no fresh complete candles"
			}
		}
		if authErr != nil {
			break
		}
	}
	if authErr != nil {
		for _, entry := range c.symbols {
			if entry.Enabled {
				for _, tf := range c.timeframes {
					key := entry.Symbol + "/" + tf
					if _, ok := issues[key]; !ok {
						issues[key] = authErr.Error()
					}
				}
			}
		}
	}
	c.readiness.set(issues)
	if c.onStatus != nil {
		c.onStatus(c.Issues())
	}
	if authErr != nil {
		return fmt.Errorf("%w: %v", authErr, issuesError(issues))
	}
	if err := issuesError(issues); err != nil {
		return err
	}
	if saved && c.onSuccess != nil {
		c.onSuccess()
	}
	return nil
}

func (c *OANDACollector) Fetch(ctx context.Context, entry config.SymbolEntry, tf string) ([]models.OHLCV, error) {
	granularity, ok := oandaTF[tf]
	if !ok {
		return nil, fmt.Errorf("unsupported timeframe %s", tf)
	}
	if err := config.ValidateForexPair(entry.Symbol); err != nil {
		return nil, err
	}
	instrument := entry.ProviderSymbol
	if instrument == "" {
		instrument = entry.Symbol[:3] + "_" + entry.Symbol[3:]
	}
	u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + "/v3/instruments/" + url.PathEscape(instrument) + "/candles")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("granularity", granularity)
	q.Set("price", "M")
	q.Set("count", "500")
	q.Set("dailyAlignment", "17")
	q.Set("alignmentTimezone", "America/New_York")
	q.Set("weeklyAlignment", "Friday")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 401, 403:
		return nil, ErrOANDATokenRejected
	case 429:
		return nil, ErrOANDARateLimited
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OANDA HTTP %d", resp.StatusCode)
	}
	var data struct {
		Candles []struct {
			Complete bool    `json:"complete"`
			Time     string  `json:"time"`
			Volume   float64 `json:"volume"`
			Mid      struct {
				O string `json:"o"`
				H string `json:"h"`
				L string `json:"l"`
				C string `json:"c"`
			} `json:"mid"`
		} `json:"candles"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	out := make([]models.OHLCV, 0, len(data.Candles))
	for _, candle := range data.Candles {
		if !candle.Complete {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, candle.Time)
		if err != nil {
			return nil, err
		}
		prices := []string{candle.Mid.O, candle.Mid.H, candle.Mid.L, candle.Mid.C}
		v := make([]float64, 4)
		for i, p := range prices {
			v[i], err = strconv.ParseFloat(p, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid OANDA candle price: %w", err)
			}
		}
		out = append(out, models.OHLCV{Symbol: entry.Symbol, Timeframe: tf, OpenTime: t.UTC(), Open: v[0], High: v[1], Low: v[2], Close: v[3], Volume: candle.Volume})
	}
	return completeFXBars(out, tf, c.now()), nil
}
