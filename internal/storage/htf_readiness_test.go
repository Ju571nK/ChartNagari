package storage

import (
	"context"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestHTFInventoryBoundariesAndIsolation(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	base := models.Signal{Symbol: "TEST", Timeframe: "1H", Direction: "SHORT", HTFTrend: "LONG", CreatedAt: now.AddDate(-3, 0, 0)}
	save := func(s models.Signal) {
		t.Helper()
		if _, err := db.SaveSignal(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []float64{0, 9.99, 10, 99.99, 100, -1, 101} {
		s := base
		s.ATRPercentile = p
		save(s)
	}
	for i := 0; i < 30; i++ {
		s := base
		s.ATRPercentile = 25
		s.CreatedAt = base.CreatedAt.AddDate(0, 0, i*30)
		save(s)
	}
	for i := 0; i < 40; i++ {
		s := base
		s.ATRPercentile = 35
		save(s)
	}
	s := base
	s.Symbol = "OTHER"
	save(s)
	s = base
	s.Timeframe = "4H"
	save(s)
	s = base
	s.Direction = "LONG"
	save(s)
	s = base
	s.HTFTrend = ""
	save(s)
	s = base
	s.CreatedAt = now.AddDate(0, 0, 1)
	save(s)
	r, err := db.HTFCalibrationReadiness(context.Background(), "TEST", "1H", now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ready || len(r.Buckets) != 10 {
		t.Fatalf("unsafe readiness: %+v", r)
	}
	if r.Buckets[0].Signals != 2 || r.Buckets[1].Signals != 1 || r.Buckets[9].Signals != 2 {
		t.Fatalf("bad deciles: %+v", r.Buckets)
	}
	if !r.Buckets[2].HistorySufficient || r.Buckets[3].HistorySufficient || r.Buckets[3].DistinctDays != 1 {
		t.Fatalf("bad history gate: %+v", r.Buckets)
	}
	if r.Buckets[4].From != nil || r.Buckets[4].To != nil {
		t.Fatal("invented empty range")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.HTFCalibrationReadiness(cancelled, "TEST", "1H", now); err == nil {
		t.Fatal("ignored cancellation")
	}
}
