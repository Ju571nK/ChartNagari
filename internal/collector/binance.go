package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

const (
	binanceWSBase    = "wss://stream.binance.com:9443/stream"
	binanceRESTBase  = "https://api.binance.com/api/v3/klines"
	binanceReconnect = 5 * time.Second
	binanceHistLimit = 500 // historical bars to pre-fetch on startup
)

// BinanceCollector subscribes to Binance kline WebSocket streams
// for the given symbols and timeframes, storing closed candles to SQLite.
type BinanceCollector struct {
	db         *storage.DB
	symbols    []string
	timeframes []string // e.g. ["1H","4H","1D","1W"]
}

// NewBinanceCollector creates a new BinanceCollector.
func NewBinanceCollector(db *storage.DB, symbols, timeframes []string) *BinanceCollector {
	return &BinanceCollector{
		db:         db,
		symbols:    symbols,
		timeframes: timeframes,
	}
}

// Start begins collecting in a goroutine. Blocks until ctx is cancelled.
// On startup it pre-fetches binanceHistLimit historical bars per symbol×TF via REST,
// then switches to the WebSocket stream for live candles.
func (c *BinanceCollector) Start(ctx context.Context) {
	streamURL := c.buildStreamURL()
	log.Info().
		Str("url", streamURL).
		Strs("symbols", c.symbols).
		Msg("[Binance] collector started")

	// ── 과거 데이터 선행 수집 (REST) ─────────────────────────────────
	c.fetchHistory()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("[Binance] collector stopped")
			return
		default:
			if err := c.connect(ctx, streamURL); err != nil {
				log.Error().Err(err).
					Dur("retry_in", binanceReconnect).
					Msg("[Binance] connection error, waiting to reconnect")
				select {
				case <-ctx.Done():
					return
				case <-time.After(binanceReconnect):
				}
			}
		}
	}
}

// connect establishes a WebSocket connection and processes messages until error or ctx cancel.
func (c *BinanceCollector) connect(ctx context.Context, url string) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}
	defer conn.Close()

	log.Info().Msg("[Binance] WebSocket connected")

	// ping 타임아웃 설정
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// ping goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("message read error: %w", err)
			}
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			if err := c.handleMessage(msg); err != nil {
				log.Warn().Err(err).Msg("[Binance] message processing error")
			}
		}
	}
}

// binanceStreamMsg is the combined stream message format.
type binanceStreamMsg struct {
	Stream string          `json:"stream"`
	Data   binanceKlineMsg `json:"data"`
}

type binanceKlineMsg struct {
	EventType json.RawMessage `json:"e"` // Binance may send string or number (e.g. 24 for kline)
	Kline     binanceKline    `json:"k"`
}

// flexStr unmarshals from JSON string or number (Binance may send OHLCV as either).
type flexStr string

func (s *flexStr) UnmarshalJSON(data []byte) error {
	if len(data) >= 2 && data[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*s = flexStr(str)
		return nil
	}
	*s = flexStr(string(data))
	return nil
}

type binanceKline struct {
	Symbol    string  `json:"s"`
	Interval  string  `json:"i"`
	OpenTime  int64   `json:"t"` // milliseconds
	Open      flexStr `json:"o"`
	High      flexStr `json:"h"`
	Low       flexStr `json:"l"`
	Close     flexStr `json:"c"`
	Volume    flexStr `json:"v"`
	IsClosed  bool    `json:"x"` // true = candle closed
}

func (c *BinanceCollector) handleMessage(raw []byte) error {
	var msg binanceStreamMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}

	k := msg.Data.Kline
	// only save closed candles
	if !k.IsClosed {
		return nil
	}

	tf := binanceIntervalToTF(k.Interval)
	bar := models.OHLCV{
		Symbol:    strings.ToUpper(k.Symbol),
		Timeframe: tf,
		OpenTime:  time.UnixMilli(k.OpenTime).UTC(),
		Open:      parseFloat(string(k.Open)),
		High:      parseFloat(string(k.High)),
		Low:       parseFloat(string(k.Low)),
		Close:     parseFloat(string(k.Close)),
		Volume:    parseFloat(string(k.Volume)),
	}

	if err := c.db.SaveOHLCV(bar, "binance"); err != nil {
		return err
	}

	log.Debug().
		Str("symbol", bar.Symbol).
		Str("tf", bar.Timeframe).
		Time("open_time", bar.OpenTime).
		Float64("close", bar.Close).
		Msg("[Binance] candle saved")

	return nil
}

