package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// TradeSignalVersion is the envelope schema version sent to execution plugins.
const TradeSignalVersion = "1.0"

// TradeSignal is the webhook payload delivered to external execution plugins.
// It is an envelope wrapped around an internal Signal: id/timestamp/version and
// asset_class/exchange are created at dispatch time, not stored on Signal.
type TradeSignal struct {
	ID         string    `json:"id"`
	Version    string    `json:"version"`
	Timestamp  time.Time `json:"timestamp"`
	Symbol     string    `json:"symbol"`
	Direction  string    `json:"direction"`
	Timeframe  string    `json:"timeframe"`
	Rule       string    `json:"rule"`
	EntryPrice float64   `json:"entry_price"`
	TakeProfit float64   `json:"take_profit"`
	StopLoss   float64   `json:"stop_loss"`
	Score      float64   `json:"score"`

	HTFTrend         string  `json:"htf_trend,omitempty"`
	ATRPercentile    float64 `json:"atr_percentile,omitempty"`
	AssetClass       string  `json:"asset_class"`
	Exchange         string  `json:"exchange"`
	AIInterpretation string  `json:"ai_interpretation,omitempty"`
}

// OrderFeedback is the callback posted back by a plugin after order lifecycle events.
type OrderFeedback struct {
	SignalID    string    `json:"signal_id"`
	PluginName  string    `json:"plugin_name"`
	Status      string    `json:"status"`
	OrderID     string    `json:"order_id,omitempty"`
	Symbol      string    `json:"symbol,omitempty"`    // echoed by plugin for UI enrichment
	FilledQty   float64   `json:"filled_qty,omitempty"`
	FilledPrice float64   `json:"filled_price,omitempty"`
	Message     string    `json:"message,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// OrderFeedback status enum.
const (
	OrderStatusReceived    = "RECEIVED"
	OrderStatusSubmitted   = "SUBMITTED"
	OrderStatusFilled      = "FILLED"
	OrderStatusPartialFill = "PARTIAL_FILL"
	OrderStatusRejected    = "REJECTED"
	OrderStatusCancelled   = "CANCELLED"
	OrderStatusError       = "ERROR"
)

// ToTradeSignal wraps an internal Signal into an outbound TradeSignal envelope.
// id (uuid v4), timestamp (UTC now), version, and asset_class/exchange are
// injected here — the Signal struct itself does not carry these.
func ToTradeSignal(s Signal) TradeSignal {
	ac, ex := InferAssetClassAndExchange(s.Symbol)
	return TradeSignal{
		ID:               uuid.NewString(),
		Version:          TradeSignalVersion,
		Timestamp:        time.Now().UTC(),
		Symbol:           s.Symbol,
		Direction:        s.Direction,
		Timeframe:        s.Timeframe,
		Rule:             s.Rule,
		EntryPrice:       s.EntryPrice,
		TakeProfit:       s.TP,
		StopLoss:         s.SL,
		Score:            s.Score,
		HTFTrend:         s.HTFTrend,
		ATRPercentile:    s.ATRPercentile,
		AssetClass:       ac,
		Exchange:         ex,
		AIInterpretation: s.AIInterpretation,
	}
}

// InferAssetClassAndExchange guesses asset_class + default exchange from symbol.
// Heuristic: symbols ending in USDT/USDC/BUSD/BTC/ETH are treated as crypto on
// Binance; everything else defaults to stock on nasdaq. Plugins remap via
// symbol_map in execution.yaml when they need a different exchange.
func InferAssetClassAndExchange(symbol string) (assetClass, exchange string) {
	up := strings.ToUpper(symbol)
	for _, q := range []string{"USDT", "USDC", "BUSD"} {
		if strings.HasSuffix(up, q) && len(up) > len(q) {
			return "crypto", "binance"
		}
	}
	if strings.HasSuffix(up, "BTC") && len(up) > 3 && up != "BTC" {
		return "crypto", "binance"
	}
	if strings.HasSuffix(up, "ETH") && len(up) > 3 && up != "ETH" {
		return "crypto", "binance"
	}
	return "stock", "nasdaq"
}
