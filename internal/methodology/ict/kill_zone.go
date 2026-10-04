package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTKillZoneRule signals in the New York-local ICT kill zones. Session
// boundaries follow daylight saving time through market.Sessions.
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

	t := nowFn()
	sessions := market.SessionsAt(t)
	var sessionName string
	switch {
	case sessions.Asia.Contains(t):
		sessionName = "Asia"
	case sessions.London.Contains(t):
		sessionName = "London"
	case sessions.NewYork.Contains(t):
		sessionName = "New York AM"
	case sessions.LondonClose.Contains(t):
		sessionName = "London Close"
	default:
		return nil, nil
	}
	local, _ := time.LoadLocation("America/New_York")
	ny := t.In(local)

	return &models.Signal{
		Symbol:        ctx.Symbol,
		Timeframe:     "ALL",
		Rule:          r.Name(),
		Direction:     "NEUTRAL",
		Score:         1.0,
		Message:       fmt.Sprintf("%s kill zone active (%02d:%02d New York)", sessionName, ny.Hour(), ny.Minute()),
		MessageKey:    "signal.ict.kill_zone",
		MessageParams: map[string]string{"session": sessionName, "time": ny.Format("15:04"), "timezone": "America/New_York"},
		CreatedAt:     t,
	}, nil
}
