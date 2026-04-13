package alpaca

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// MapTradeSignalToOrder converts a TradeSignal into an Alpaca OrderRequest
// using the operator-configured notional-per-trade for qty sizing.
//
// Direction semantics (Phase 3 — domain analyst decisions):
//
//	LONG  → buy  (market)  — open a new long
//	SHORT → sell (market)  — Alpaca requires the equity to be shortable; if it
//	                         is not, Alpaca rejects with 4xx and we return 422.
//
// Qty sizing:
//
//	notional = cfg.NotionalPerTrade (USD)
//	qty      = floor(notional / entry_price)
//	qty must be >= 1 — smaller notionals are rejected with a clear error so
//	the operator sees it in the feedback status.
//
// Asset class:
//
//	Only stocks are supported in v1. Crypto signals (asset_class = "crypto") are
//	rejected with an explicit error so the operator sees a clear REJECTED
//	feedback status instead of a mysterious Alpaca 4xx.
func MapTradeSignalToOrder(sig models.TradeSignal, notional float64) (OrderRequest, error) {
	if strings.TrimSpace(sig.Symbol) == "" {
		return OrderRequest{}, errors.New("mapper: empty symbol")
	}
	if strings.EqualFold(sig.AssetClass, "crypto") {
		return OrderRequest{}, fmt.Errorf("mapper: crypto asset_class not supported in Phase 3 (symbol=%s)", sig.Symbol)
	}
	if sig.EntryPrice <= 0 {
		return OrderRequest{}, fmt.Errorf("mapper: invalid entry_price %.6f", sig.EntryPrice)
	}
	if notional <= 0 {
		return OrderRequest{}, errors.New("mapper: notional must be > 0")
	}
	side, err := directionToSide(sig.Direction)
	if err != nil {
		return OrderRequest{}, err
	}
	qty := int64(math.Floor(notional / sig.EntryPrice))
	if qty < 1 {
		return OrderRequest{}, fmt.Errorf("mapper: computed qty < 1 for notional=%.2f entry_price=%.4f (raise ALPACA_NOTIONAL_PER_TRADE)", notional, sig.EntryPrice)
	}

	return OrderRequest{
		Symbol:        strings.ToUpper(strings.TrimSpace(sig.Symbol)),
		Qty:           strconv.FormatInt(qty, 10),
		Side:          side,
		Type:          "market",
		TimeInForce:   "day",
		ClientOrderID: sig.ID, // = TradeSignal.ID (uuid) — survives in Alpaca for manual reconciliation
	}, nil
}

// directionToSide maps TradeSignal.Direction to the Alpaca side value.
// Alpaca uses `"buy"` / `"sell"` — the exchange decides whether the sell opens
// a short based on account position / shortable status.
func directionToSide(dir string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(dir)) {
	case "LONG":
		return "buy", nil
	case "SHORT":
		return "sell", nil
	default:
		return "", fmt.Errorf("mapper: unsupported direction %q", dir)
	}
}
