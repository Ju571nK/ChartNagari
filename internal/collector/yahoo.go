package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

const (
	// Yahoo Finance v8 chart API — 공개 엔드포인트 (인증 불필요)
	yahooChartURL = "https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&range=%s"

	// 장중 시간 (NYSE 기준, UTC)
	marketOpenUTC  = 14*60 + 30 // 14:30 UTC = 09:30 EST
	marketCloseUTC = 21 * 60    // 21:00 UTC = 16:00 EST
)

// YahooCollector polls Yahoo Finance REST API for stock OHLCV data.
type YahooCollector struct {
	db           *storage.DB
	symbols      []string
	timeframes   []string
	pollInterval time.Duration
	httpClient   *http.Client
}

// NewYahooCollector creates a new YahooCollector.
func NewYahooCollector(db *storage.DB, symbols, timeframes []string, pollInterval time.Duration) *YahooCollector {
	return &YahooCollector{
		db:           db,
		symbols:      symbols,
		timeframes:   timeframes,
		pollInterval: pollInterval,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Start begins polling in a ticker loop. Blocks until ctx is cancelled.
func (c *YahooCollector) Start(ctx context.Context) {
	log.Info().
		Strs("symbols", c.symbols).
		Dur("poll_interval", c.pollInterval).
		Msg("[Yahoo] 수집기 시작")

	// 최초 즉시 실행
	c.fetchAll()

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("[Yahoo] 수집기 종료")
			return
		case <-ticker.C:
			// 장외 시간이면 1D/1W만 폴링
			if !isMarketOpen() {
				log.Debug().Msg("[Yahoo] 장외 시간 — 일봉/주봉만 폴링")
				c.fetchForTimeframes([]string{"1D", "1W"})
				continue
			}
			c.fetchAll()
		}
	}
}

func (c *YahooCollector) fetchAll() {
	c.fetchForTimeframes(c.timeframes)
}

func (c *YahooCollector) fetchForTimeframes(timeframes []string) {
	for _, sym := range c.symbols {
		for _, tf := range timeframes {
			bars, err := c.fetchOHLCV(sym, tf)
			if err != nil {
				log.Error().Err(err).
					Str("symbol", sym).
					Str("tf", tf).
					Msg("[Yahoo] 데이터 수집 실패")
				continue
			}
			if err := c.db.SaveOHLCVBatch(bars, "yahoo"); err != nil {
				log.Error().Err(err).Msg("[Yahoo] 저장 실패")
				continue
			}
			log.Debug().
				Str("symbol", sym).
				Str("tf", tf).
				Int("bars", len(bars)).
				Msg("[Yahoo] 데이터 저장 완료")
		}
	}
}

// yahooTFParams maps our TF keys to Yahoo interval + range query params.
var yahooTFParams = map[string][2]string{
	"1H": {"1h", "5d"},   // 최근 5일 1시간봉
	"4H": {"1h", "30d"},  // 최근 30일 → 4H로 재구성
	"1D": {"1d", "60d"},  // 최근 60일 일봉
	"1W": {"1wk", "2y"},  // 최근 2년 주봉
}

func (c *YahooCollector) fetchOHLCV(symbol, tf string) ([]models.OHLCV, error) {
	params, ok := yahooTFParams[tf]
	if !ok {
		return nil, fmt.Errorf("지원하지 않는 타임프레임: %s", tf)
	}

	url := fmt.Sprintf(yahooChartURL, symbol, params[0], params[1])
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Yahoo는 브라우저 User-Agent를 선호
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return parseYahooResponse(symbol, tf, resp.Body)
}

// yahooChartResponse represents the Yahoo Finance v8 chart API response.
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Timestamps []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []float64 `json:"open"`
					High   []float64 `json:"high"`
					Low    []float64 `json:"low"`
					Close  []float64 `json:"close"`
					Volume []float64 `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func parseYahooResponse(symbol, tf string, body io.Reader) ([]models.OHLCV, error) {
	var raw yahooChartResponse
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("JSON 파싱 실패: %w", err)
	}

	if raw.Chart.Error != nil {
		return nil, fmt.Errorf("Yahoo API 오류 [%s]: %s",
			raw.Chart.Error.Code, raw.Chart.Error.Description)
	}

	if len(raw.Chart.Result) == 0 {
		return nil, fmt.Errorf("결과 없음: %s %s", symbol, tf)
	}

	res := raw.Chart.Result[0]
	if len(res.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("Quote 데이터 없음: %s", symbol)
	}

	quote := res.Indicators.Quote[0]
	n := len(res.Timestamps)

	// 4H는 1H 데이터를 재구성
	internalTF := tf
	if tf == "4H" {
		internalTF = "1H" // 우선 1H로 저장 후 재구성
	}

	var bars []models.OHLCV
	for i := 0; i < n; i++ {
		if i >= len(quote.Close) || quote.Close[i] == 0 {
			continue // 결측값 스킵
		}
		bars = append(bars, models.OHLCV{
			Symbol:    strings.ToUpper(symbol),
			Timeframe: internalTF,
			OpenTime:  time.Unix(res.Timestamps[i], 0).UTC(),
			Open:      safeAt(quote.Open, i),
			High:      safeAt(quote.High, i),
			Low:       safeAt(quote.Low, i),
			Close:     safeAt(quote.Close, i),
			Volume:    safeAt(quote.Volume, i),
		})
	}

	// 4H 재구성
	if tf == "4H" && len(bars) > 0 {
		rebuilt := RebuildHigherTF(symbol, bars)
		return rebuilt["4H"], nil
	}

	return bars, nil
}

// isMarketOpen returns true if the current UTC time is within NYSE trading hours (Mon–Fri).
func isMarketOpen() bool {
	now := time.Now().UTC()
	wd := now.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	minutesUTC := now.Hour()*60 + now.Minute()
	return minutesUTC >= marketOpenUTC && minutesUTC < marketCloseUTC
}

func safeAt(s []float64, i int) float64 {
	if i < len(s) {
		return s[i]
	}
	return 0
}
