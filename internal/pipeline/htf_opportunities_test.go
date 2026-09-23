package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/wyckoff"
	"github.com/Ju571nK/Chatter/pkg/models"
	"testing"
	"time"
)

type opportunityStore struct {
	rows []models.HTFOpportunity
	err  error
}

func (s *opportunityStore) SaveHTFOpportunities(_ context.Context, rows []models.HTFOpportunity) error {
	s.rows = append(s.rows, rows...)
	return s.err
}

type opportunityRule struct{}

func (opportunityRule) Name() string                 { return "opportunity" }
func (opportunityRule) RequiredIndicators() []string { return nil }
func (opportunityRule) Analyze(c models.AnalysisContext) (*models.Signal, error) {
	return &models.Signal{Symbol: c.Symbol, Timeframe: "1H", Rule: "opportunity", Direction: "SHORT", Score: 1}, nil
}

func TestOpportunityCollectionBeforeMTFRejection(t *testing.T) {
	for _, storeErr := range []error{nil, errors.New("disk unavailable")} {
		db := &mockDB{bars: map[string][]models.OHLCV{"TEST|1H": {{OpenTime: time.Now().Add(-time.Hour), Close: 100}}}}
		p := newTestPipeline(db, []string{"TEST"})
		p.eng = engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{"opportunity": {Enabled: true, Timeframe: "1H", Weight: 2}}})
		p.eng.Register(opportunityRule{})
		store := &opportunityStore{err: storeErr}
		p.SetHTFOpportunitySaver(store)
		// Default MTF gate requires two agreeing timeframes; this lone signal fails.
		p.RunOnce(context.Background())
		if len(store.rows) != 1 || store.rows[0].Signal.Score != 2 {
			t.Fatalf("missing unfiltered weighted score: %+v", store.rows)
		}
		if store.rows[0].ATRPercentile != -1 {
			t.Fatal("missing ATR must remain unknown")
		}
	}
}

func TestOpportunityContextMatchesLiveFilter(t *testing.T) {
	for _, phase := range []wyckoff.Phase{"", wyckoff.PhaseDistribution} {
		p := newTestPipeline(&mockDB{}, nil)
		store := &opportunityStore{}
		p.SetHTFOpportunitySaver(store)
		bars := htfBarsAboveEMA("1D")
		bars["1H"] = []models.OHLCV{{OpenTime: time.Now().Add(-time.Hour), Close: 100}}
		indicators := htfIndicatorsLong("1D")
		signals := []models.Signal{{Symbol: "TEST", Timeframe: "1H", Rule: "test", Direction: "SHORT", Score: 10}}
		p.recordHTFOpportunities(context.Background(), signals, bars, indicators, phase)
		filtered := penalizeHTFContext(signals, indicators, bars, phase, 100)
		if len(store.rows) != 1 {
			t.Fatal("suppressed opportunity lost")
		}
		o := store.rows[0]
		if o.RawHTFTrend != "LONG" || o.Signal.Score != 10 || !json.Valid([]byte(o.Snapshot)) {
			t.Fatalf("invalid snapshot: %+v", o)
		}
		if phase == "" && (o.EffectiveHTFTrend != "LONG" || len(filtered) != 0) {
			t.Fatal("counter-trend context mismatch")
		}
		if phase != "" && (o.EffectiveHTFTrend != "" || len(filtered) != 1) {
			t.Fatal("Wyckoff override mismatch")
		}
	}
}

type opportunityPaperTrader struct{ calls int }

func (p *opportunityPaperTrader) OnSignals([]models.Signal)                        { p.calls++ }
func (p *opportunityPaperTrader) CheckPositions(string, map[string][]models.OHLCV) {}

func TestOpportunityWriteFailureDoesNotStopPipeline(t *testing.T) {
	db := &mockDB{bars: map[string][]models.OHLCV{"TEST|1H": {{OpenTime: time.Now().Add(-time.Hour), Close: 100}}}}
	p := newTestPipeline(db, []string{"TEST"})
	p.cfg.MTFConsensusMin = 1
	p.eng = engine.New(engine.RuleConfig{Rules: map[string]engine.RuleEntry{"opportunity": {Enabled: true, Timeframe: "1H", Weight: 2}}})
	p.eng.Register(opportunityRule{})
	p.SetHTFOpportunitySaver(&opportunityStore{err: errors.New("disk unavailable")})
	trader := &opportunityPaperTrader{}
	p.SetPaperTrader(trader)
	p.RunOnce(context.Background())
	if trader.calls != 1 {
		t.Fatal("observation write failure prevented downstream processing")
	}
}
