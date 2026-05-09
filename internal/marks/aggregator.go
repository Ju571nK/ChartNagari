// Package marks computes rolled-up statistics from the signal_marks table.
package marks

import (
	"fmt"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
)

// GroupBy controls the dimension Rollup aggregates over.
type GroupBy string

const (
	GroupByRule        GroupBy = "rule"
	GroupBySymbol      GroupBy = "symbol"
	GroupByMethodology GroupBy = "methodology"
	GroupByTimeframe   GroupBy = "timeframe"
)

// RollupRow is one row of aggregated personal stats.
type RollupRow struct {
	Key        string  `json:"key"`
	Took       int     `json:"took"`
	Skipped    int     `json:"skipped"`
	Wins       int     `json:"wins"`
	Losses     int     `json:"losses"`
	BreakEvens int     `json:"bes"`
	HitRate    float64 `json:"hit_rate"`
	SkipRate   float64 `json:"skip_rate"`
}

// Aggregator computes rollup stats from signal_marks.
type Aggregator struct {
	db    *storage.DB
	store *storage.SignalMarkStore
}

// NewAggregator creates an aggregator backed by the given DB.
func NewAggregator(db *storage.DB, store *storage.SignalMarkStore) *Aggregator {
	return &Aggregator{db: db, store: store}
}

// Rollup returns aggregated counts grouped by the given dimension.
// Pass time.Time{} (zero) to include all marks regardless of date.
func (a *Aggregator) Rollup(by GroupBy, since time.Time) ([]RollupRow, error) {
	sinceUnix := int64(0)
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}

	switch by {
	case GroupByRule:
		return a.rollupByColumn("s.rule", sinceUnix)
	case GroupBySymbol:
		return a.rollupByColumn("s.symbol", sinceUnix)
	case GroupByTimeframe:
		return a.rollupByColumn("s.timeframe", sinceUnix)
	case GroupByMethodology:
		return a.rollupByMethodology(sinceUnix)
	default:
		return nil, fmt.Errorf("unknown groupBy: %q", by)
	}
}

func (a *Aggregator) rollupByColumn(col string, sinceUnix int64) ([]RollupRow, error) {
	q := fmt.Sprintf(`
		SELECT %s,
		       SUM(CASE WHEN m.status IN ('TOOK','WIN','LOSS','BE') THEN 1 ELSE 0 END) AS took,
		       SUM(CASE WHEN m.status = 'SKIPPED' THEN 1 ELSE 0 END) AS skipped,
		       SUM(CASE WHEN m.status = 'WIN'  THEN 1 ELSE 0 END) AS wins,
		       SUM(CASE WHEN m.status = 'LOSS' THEN 1 ELSE 0 END) AS losses,
		       SUM(CASE WHEN m.status = 'BE'   THEN 1 ELSE 0 END) AS bes
		FROM signal_marks m
		JOIN signals s ON s.id = m.signal_id
		WHERE m.updated_at >= ?
		GROUP BY %s
		ORDER BY took DESC`, col, col)

	rows, err := a.db.Conn().Query(q, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("rollup query: %w", err)
	}
	defer rows.Close()
	out := []RollupRow{}
	for rows.Next() {
		var r RollupRow
		if err := rows.Scan(&r.Key, &r.Took, &r.Skipped, &r.Wins, &r.Losses, &r.BreakEvens); err != nil {
			return nil, err
		}
		r.HitRate, r.SkipRate = computeRates(r.Wins, r.Losses, r.BreakEvens, r.Took, r.Skipped)
		out = append(out, r)
	}
	return out, rows.Err()
}

// rollupByMethodology runs a rule-level rollup then re-aggregates by methodology
// using RuleMethodology (no SQL UDF available in modernc.org/sqlite).
func (a *Aggregator) rollupByMethodology(sinceUnix int64) ([]RollupRow, error) {
	ruleRows, err := a.rollupByColumn("s.rule", sinceUnix)
	if err != nil {
		return nil, err
	}
	merged := map[string]*RollupRow{}
	for _, r := range ruleRows {
		method := appconfig.RuleMethodology(r.Key)
		m := merged[method]
		if m == nil {
			m = &RollupRow{Key: method}
			merged[method] = m
		}
		m.Took += r.Took
		m.Skipped += r.Skipped
		m.Wins += r.Wins
		m.Losses += r.Losses
		m.BreakEvens += r.BreakEvens
	}
	out := []RollupRow{}
	for _, m := range merged {
		m.HitRate, m.SkipRate = computeRates(m.Wins, m.Losses, m.BreakEvens, m.Took, m.Skipped)
		out = append(out, *m)
	}
	return out, nil
}

// computeRates returns (hitRate, skipRate). Both clamped to 0 when denominator is 0.
// HitRate = wins / (wins+losses+bes). SkipRate = skipped / (took+skipped).
func computeRates(wins, losses, bes, took, skipped int) (float64, float64) {
	hit := 0.0
	if d := wins + losses + bes; d > 0 {
		hit = float64(wins) / float64(d)
	}
	skip := 0.0
	if d := took + skipped; d > 0 {
		skip = float64(skipped) / float64(d)
	}
	return hit, skip
}
