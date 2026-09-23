package pipeline

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Ju571nK/Chatter/internal/wyckoff"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// HTFOpportunitySaver persists observations separately from alerted signals.
type HTFOpportunitySaver interface {
	SaveHTFOpportunities(context.Context, []models.HTFOpportunity) error
}

// SetHTFOpportunitySaver enables pre-filter collection. Call before Run.
func (p *Pipeline) SetHTFOpportunitySaver(s HTFOpportunitySaver) {
	p.opportunitySaver = s
}

func (p *Pipeline) recordHTFOpportunities(ctx context.Context, signals []models.Signal, bars map[string][]models.OHLCV, indicators map[string]float64, phase wyckoff.Phase) {
	if p.opportunitySaver == nil || len(signals) == 0 {
		return
	}
	raw, effective := effectiveHTFContext(indicators, bars, phase)
	percentile := -1.0
	if daily := bars["1D"]; len(daily) > 0 && indicators["1D:ATR_14"] > 0 {
		percentile = atrPercentile(daily, indicators["1D:ATR_14"], 14, 90)
	}
	latest := make(map[string]models.OHLCV, len(bars))
	for tf, history := range bars {
		if len(history) > 0 {
			latest[tf] = history[0]
		}
	}
	snapshot, err := json.Marshal(struct {
		Indicators map[string]float64      `json:"indicators"`
		LatestBars map[string]models.OHLCV `json:"latest_bars"`
		Phase      wyckoff.Phase           `json:"wyckoff_phase"`
	}{indicators, latest, phase})
	if err != nil {
		p.log.Error().Err(err).Msg("HTF opportunity snapshot failed")
		return
	}
	now := time.Now().UTC()
	var observations []models.HTFOpportunity
	for _, sig := range signals {
		if (sig.Timeframe != "1H" && sig.Timeframe != "4H") || (sig.Direction != "LONG" && sig.Direction != "SHORT") {
			continue
		}
		history := bars[sig.Timeframe]
		if len(history) == 0 || history[0].OpenTime.IsZero() || history[0].OpenTime.After(now) {
			continue
		}
		observations = append(observations, models.HTFOpportunity{
			Version: models.HTFOpportunityVersion, Signal: sig,
			BarOpenTime: history[0].OpenTime, ObservedAt: now,
			RawHTFTrend: raw, EffectiveHTFTrend: effective,
			ATRPercentile: percentile, Snapshot: string(snapshot),
		})
	}
	if len(observations) > 0 {
		if err := p.opportunitySaver.SaveHTFOpportunities(ctx, observations); err != nil {
			p.log.Error().Err(err).Msg("HTF opportunity persistence failed")
		}
	}
}
