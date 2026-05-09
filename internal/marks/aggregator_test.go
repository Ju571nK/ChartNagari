package marks

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/storage"
)

func newAgg(t *testing.T) (*storage.DB, *Aggregator) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agg.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := storage.NewSignalMarkStore(db)
	return db, NewAggregator(db, store)
}

// seedAndMark inserts a signal and applies a sequence of mark actions.
func seedAndMark(t *testing.T, db *storage.DB, store *storage.SignalMarkStore, rule, symbol, tf string, actions ...string) {
	t.Helper()
	res, err := db.Conn().Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at)
		VALUES (?, ?, ?, 'LONG', 10.0, '', '', 0, 0, '', 0, ?)`,
		symbol, tf, rule, time.Now().Unix())
	if err != nil {
		t.Fatalf("seed signal: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	for _, a := range actions {
		if _, err := store.Mark(id, a); err != nil {
			t.Fatalf("mark %q: %v", a, err)
		}
	}
}

func TestAggregator_RollupByRule(t *testing.T) {
	db, agg := newAgg(t)
	store := storage.NewSignalMarkStore(db)

	// rule_a: 3 took, 2 win, 1 loss; 1 skip. HitRate = 2/3, SkipRate = 1/4.
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "took", "loss")
	seedAndMark(t, db, store, "rule_a", "BTC", "1H", "skip")
	// rule_b: 1 took, 1 BE.
	seedAndMark(t, db, store, "rule_b", "ETH", "4H", "took", "be")

	rows, err := agg.Rollup(GroupByRule, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	byKey := map[string]RollupRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	a := byKey["rule_a"]
	if a.Took != 3 || a.Wins != 2 || a.Losses != 1 || a.Skipped != 1 {
		t.Errorf("rule_a counts wrong: %+v", a)
	}
	wantHit := 2.0 / 3.0
	if absF(a.HitRate-wantHit) > 0.001 {
		t.Errorf("rule_a HitRate = %.3f, want %.3f", a.HitRate, wantHit)
	}
	if absF(a.SkipRate-0.25) > 0.001 {
		t.Errorf("rule_a SkipRate = %.3f, want 0.25", a.SkipRate)
	}

	b := byKey["rule_b"]
	if b.Took != 1 || b.BreakEvens != 1 || b.HitRate != 0.0 {
		t.Errorf("rule_b: %+v (BE in denominator only, HitRate should be 0)", b)
	}
}

func TestAggregator_RollupBySymbol(t *testing.T) {
	db, agg := newAgg(t)
	store := storage.NewSignalMarkStore(db)
	seedAndMark(t, db, store, "rule_x", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "rule_y", "BTC", "1H", "took", "loss")
	seedAndMark(t, db, store, "rule_x", "ETH", "4H", "skip")

	rows, _ := agg.Rollup(GroupBySymbol, time.Time{})
	byKey := map[string]RollupRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	if byKey["BTC"].Took != 2 || byKey["ETH"].Skipped != 1 {
		t.Errorf("symbol rollup wrong: %+v", byKey)
	}
}

func TestAggregator_RollupByMethodology(t *testing.T) {
	db, agg := newAgg(t)
	store := storage.NewSignalMarkStore(db)
	seedAndMark(t, db, store, "ict_order_block", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "wyckoff_spring", "BTC", "1H", "took", "loss")

	rows, _ := agg.Rollup(GroupByMethodology, time.Time{})
	byKey := map[string]RollupRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	if byKey["ict"].Wins != 1 {
		t.Errorf("ict wins = %d, want 1", byKey["ict"].Wins)
	}
	if byKey["wyckoff"].Losses != 1 {
		t.Errorf("wyckoff losses = %d, want 1", byKey["wyckoff"].Losses)
	}
}

func TestAggregator_RollupByTimeframe(t *testing.T) {
	db, agg := newAgg(t)
	store := storage.NewSignalMarkStore(db)
	seedAndMark(t, db, store, "rule_x", "BTC", "1H", "took", "win")
	seedAndMark(t, db, store, "rule_x", "BTC", "1D", "took", "loss")

	rows, _ := agg.Rollup(GroupByTimeframe, time.Time{})
	byKey := map[string]RollupRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	if byKey["1H"].Wins != 1 || byKey["1D"].Losses != 1 {
		t.Errorf("timeframe rollup wrong: %+v", byKey)
	}
}

func TestAggregator_EmptyResult(t *testing.T) {
	_, agg := newAgg(t)
	rows, err := agg.Rollup(GroupByRule, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("empty DB should return empty rollup, got %d rows", len(rows))
	}
}

func TestAggregator_InvalidGroupBy(t *testing.T) {
	_, agg := newAgg(t)
	_, err := agg.Rollup(GroupBy("explode"), time.Time{})
	if err == nil {
		t.Errorf("expected error for invalid GroupBy")
	}
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
