package storage

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// HTFOutcomePolicy is immutable: changing any assumption requires a new ID.
// First available subsequent bar open; five calendar days; up to seven days
// of calendar tolerance at either end. No TP/SL, leverage or portfolio sizing.
const HTFOutcomePolicy = "first_available_open_5d_cost30bps_v1"
const htfCostPct = 0.30 // 10 bps fee + 5 bps slippage per side, illustrative.

// UpdateHTFOutcomes processes a bounded fair batch, including missing-data retries.
// A separate table avoids confusing absent outcomes with genuine zero returns.
func (db *DB) UpdateHTFOutcomes(ctx context.Context, now time.Time) error {
	rows, err := db.conn.QueryContext(ctx, `SELECT o.id,o.symbol,o.timeframe,o.direction,o.observed_at
 FROM htf_opportunities o LEFT JOIN htf_outcomes r ON r.opportunity_id=o.id AND r.policy=?
 WHERE o.version=? AND o.observed_at<=? AND
 (r.opportunity_id IS NULL OR (r.status!='complete' AND r.checked_at<=?))
 ORDER BY COALESCE(r.checked_at,0),o.id LIMIT 200`, HTFOutcomePolicy, models.HTFOpportunityVersion, now.UnixMilli(), now.Add(-time.Hour).UnixMilli())
	if err != nil {
		return err
	}
	type item struct {
		id                    int64
		symbol, tf, direction string
		observed              int64
	}
	var items []item
	for rows.Next() {
		var o item
		if err := rows.Scan(&o.id, &o.symbol, &o.tf, &o.direction, &o.observed); err != nil {
			rows.Close()
			return err
		}
		items = append(items, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, o := range items {
		duration := time.Hour
		if o.tf == "4H" {
			duration = 4 * time.Hour
		}
		observed := time.UnixMilli(o.observed)
		entry, err := db.htfOutcomePrice(ctx, o.symbol, o.tf, observed.Add(time.Millisecond), observed.Add(7*24*time.Hour), now.Add(-duration))
		if err != nil {
			return err
		}
		status := "pending"
		var entryTime, exitTime, entryPrice, exitPrice, gross, net any
		if entry == nil {
			if !now.Before(observed.Add(7*24*time.Hour + duration)) {
				status = "missing_data"
			}
		} else {
			target := entry.OpenTime.Add(5 * 24 * time.Hour)
			exit, err := db.htfOutcomePrice(ctx, o.symbol, o.tf, target, target.Add(7*24*time.Hour), now.Add(-duration))
			if err != nil {
				return err
			}
			if exit == nil {
				if !now.Before(target.Add(7*24*time.Hour + duration)) {
					status = "missing_data"
				}
			} else if validHTFPrice(entry.Open) && validHTFPrice(exit.Open) {
				r := (exit.Open/entry.Open - 1) * 100
				if o.direction == "SHORT" {
					r = -r
				}
				if !math.IsNaN(r) && !math.IsInf(r, 0) {
					status = "complete"
					entryTime = entry.OpenTime.UnixMilli()
					exitTime = exit.OpenTime.UnixMilli()
					entryPrice = entry.Open
					exitPrice = exit.Open
					gross = r
					net = r - htfCostPct
				} else {
					status = "missing_data"
				}
			} else {
				status = "missing_data"
			}
		}
		_, err = db.conn.ExecContext(ctx, `INSERT INTO htf_outcomes
  (opportunity_id,policy,status,checked_at,entry_time,exit_time,entry_price,exit_price,gross_return,net_return)
  VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(opportunity_id,policy) DO UPDATE SET
  status=excluded.status,checked_at=excluded.checked_at,entry_time=excluded.entry_time,exit_time=excluded.exit_time,
  entry_price=excluded.entry_price,exit_price=excluded.exit_price,gross_return=excluded.gross_return,net_return=excluded.net_return
  WHERE htf_outcomes.status!='complete'`, o.id, HTFOutcomePolicy, status, now.UnixMilli(), entryTime, exitTime, entryPrice, exitPrice, gross, net)
		if err != nil {
			return err
		}
	}
	return nil
}

func validHTFPrice(p float64) bool { return p > 0 && !math.IsNaN(p) && !math.IsInf(p, 0) }

// Restrict to elapsed bars, including for open-price valuation: mutable current
// candles are never read. Select first, then validate, rather than skip bad prices.
func (db *DB) htfOutcomePrice(ctx context.Context, symbol, tf string, from, to, closedBefore time.Time) (*models.OHLCV, error) {
	var b models.OHLCV
	var ts int64
	err := db.conn.QueryRowContext(ctx, `SELECT open_time,open FROM ohlcv
 WHERE symbol=? AND timeframe=? AND open_time>=? AND open_time<=? AND open_time<=?
 ORDER BY open_time LIMIT 1`, symbol, tf, from.UnixMilli(), to.UnixMilli(), closedBefore.UnixMilli()).Scan(&ts, &b.Open)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.OpenTime = time.UnixMilli(ts)
	return &b, nil
}

type HTFPerformanceBucket struct {
	Decile             int      `json:"decile"`
	Opportunities      int      `json:"opportunities"`
	DistinctDays       int      `json:"distinct_days"`
	From               *int64   `json:"from"`
	To                 *int64   `json:"to"`
	Pending            int      `json:"pending"`
	MissingData        int      `json:"missing_data"`
	Complete           int      `json:"complete"`
	AverageGrossReturn *float64 `json:"average_gross_return_pct"`
	AverageNetReturn   *float64 `json:"average_net_return_pct"`
	HistorySufficient  bool     `json:"history_sufficient"`
}

type HTFPerformance struct {
	Symbol           string                 `json:"symbol"`
	Timeframe        string                 `json:"timeframe"`
	Policy           string                 `json:"policy"`
	RoundTripCostPct float64                `json:"round_trip_cost_pct"`
	Ready            bool                   `json:"ready"`
	Blockers         []string               `json:"blockers"`
	Buckets          []HTFPerformanceBucket `json:"buckets"`
}

// HTFPerformance reports effective counter-trend observations only. Averages
// are per opportunity, not independent trades or a compounded portfolio return.
func (db *DB) HTFPerformance(ctx context.Context, symbol, tf string, now time.Time) (*HTFPerformance, error) {
	r := &HTFPerformance{Symbol: symbol, Timeframe: tf, Policy: HTFOutcomePolicy, RoundTripCostPct: htfCostPct,
		Blockers: []string{"closed_candle_replay_required", "out_of_sample_validation_required"}, Buckets: make([]HTFPerformanceBucket, 10)}
	for i := range r.Buckets {
		r.Buckets[i].Decile = i
	}
	rows, err := db.conn.QueryContext(ctx, `SELECT
 CASE WHEN o.atr_percentile=100 THEN 9 ELSE CAST(o.atr_percentile/10 AS INTEGER) END,
 COUNT(*),COUNT(DISTINCT date(o.observed_at/1000,'unixepoch')),MIN(o.observed_at),MAX(o.observed_at),
 SUM(CASE WHEN r.status IS NULL OR r.status='pending' THEN 1 ELSE 0 END),
 SUM(CASE WHEN r.status='missing_data' THEN 1 ELSE 0 END),
 SUM(CASE WHEN r.status='complete' THEN 1 ELSE 0 END),AVG(r.gross_return),AVG(r.net_return)
 FROM htf_opportunities o LEFT JOIN htf_outcomes r ON r.opportunity_id=o.id AND r.policy=? AND r.checked_at<=?
 WHERE o.version=? AND o.symbol=? AND o.timeframe=? AND o.observed_at<=?
 AND o.effective_htf_trend IN ('LONG','SHORT') AND o.direction!=o.effective_htf_trend
 AND o.atr_percentile BETWEEN 0 AND 100 GROUP BY 1`, HTFOutcomePolicy, now.UnixMilli(), models.HTFOpportunityVersion, symbol, tf, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b HTFPerformanceBucket
		var first, last int64
		if err := rows.Scan(&b.Decile, &b.Opportunities, &b.DistinctDays, &first, &last, &b.Pending, &b.MissingData, &b.Complete, &b.AverageGrossReturn, &b.AverageNetReturn); err != nil {
			return nil, err
		}
		b.From = &first
		b.To = &last
		b.HistorySufficient = b.DistinctDays >= 30 && !time.UnixMilli(last).Before(time.UnixMilli(first).UTC().AddDate(2, 0, 0))
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
