// Package pipeline — forward return tracking for signal performance measurement.
package pipeline

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ForwardReturnDB is the storage interface for forward return tracking.
type ForwardReturnDB interface {
	GetSignalsNeedingForwardReturn(minAgeDays int) ([]storage.SignalForForwardReturn, error)
	UpdateForwardReturns(signalID int64, r5, r10, r20, r40 float64) error
}

// ForwardReturnOHLCVReader reads OHLCV bars for forward return calculation.
type ForwardReturnOHLCVReader interface {
	GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
}

// forwardReturnPeriods defines the day offsets we track.
var forwardReturnPeriods = []int{5, 10, 20, 40}

// UpdateForwardReturns checks signals older than 5 days and fills in forward returns.
// Called periodically (e.g., once per pipeline tick).
// It gracefully handles partial data: if only 5d data is available, it fills 5d and
// leaves the rest at 0 for future updates.
func UpdateForwardReturns(frDB ForwardReturnDB, ohlcv ForwardReturnOHLCVReader, log zerolog.Logger) {
	sigs, err := frDB.GetSignalsNeedingForwardReturn(5)
	if err != nil {
		log.Warn().Err(err).Msg("forward return: failed to query signals")
		return
	}
	if len(sigs) == 0 {
		return
	}

	for _, sig := range sigs {
		if sig.EntryPrice <= 0 {
			continue // no entry price available; skip
		}

		// Load 1D bars for the symbol to find closes after N days.
		bars, err := ohlcv.GetOHLCV(sig.Symbol, "1D", 200)
		if err != nil {
			log.Debug().Err(err).Str("symbol", sig.Symbol).Msg("forward return: OHLCV load failed")
			continue
		}
		if len(bars) == 0 {
			continue
		}

		// bars are in DESC order (newest first). We need to find bars after the signal date.
		// Convert to a map of day offsets from signal creation.
		signalDate := sig.CreatedAt

		r5 := sig.FR5d
		r10 := sig.FR10d
		r20 := sig.FR20d
		r40 := sig.FR40d
		updated := false

		for _, period := range forwardReturnPeriods {
			var currentVal *float64
			switch period {
			case 5:
				currentVal = &r5
			case 10:
				currentVal = &r10
			case 20:
				currentVal = &r20
			case 40:
				currentVal = &r40
			}

			// Already computed
			if *currentVal != 0 {
				continue
			}

			// Check if enough time has passed
			targetDate := signalDate.Add(time.Duration(period) * 24 * time.Hour)
			if time.Now().Before(targetDate) {
				continue // not enough time elapsed yet
			}

			// Find the closest bar to the target date
			closePrice := findCloseNearDate(bars, targetDate)
			if closePrice <= 0 {
				continue // data not available
			}

			*currentVal = (closePrice - sig.EntryPrice) / sig.EntryPrice * 100
			updated = true
		}

		if !updated {
			continue
		}

		if err := frDB.UpdateForwardReturns(sig.ID, r5, r10, r20, r40); err != nil {
			log.Warn().Err(err).Int64("signal_id", sig.ID).Msg("forward return: update failed")
		} else {
			log.Debug().
				Int64("signal_id", sig.ID).
				Str("symbol", sig.Symbol).
				Float64("r5d", r5).Float64("r10d", r10).
				Float64("r20d", r20).Float64("r40d", r40).
				Msg("forward return updated")
		}
	}
}

// findCloseNearDate finds the close price of the bar closest to the target date.
// Bars are expected in DESC order (newest first).
// Returns 0 if no suitable bar is found within a 3-day tolerance window.
func findCloseNearDate(bars []models.OHLCV, target time.Time) float64 {
	const toleranceDays = 3
	bestClose := 0.0
	bestDelta := time.Duration(toleranceDays+1) * 24 * time.Hour

	for _, b := range bars {
		delta := target.Sub(b.OpenTime)
		if delta < 0 {
			delta = -delta
		}
		if delta < bestDelta {
			bestDelta = delta
			bestClose = b.Close
		}
	}

	if bestDelta > time.Duration(toleranceDays)*24*time.Hour {
		return 0 // no bar within tolerance
	}
	return bestClose
}