// buildStreamURL constructs the combined stream URL for all symbols × timeframes.
// e.g. wss://stream.binance.com:9443/stream?streams=btcusdt@kline_1h/btcusdt@kline_4h/...
func (c *BinanceCollector) buildStreamURL() string {
	var streams []string
	for _, sym := range c.symbols {
		lower := strings.ToLower(sym)
		for _, tf := range c.timeframes {
			interval, ok := BinanceTFMap[tf]
			if !ok {
				continue
			}
			streams = append(streams, fmt.Sprintf("%s@kline_%s", lower, interval))
		}
	}
	return fmt.Sprintf("%s?streams=%s", binanceWSBase, strings.Join(streams, "/"))
}

// binanceIntervalToTF converts Binance interval string to our TF key.
func binanceIntervalToTF(interval string) string {
	m := map[string]string{
		"1h": "1H",
		"4h": "4H",
		"1d": "1D",
		"1w": "1W",
	}
	if tf, ok := m[interval]; ok {
		return tf
	}
	return strings.ToUpper(interval)
}

func parseFloat(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

// fetchHistory pre-loads historical OHLCV bars from the Binance REST klines API.
// Called once on Start() so the chart and analysis engine have enough data immediately.
func (c *BinanceCollector) fetchHistory() {
	client := &http.Client{Timeout: 15 * time.Second}
	for _, sym := range c.symbols {
		for _, tf := range c.timeframes {
			interval, ok := BinanceTFMap[tf]
			if !ok {
				continue
			}
			bars, err := fetchBinanceKlines(client, sym, interval, binanceHistLimit)
			if err != nil {
				log.Warn().Err(err).Str("symbol", sym).Str("tf", tf).
					Msg("[Binance] historical data fetch failed — continuing with WebSocket")
				continue
			}
			if err := c.db.SaveOHLCVBatch(bars, "binance"); err != nil {
				log.Warn().Err(err).Msg("[Binance] historical data save failed")
				continue
			}
			log.Debug().Str("symbol", sym).Str("tf", tf).Int("bars", len(bars)).
				Msg("[Binance] historical data saved")
		}
	}
}

// fetchBinanceKlines calls GET /api/v3/klines and returns parsed OHLCV bars.
func fetchBinanceKlines(client *http.Client, symbol, interval string, limit int) ([]models.OHLCV, error) {
	url := fmt.Sprintf("%s?symbol=%s&interval=%s&limit=%d",
		binanceRESTBase, strings.ToUpper(symbol), interval, limit)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("REST request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	// Binance klines: array of arrays
	// [openTime, open, high, low, close, volume, closeTime, ...]
	var raw [][]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}

	tf := binanceIntervalToTF(interval)
	sym := strings.ToUpper(symbol)
	bars := make([]models.OHLCV, 0, len(raw))

	for _, row := range raw {
		if len(row) < 6 {
			continue
		}
		var openTimeMs int64
		if err := json.Unmarshal(row[0], &openTimeMs); err != nil {
			continue
		}
		open := parseRawFloat(row[1])
		high := parseRawFloat(row[2])
		low := parseRawFloat(row[3])
		close_ := parseRawFloat(row[4])
		vol := parseRawFloat(row[5])
		if close_ == 0 {
			continue // skip incomplete candle
		}
		bars = append(bars, models.OHLCV{
			Symbol:    sym,
			Timeframe: tf,
			OpenTime:  time.UnixMilli(openTimeMs).UTC(),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    vol,
		})
	}
	return bars, nil
}

func parseRawFloat(raw json.RawMessage) float64 {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		// might be a number
		var f float64
		json.Unmarshal(raw, &f)
		return f
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
