package storage

import (
	"context"
	"time"
)

// HTFBucketReadiness describes persisted, post-filter observations, NOT an
// unbiased sample of counter-trend opportunities or completed trades.
type HTFBucketReadiness struct {
	Decile            int    `json:"decile"`
	Signals           int    `json:"signals"`
	DistinctDays      int    `json:"distinct_days"`
	From              *int64 `json:"from"`
	To                *int64 `json:"to"`
	HistorySufficient bool   `json:"history_sufficient"`
}

type HTFReadiness struct {
	Symbol              string               `json:"symbol"`
	Timeframe           string               `json:"timeframe"`
	MinimumYears        int                  `json:"minimum_years"`
	MinimumDistinctDays int                  `json:"minimum_distinct_days"`
	Ready               bool                 `json:"ready"`
	Source              string               `json:"source"`
	Blockers            []string             `json:"blockers"`
	Buckets             []HTFBucketReadiness `json:"buckets"`
}

// HTFCalibrationReadiness is a read-only inventory for issue #32. Thirty
// distinct dates is an initial sparsity guard, not a statistical confidence
// guarantee. Automatic calibration stays blocked even with sufficient dates:
// suppressed signals are missing and zero forward returns have no validity bit.
func (db *DB) HTFCalibrationReadiness(ctx context.Context, symbol, timeframe string, now time.Time) (*HTFReadiness, error) {
	r := &HTFReadiness{Symbol: symbol, Timeframe: timeframe, MinimumYears: 2, MinimumDistinctDays: 30,
		Source: "post_filter_signals", Blockers: []string{"pre_filter_history_required", "validated_outcomes_required", "out_of_sample_validation_required"},
		Buckets: make([]HTFBucketReadiness, 10)}
	for i := range r.Buckets {
		r.Buckets[i].Decile = i
	}
	rows, err := db.conn.QueryContext(ctx, `SELECT
		CASE WHEN atr_percentile = 100 THEN 9 ELSE CAST(atr_percentile / 10 AS INTEGER) END,
		COUNT(*), COUNT(DISTINCT date(created_at, 'unixepoch')), MIN(created_at), MAX(created_at)
		FROM signals WHERE symbol = ? AND timeframe = ?
		AND direction IN ('LONG','SHORT') AND htf_trend IN ('LONG','SHORT')
		AND direction != htf_trend AND atr_percentile BETWEEN 0 AND 100
		AND created_at > 0 AND created_at <= ? GROUP BY 1`, symbol, timeframe, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b HTFBucketReadiness
		var first, last int64
		if err := rows.Scan(&b.Decile, &b.Signals, &b.DistinctDays, &first, &last); err != nil {
			return nil, err
		}
		b.From, b.To = &first, &last
		b.HistorySufficient = b.DistinctDays >= r.MinimumDistinctDays && !time.Unix(last, 0).Before(time.Unix(first, 0).AddDate(2, 0, 0))
		r.Buckets[b.Decile] = b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, b := range r.Buckets {
		if !b.HistorySufficient {
			r.Blockers = append(r.Blockers, "insufficient_bucket_history")
			break
		}
	}
	return r, nil
}
