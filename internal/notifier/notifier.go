package notifier

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// Sender is the interface that any notification backend must implement.
type Sender interface {
	// Send dispatches a single signal. Returns an error on transport/API failure.
	Send(ctx context.Context, sig models.Signal) error
	// Name returns a human-readable backend identifier used in log messages.
	Name() string
}

// AlertSender is an optional extension to Sender for backends that can return
// a platform message_id after dispatch (e.g. TelegramSender.SendAlert).
// Notifier uses this to capture the id for later editMessageReplyMarkup calls.
type AlertSender interface {
	Sender
	SendAlert(ctx context.Context, sig models.Signal) (int64, error)
}

// MarkStoreSet is the subset of SignalMarkStore the Notifier uses.
// *storage.SignalMarkStore satisfies it. Defined here as a small interface
// to avoid a hard dependency on the storage package from notifier internals.
type MarkStoreSet interface {
	SetMessageID(signalID int64, msgID int64) error
}

// TextSender is an optional extension to Sender for sending raw text messages.
// Backends that implement this receive Announce() calls (e.g. startup notices).
type TextSender interface {
	SendText(ctx context.Context, text string) error
}

// Config holds Notifier tuning parameters.
type Config struct {
	// ScoreThreshold is the minimum signal score required to trigger a notification.
	// Signals below this value are silently dropped.
	// PRD reference: weak≥5, medium≥8, strong≥12.
	ScoreThreshold float64

	// CooldownDur is the minimum time between notifications for the same
	// symbol+rule combination. Default per PRD is 4 hours.
	CooldownDur time.Duration
}

// DefaultConfig returns sane production defaults.
func DefaultConfig() Config {
	return Config{
		ScoreThreshold: 5.0,
		CooldownDur:    4 * time.Hour,
	}
}

// Notifier filters scored signals and dispatches them to all registered senders.
// It is safe for concurrent use.
type Notifier struct {
	cfg           Config
	cooldown      *Cooldown
	dailyLimit    *DailyLimit
	senders       []Sender
	log           zerolog.Logger
	alertHolder   *appconfig.AlertConfigHolder
	profileHolder *appconfig.SymbolProfilesHolder // optional; set via WithProfileHolder
	overrideStore appconfig.OverrideGetter        // optional; set via WithOverrideStore
	markStore     MarkStoreSet                    // optional; nil disables message_id capture
}

// New creates a Notifier from the given config and logger.
func New(cfg Config, log zerolog.Logger) *Notifier {
	return &Notifier{
		cfg:        cfg,
		cooldown:   NewCooldown(cfg.CooldownDur),
		dailyLimit: NewDailyLimit(),
		log:        log,
	}
}

// Register adds a sender backend. Call before the first Notify.
func (n *Notifier) Register(s Sender) {
	n.senders = append(n.senders, s)
}

// SetAlertConfigHolder wires an optional live-updated alert configuration holder.
func (n *Notifier) SetAlertConfigHolder(h *appconfig.AlertConfigHolder) {
	n.alertHolder = h
}

// WithProfileHolder wires an optional symbol profiles holder for per-symbol
// cooldown and daily limit resolution. Returns n for chaining.
func (n *Notifier) WithProfileHolder(h *appconfig.SymbolProfilesHolder) *Notifier {
	n.profileHolder = h
	return n
}

// WithOverrideStore wires an optional per-symbol override store used together
// with profileHolder by EffectiveAlertConfig. Returns n for chaining.
func (n *Notifier) WithOverrideStore(s appconfig.OverrideGetter) *Notifier {
	n.overrideStore = s
	return n
}

// WithMarkStore wires an optional mark store used to persist the Telegram
// message_id after SendAlert. When nil, message_id capture is disabled.
// Returns n for chaining.
func (n *Notifier) WithMarkStore(s MarkStoreSet) *Notifier {
	n.markStore = s
	return n
}

// Announce sends a raw HTML text message to all senders that implement TextSender.
// Bypasses score threshold and cooldown — intended for system-level notices only.
func (n *Notifier) Announce(ctx context.Context, text string) {
	for _, s := range n.senders {
		if ts, ok := s.(TextSender); ok {
			if err := ts.SendText(ctx, text); err != nil {
				n.log.Error().Err(err).Str("sender", s.Name()).Msg("failed to send system announcement")
			}
		}
	}
}

// Notify processes a batch of signals:
//  1. Drops signals whose Score is below ScoreThreshold.
//  2. Skips signals still within the cooldown window.
//  3. Dispatches accepted signals to every registered sender in order.
//     Errors from individual senders are logged but do not abort remaining senders.
func (n *Notifier) Notify(ctx context.Context, signals []models.Signal) {
	threshold := n.cfg.ScoreThreshold
	if n.alertHolder != nil {
		threshold = n.alertHolder.Get().ScoreThreshold
	}

	for _, sig := range signals {
		if sig.Score < threshold {
			n.log.Debug().
				Str("symbol", sig.Symbol).
				Str("rule", sig.Rule).
				Float64("score", sig.Score).
				Float64("threshold", threshold).
				Msg("signal score below threshold — skipping")
			continue
		}

		// Per-symbol effective config (falls through to global when no override / no holder).
		var cooldownDur time.Duration
		var dailyLimit int
		if n.profileHolder != nil {
			eff := appconfig.EffectiveAlertConfig(sig.Symbol, n.profileHolder, n.overrideStore)
			if eff.CooldownHours > 0 {
				cooldownDur = time.Duration(eff.CooldownHours) * time.Hour
			}
			dailyLimit = eff.AlertLimitPerDay
		}

		if !n.cooldown.AllowWithDuration(sig.Symbol, sig.Rule, cooldownDur) {
			n.log.Debug().
				Str("symbol", sig.Symbol).
				Str("rule", sig.Rule).
				Msg("notification cooldown active — skipping")
			continue
		}

		if !n.dailyLimit.Allow(sig.Symbol, dailyLimit) {
			n.log.Debug().
				Str("symbol", sig.Symbol).
				Int("limit", dailyLimit).
				Msg("daily alert limit reached — skipping")
			continue
		}

		n.log.Info().
			Str("symbol", sig.Symbol).
			Str("rule", sig.Rule).
			Str("direction", sig.Direction).
			Float64("score", sig.Score).
			Msg("dispatching signal")

		for _, sender := range n.senders {
			if as, ok := sender.(AlertSender); ok {
				msgID, err := as.SendAlert(ctx, sig)
				if err != nil {
					n.log.Error().
						Err(err).
						Str("sender", sender.Name()).
						Str("symbol", sig.Symbol).
						Msg("failed to send notification")
					continue
				}
				if n.markStore != nil && msgID != 0 {
					if err := n.markStore.SetMessageID(sig.ID, msgID); err != nil {
						n.log.Warn().Err(err).Int64("signal_id", sig.ID).Msg("set message_id failed")
					}
				}
			} else {
				if err := sender.Send(ctx, sig); err != nil {
					n.log.Error().
						Err(err).
						Str("sender", sender.Name()).
						Str("symbol", sig.Symbol).
						Msg("failed to send notification")
				}
			}
		}
	}
}
