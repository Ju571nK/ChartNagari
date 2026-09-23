package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SaveHTFOpportunities atomically records the first observation for each
// version/symbol/timeframe/rule/direction/bar. Retries and subsequent ticks never
// overwrite the initial score or context. Unknown outcomes are not stored as zero.
func (db *DB) SaveHTFOpportunities(ctx context.Context, observations []models.HTFOpportunity) error {
	if len(observations) == 0 {
		return nil
	}
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, o := range observations {
		s := o.Signal
		validTrend := func(t string) bool { return t == "" || t == "LONG" || t == "SHORT" }
		if o.Version != models.HTFOpportunityVersion || s.Symbol == "" || s.Rule == "" ||
			(s.Timeframe != "1H" && s.Timeframe != "4H") || (s.Direction != "LONG" && s.Direction != "SHORT") ||
			o.BarOpenTime.IsZero() || o.ObservedAt.IsZero() || o.BarOpenTime.After(o.ObservedAt) ||
			!validTrend(o.RawHTFTrend) || !validTrend(o.EffectiveHTFTrend) ||
			math.IsNaN(s.Score) || math.IsInf(s.Score, 0) ||
			math.IsNaN(o.ATRPercentile) || math.IsInf(o.ATRPercentile, 0) ||
			(o.ATRPercentile != -1 && (o.ATRPercentile < 0 || o.ATRPercentile > 100)) || !json.Valid([]byte(o.Snapshot)) {
			return fmt.Errorf("invalid HTF opportunity for %s/%s/%s", s.Symbol, s.Timeframe, s.Rule)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO htf_opportunities
			(version,symbol,timeframe,rule,direction,bar_open_time,observed_at,original_score,raw_htf_trend,effective_htf_trend,atr_percentile,snapshot)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(version,symbol,timeframe,rule,direction,bar_open_time) DO NOTHING`,
			o.Version, s.Symbol, s.Timeframe, s.Rule, s.Direction, o.BarOpenTime.UnixMilli(), o.ObservedAt.UnixMilli(),
			s.Score, o.RawHTFTrend, o.EffectiveHTFTrend, o.ATRPercentile, o.Snapshot)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
