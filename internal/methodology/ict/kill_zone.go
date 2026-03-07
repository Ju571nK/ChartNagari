package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTKillZoneRule signals when the current UTC time is within a kill zone session.
//
// Kill zones (UTC):
//   London:   08:00–11:00
//   New York: 13:00–16:00
//
// Returns a NEUTRAL signal when in a kill zone, nil otherwise.
// Score = 1.0 in kill zone.
// The `now` field is injectable for testing (defaults to time.Now).
type ICTKillZoneRule struct {
	now func() time.Time // use time.Now in production; override in tests
}

// NewICTKillZoneRule creates a kill zone rule with time.Now as the clock.
func NewICTKillZoneRule() *ICTKillZoneRule {
	return &ICTKillZoneRule{now: time.Now}
}

func (r *ICTKillZoneRule) Name() string                 { return "ict_kill_zone" }
func (r *ICTKillZoneRule) RequiredIndicators() []string { return nil }

func (r *ICTKillZoneRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	nowFn := r.now
	if nowFn == nil {
		nowFn = time.Now
	}

	t := nowFn().UTC()
	hour := t.Hour()
	minute := t.Minute()
	hhmm := hour*100 + minute

	var sessionName string

	if hhmm >= 800 && hhmm < 1100 {
		sessionName = "London"
	} else if hhmm >= 1300 && hhmm < 1600 {
		sessionName = "New York"
	} else {
		return nil, nil
	}

	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: "ALL",
		Rule:      r.Name(),
		Direction: "NEUTRAL",
		Score:     1.0,
		Message:   fmt.Sprintf("%s Kill Zone 활성 (UTC %02d:%02d)", sessionName, hour, minute),
		CreatedAt: nowFn(),
	}, nil
}
