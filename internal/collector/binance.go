package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

const (
	binanceWSBase    = "wss://stream.binance.com:9443/stream"
	binanceReconnect = 5 * time.Second
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
// Automatically reconnects on disconnect.
func (c *BinanceCollector) Start(ctx context.Context) {
	streamURL := c.buildStreamURL()
	log.Info().
		Str("url", streamURL).
		Strs("symbols", c.symbols).
		Msg("[Binance] 수집기 시작")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("[Binance] 수집기 종료")
			return
		default:
			if err := c.connect(ctx, streamURL); err != nil {
				log.Error().Err(err).
					Dur("retry_in", binanceReconnect).
					Msg("[Binance] 연결 오류, 재접속 대기 중")
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
		return fmt.Errorf("WebSocket 연결 실패: %w", err)
	}
	defer conn.Close()

	log.Info().Msg("[Binance] WebSocket 연결됨")

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
				return fmt.Errorf("메시지 수신 오류: %w", err)
			}
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			if err := c.handleMessage(msg); err != nil {
				log.Warn().Err(err).Msg("[Binance] 메시지 처리 오류")
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
	IsClosed  bool    `json:"x"` // true = 캔들 확정
}

func (c *BinanceCollector) handleMessage(raw []byte) error {
	var msg binanceStreamMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}

	k := msg.Data.Kline
	// 확정된 캔들만 저장
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
		Msg("[Binance] 캔들 저장")

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
