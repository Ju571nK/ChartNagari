// Package alertdrop is the domain model for a signal that did not become an
// alert. It owns the reason vocabulary and the Store contract. It contains no
// SQL and no HTTP.
package alertdrop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// Reason is a closed enum of drop causes. The zero value is invalid so a
// forgotten assignment can never be persisted as a real reason.
type Reason uint8

const (
	invalidReason Reason = iota

	// Pipeline stage (batch filters in analyzeSymbol).
	Profile   // methodology or rule not allowed by profile + override
	Score     // below per-symbol effective score threshold
	Timeframe // timeframe not in effective allowlist
	MTF       // multi-timeframe consensus not met
	HTF       // counter-trend vs higher-timeframe context, hard removal only

	// Notifier stage (per-signal gates in Notify).
	ScoreFloor   // below global notifier threshold
	Cooldown     // symbol+rule notified within cooldown window
	DailyLimit   // per-symbol daily alert cap reached
	DeliveryFail // a sender returned an error (other senders may have succeeded)

	reasonCount // sentinel; keep last
)

// DeliveryFailDetail is the only detail text stored for a sender error.
// Raw errors can contain webhook URLs and bot tokens, so they stay in the Error log.
const DeliveryFailDetail = "delivery failed"

// Stage tells the operator which layer suppressed the signal.
type Stage uint8

const (
	StagePipeline Stage = iota + 1
	StageNotifier
)

// reasonInfo is the single source of truth for every Reason.
type reasonInfo struct {
	code  string
	stage Stage
	knob  string
}

// registry is indexed by Reason. A missing row stays zero and TestRegistryComplete fails.
var registry = [reasonCount]reasonInfo{
	Profile:      {code: "profile", stage: StagePipeline, knob: "symbol_profiles.allowed_rules"},
	Score:        {code: "score", stage: StagePipeline, knob: "override.score_threshold"},
	Timeframe:    {code: "timeframe", stage: StagePipeline, knob: "override.timeframes"},
	MTF:          {code: "mtf", stage: StagePipeline, knob: "alert.mtf_consensus_min"},
	HTF:          {code: "htf", stage: StagePipeline, knob: "tuning.htf_filter"},
	ScoreFloor:   {code: "score_floor", stage: StageNotifier, knob: "alert.score_threshold"},
	Cooldown:     {code: "cooldown", stage: StageNotifier, knob: "override.cooldown_hours"},
	DailyLimit:   {code: "daily_limit", stage: StageNotifier, knob: "override.alert_limit_per_day"},
	DeliveryFail: {code: "delivery_fail", stage: StageNotifier, knob: "notifier credentials"},
}

// Code returns the stable wire code. It panics on an invalid Reason because
// that is a programming error, not runtime input.
func (r Reason) Code() string {
	if !r.Valid() {
		panic(fmt.Sprintf("alertdrop: invalid reason %d", r))
	}
	return registry[r].code
}

// Stage returns the layer that owns this reason.
func (r Reason) Stage() Stage {
	if !r.Valid() {
		return 0
	}
	return registry[r].stage
}

// Knob returns the settings hint for the UI.
func (r Reason) Knob() string {
	if !r.Valid() {
		return ""
	}
	return registry[r].knob
}

// Valid reports whether r is a registered reason.
func (r Reason) Valid() bool {
	return r > invalidReason && r < reasonCount && registry[r].code != ""
}

// MarshalText serializes Reason as its registry code.
func (r Reason) MarshalText() ([]byte, error) {
	if !r.Valid() {
		return nil, fmt.Errorf("invalid alert drop reason %d", r)
	}
	return []byte(registry[r].code), nil
}

// UnmarshalText parses a registry code. Unknown codes return an error.
func (r *Reason) UnmarshalText(b []byte) error {
	code := string(b)
	for _, cand := range Reasons() {
		if registry[cand].code == code {
			*r = cand
			return nil
		}
	}
	return fmt.Errorf("unknown alert drop reason %q", code)
}

// Reasons returns all valid reasons in declaration order.
func Reasons() []Reason {
	out := make([]Reason, 0, int(reasonCount)-1)
	for r := invalidReason + 1; r < reasonCount; r++ {
		out = append(out, r)
	}
	return out
}

// String returns "pipeline" or "notifier".
func (s Stage) String() string {
	switch s {
	case StagePipeline:
		return "pipeline"
	case StageNotifier:
		return "notifier"
	default:
		return ""
	}
}

// Event is one observed drop. SignalID is 0 when the signal was never saved.
//
// Identity for coalescing is (Symbol, Rule, Timeframe, Direction, Reason, Sender).
// Score, Threshold, Detail, and SignalID are the latest observation.
type Event struct {
	Reason    Reason
	Symbol    string
	Rule      string
	Timeframe string
	Direction string
	Score     float64
	Threshold float64
	SignalID  int64
	Sender    string
	Detail    string
	At        time.Time
}

// Of builds an Event from a signal. At is time.Now().UTC().
func Of(r Reason, sig models.Signal) Event {
	return Event{
		Reason:    r,
		Symbol:    sig.Symbol,
		Rule:      sig.Rule,
		Timeframe: sig.Timeframe,
		Direction: sig.Direction,
		Score:     sig.Score,
		SignalID:  sig.ID,
		At:        time.Now().UTC(),
	}
}

// WithThreshold returns a copy with Threshold set.
func (e Event) WithThreshold(t float64) Event {
	e.Threshold = t
	return e
}

// WithDetail returns a copy with Detail set.
func (e Event) WithDetail(d string) Event {
	e.Detail = d
	return e
}

// WithSender returns a copy with Sender set.
func (e Event) WithSender(name string) Event {
	e.Sender = name
	return e
}

// Summary is one coalesced drop row.
type Summary struct {
	Event
	Count     int64
	FirstSeen time.Time
	LastSeen  time.Time
}

// Query filters Recent. The zero value is all symbols and DefaultLimit rows.
type Query struct {
	Symbol string
	Limit  int
}

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Normalize upper-cases Symbol and clamps Limit. Stores call this so every
// implementation applies the same bounds.
func (q Query) Normalize() Query {
	q.Symbol = strings.ToUpper(strings.TrimSpace(q.Symbol))
	switch {
	case q.Limit <= 0:
		q.Limit = DefaultLimit
	case q.Limit > MaxLimit:
		q.Limit = MaxLimit
	}
	return q
}

// Store is the drop-reason contract used by pipeline, notifier, and API.
//
// Record never blocks the caller beyond a struct copy and a non-blocking send,
// never returns an error, and is safe for concurrent use. An invalid Reason is
// dropped, not persisted. Recent returns rows ordered by LastSeen descending.
type Store interface {
	Record(ev Event)
	Recent(ctx context.Context, q Query) ([]Summary, error)
}

// Nop is the default Store. Constructors install Nop so call sites never nil-check.
type Nop struct{}

// Record does nothing.
func (Nop) Record(Event) {}

// Recent returns no rows.
func (Nop) Recent(context.Context, Query) ([]Summary, error) { return nil, nil }

// Enabled reports whether s is a real store. A nil interface and Nop are disabled.
func Enabled(s Store) bool {
	if s == nil {
		return false
	}
	_, nop := s.(Nop)
	return !nop
}
