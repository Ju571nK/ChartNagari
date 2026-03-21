// Package pricealert checks user-defined price targets against live market prices
// and dispatches notifications when targets are hit.
package pricealert

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
)

// Store is the subset of storage.DB used by the Watcher.
type Store interface {
	GetActivePriceAlerts() ([]storage.PriceAlert, error)
	MarkAlertTriggered(id int64, at time.Time) error
}

// Announcer sends a plain-text notification message.
// *notifier.Notifier satisfies this via its Announce method.
type Announcer interface {
	Announce(ctx context.Context, text string)
}

// Watcher checks active price alerts against the current market price for a symbol.
// It is safe for concurrent use.
type Watcher struct {
	store     Store
	announcer Announcer
	log       zerolog.Logger
}

// New creates a Watcher.
func New(store Store, announcer Announcer, log zerolog.Logger) *Watcher {
	return &Watcher{store: store, announcer: announcer, log: log}
}

// CheckSymbol evaluates all active alerts for the given symbol against currentPrice.
// Matching alerts are marked triggered and a notification is sent.
// Call this once per pipeline tick per symbol, after loading the latest OHLCV close.
func (w *Watcher) CheckSymbol(ctx context.Context, symbol string, currentPrice float64) {
	alerts, err := w.store.GetActivePriceAlerts()
	if err != nil {
		w.log.Error().Err(err).Msg("price alert: failed to load active alerts")
		return
	}

	now := time.Now()
	for _, a := range alerts {
		if a.Symbol != symbol {
			continue
		}
		if !isTriggered(a.Condition, currentPrice, a.Target) {
			continue
		}

		// Mark triggered first to avoid duplicate fires on rapid ticks.
		if err := w.store.MarkAlertTriggered(a.ID, now); err != nil {
			w.log.Error().Err(err).Int64("id", a.ID).Msg("price alert: failed to mark triggered")
			continue
		}

		condLabel := "초과"
		if a.Condition == "below" {
			condLabel = "하향"
		}
		msg := fmt.Sprintf(
			"🎯 가격 목표 도달\n%s %.4g %s\n현재가: %.4g",
			symbol, a.Target, condLabel, currentPrice,
		)
		if a.Note != "" {
			msg += "\n메모: " + a.Note
		}
		w.announcer.Announce(ctx, msg)
		w.log.Info().
			Str("symbol", symbol).
			Float64("target", a.Target).
			Str("condition", a.Condition).
			Float64("price", currentPrice).
			Msg("price alert triggered")
	}
}

// isTriggered returns true when currentPrice meets the alert condition.
func isTriggered(condition string, current, target float64) bool {
	switch condition {
	case "above":
		return current >= target
	case "below":
		return current <= target
	default:
		return false
	}
}
