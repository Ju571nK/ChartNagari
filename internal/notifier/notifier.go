package notifier

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// Sender is the interface that any notification backend must implement.
type Sender interface {
	// Send dispatches a single signal. Returns an error on transport/API failure.
	Send(ctx context.Context, sig models.Signal) error
	// Name returns a human-readable backend identifier used in log messages.
	Name() string
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
	cfg      Config
	cooldown *Cooldown
	senders  []Sender
	log      zerolog.Logger
}

// New creates a Notifier from the given config and logger.
func New(cfg Config, log zerolog.Logger) *Notifier {
	return &Notifier{
		cfg:      cfg,
		cooldown: NewCooldown(cfg.CooldownDur),
		log:      log,
	}
}

// Register adds a sender backend. Call before the first Notify.
func (n *Notifier) Register(s Sender) {
	n.senders = append(n.senders, s)
}

// Announce sends a raw HTML text message to all senders that implement TextSender.
// Bypasses score threshold and cooldown — intended for system-level notices only.
func (n *Notifier) Announce(ctx context.Context, text string) {
	for _, s := range n.senders {
		if ts, ok := s.(TextSender); ok {
			if err := ts.SendText(ctx, text); err != nil {
				n.log.Error().Err(err).Str("sender", s.Name()).Msg("시스템 알림 발송 실패")
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
	for _, sig := range signals {
		if sig.Score < n.cfg.ScoreThreshold {
			n.log.Debug().
				Str("symbol", sig.Symbol).
				Str("rule", sig.Rule).
				Float64("score", sig.Score).
				Float64("threshold", n.cfg.ScoreThreshold).
				Msg("신호 스코어 미달 — 스킵")
			continue
		}

		if !n.cooldown.Allow(sig.Symbol, sig.Rule) {
			n.log.Debug().
				Str("symbol", sig.Symbol).
				Str("rule", sig.Rule).
				Msg("알림 쿨다운 활성 — 스킵")
			continue
		}

		n.log.Info().
			Str("symbol", sig.Symbol).
			Str("rule", sig.Rule).
			Str("direction", sig.Direction).
			Float64("score", sig.Score).
			Msg("신호 발송")

		for _, sender := range n.senders {
			if err := sender.Send(ctx, sig); err != nil {
				n.log.Error().
					Err(err).
					Str("sender", sender.Name()).
					Str("symbol", sig.Symbol).
					Msg("알림 발송 실패")
			}
		}
	}
}
