package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
)

const alertWindow = 35 * time.Minute // check for events in next 35 min, alert at ~30 min before

// AlertStore is the subset of storage.DB used by the Watcher.
type AlertStore interface {
	GetUpcomingAlerts(window time.Duration) ([]storage.EconomicEvent, error)
	MarkEventAlerted(id int64) error
}

// Announcer sends plain text notifications (same interface as notifier.Notifier).
type Announcer interface {
	Announce(ctx context.Context, text string)
}

// Watcher checks for upcoming high-impact events and sends pre-event alerts.
type Watcher struct {
	store     AlertStore
	announcer Announcer
	log       zerolog.Logger
}

// NewWatcher creates a calendar Watcher.
func NewWatcher(store AlertStore, announcer Announcer, log zerolog.Logger) *Watcher {
	return &Watcher{store: store, announcer: announcer, log: log}
}

// Run checks every minute for upcoming events. Blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.check(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) check(ctx context.Context) {
	events, err := w.store.GetUpcomingAlerts(alertWindow)
	if err != nil {
		w.log.Error().Err(err).Msg("calendar watcher: failed to query alerts")
		return
	}

	for _, e := range events {
		minsUntil := int(time.Until(e.EventTime).Minutes())
		if minsUntil < 0 {
			minsUntil = 0
		}

		msg := fmt.Sprintf(
			"⚠️ %d분 후 경제 지표 발표\n🇺🇸 %s · 고영향\n📊 %s",
			minsUntil, e.Country, e.Event,
		)
		if e.Forecast != "" {
			msg += fmt.Sprintf("\n예측: %s%s", e.Forecast, e.Unit)
		}
		if e.Previous != "" {
			msg += fmt.Sprintf(" | 이전: %s%s", e.Previous, e.Unit)
		}

		w.announcer.Announce(ctx, msg)

		if err := w.store.MarkEventAlerted(e.ID); err != nil {
			w.log.Error().Err(err).Int64("id", e.ID).Msg("calendar watcher: mark alerted failed")
		}

		w.log.Info().
			Str("event", e.Event).
			Int("mins_until", minsUntil).
			Msg("calendar: pre-event alert sent")
	}
}
