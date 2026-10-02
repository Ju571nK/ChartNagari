package storage

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestHTFOutcomesAndPerformance(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	save := func(symbol, direction, trend string, pct float64) {
		t.Helper()
		err := db.SaveHTFOpportunities(ctx, []models.HTFOpportunity{{Version: 1, Signal: models.Signal{Symbol: symbol, Timeframe: "1H", Rule: "r", Direction: direction, Score: 10}, BarOpenTime: base.Add(-30 * time.Minute), ObservedAt: base, RawHTFTrend: trend, EffectiveHTFTrend: trend, ATRPercentile: pct, Snapshot: `{}`}})
		if err != nil {
			t.Fatal(err)
		}
	}
	bar := func(symbol string, at time.Time, price float64) {
		t.Helper()
		if err := db.SaveOHLCV(models.OHLCV{Symbol: symbol, Timeframe: "1H", OpenTime: at, Open: price}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	save("TEST", "LONG", "SHORT", 100)
	save("TEST", "SHORT", "LONG", 10)
	save("MISSING", "LONG", "SHORT", 5)
	save("ZERO", "LONG", "SHORT", 0)
	// Neither the signal candle nor a candle opening at the observation can be used.
	bar("TEST", base.Add(-30*time.Minute), 1)
	bar("TEST", base, 2)
	entry := base.Add(30 * time.Minute)
	exit := entry.AddDate(0, 0, 5)
	bar("TEST", entry, 100)
	bar("TEST", exit, 110)
	bar("ZERO", entry, 100)
	bar("ZERO", exit, 100)
	if err := db.UpdateHTFOutcomes(ctx, exit.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	r, err := db.HTFPerformance(ctx, "TEST", "1H", exit)
	if err != nil {
		t.Fatal(err)
	}
	if r.Buckets[9].Pending != 1 || r.Buckets[9].AverageNetReturn != nil {
		t.Fatalf("future price used: %+v", r)
	}
	now := exit.Add(time.Hour)
	if err := db.UpdateHTFOutcomes(ctx, now); err != nil {
		t.Fatal(err)
	}
	r, err = db.HTFPerformance(ctx, "TEST", "1H", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		bucket int
		want   float64
	}{{9, 9.7}, {1, -10.3}} {
		b := r.Buckets[tc.bucket]
		if b.Complete != 1 || b.AverageNetReturn == nil || math.Abs(*b.AverageNetReturn-tc.want) > 1e-8 {
			t.Fatalf("bad direction/cost: %+v", b)
		}
	}
	if r.Ready || len(r.Buckets) != 10 || r.Buckets[0].AverageNetReturn != nil {
		t.Fatal("unsafe report")
	}
	z, err := db.HTFPerformance(ctx, "ZERO", "1H", now)
	if err != nil {
		t.Fatal(err)
	}
	if z.Buckets[0].Complete != 1 || z.Buckets[0].AverageGrossReturn == nil || *z.Buckets[0].AverageGrossReturn != 0 {
		t.Fatal("valid zero lost")
	}
	// Missing rows are retried and can recover on backfill; complete results freeze.
	later := base.AddDate(0, 0, 20)
	if err := db.UpdateHTFOutcomes(ctx, later); err != nil {
		t.Fatal(err)
	}
	m, err := db.HTFPerformance(ctx, "MISSING", "1H", later)
	if err != nil {
		t.Fatal(err)
	}
	if m.Buckets[0].MissingData != 1 || m.Buckets[0].AverageNetReturn != nil {
		t.Fatal("missing data treated as zero")
	}
	bar("MISSING", entry, 100)
	bar("MISSING", exit, 120)
	bar("TEST", exit, 999)
	if err := db.UpdateHTFOutcomes(ctx, later.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	m, err = db.HTFPerformance(ctx, "MISSING", "1H", later.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if m.Buckets[0].Complete != 1 {
		t.Fatal("backfill not retried")
	}
	r, err = db.HTFPerformance(ctx, "TEST", "1H", later.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(*r.Buckets[9].AverageNetReturn-9.7) > 1e-8 {
		t.Fatal("complete result overwritten")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := db.UpdateHTFOutcomes(canceled, later); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err := db.HTFPerformance(canceled, "TEST", "1H", later); err == nil {
		t.Fatal("read cancellation ignored")
	}
}

func TestHTFOutcomeFourHourBoundaryAndIsolation(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	observed := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	o := models.HTFOpportunity{Version: 1, Signal: models.Signal{Symbol: "TEST", Timeframe: "4H", Rule: "r", Direction: "LONG", Score: 1}, ObservedAt: observed, BarOpenTime: observed.Add(-time.Hour), EffectiveHTFTrend: "SHORT", ATRPercentile: 9.99, Snapshot: `{}`}
	if err := db.SaveHTFOpportunities(ctx, []models.HTFOpportunity{o}); err != nil {
		t.Fatal(err)
	}
	entry := observed.Add(3 * time.Hour)
	exit := entry.AddDate(0, 0, 5)
	for _, b := range []models.OHLCV{
		{Symbol: "TEST", Timeframe: "4H", OpenTime: entry, Open: 100},
		{Symbol: "TEST", Timeframe: "4H", OpenTime: exit, Open: 100},
		{Symbol: "TEST", Timeframe: "1H", OpenTime: observed.Add(time.Hour), Open: 1},
		{Symbol: "OTHER", Timeframe: "4H", OpenTime: observed.Add(time.Hour), Open: 1},
	} {
		if err := db.SaveOHLCV(b, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateHTFOutcomes(ctx, exit.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	r, err := db.HTFPerformance(ctx, "TEST", "4H", exit.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r.Buckets[0].Pending != 1 {
		t.Fatal("4H bar used before end")
	}
	if err := db.UpdateHTFOutcomes(ctx, exit.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	r, err = db.HTFPerformance(ctx, "TEST", "4H", exit.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r.Buckets[0].Complete != 1 || *r.Buckets[0].AverageGrossReturn != 0 {
		t.Fatal("wrong timeframe/symbol price")
	}
	// Aligned, unknown and Wyckoff-relaxed contexts must not pollute the report.
	for _, trend := range []string{"LONG", ""} {
		o.Signal.Rule = "r" + trend
		o.EffectiveHTFTrend = trend
		if err := db.SaveHTFOpportunities(ctx, []models.HTFOpportunity{o}); err != nil {
			t.Fatal(err)
		}
	}
	r, err = db.HTFPerformance(ctx, "TEST", "4H", exit.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r.Buckets[0].Opportunities != 1 {
		t.Fatal("non-counter-trend included")
	}
}
