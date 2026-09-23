package storage

import (
	"context"
	"github.com/Ju571nK/Chatter/pkg/models"
	"math"
	"testing"
	"time"
)

func TestHTFOpportunitiesPersistence(t *testing.T) {
	path := t.TempDir() + "/test.db"
	db, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	o := models.HTFOpportunity{Version: models.HTFOpportunityVersion,
		Signal:      models.Signal{Symbol: "TEST", Timeframe: "1H", Rule: "test", Direction: "SHORT", Score: 10},
		BarOpenTime: now.Add(-time.Hour), ObservedAt: now, RawHTFTrend: "LONG", EffectiveHTFTrend: "", ATRPercentile: 100, Snapshot: `{"wyckoff_phase":"distribution"}`}
	save := func(items ...models.HTFOpportunity) {
		t.Helper()
		if err := db.SaveHTFOpportunities(context.Background(), items); err != nil {
			t.Fatal(err)
		}
	}
	save(o)
	changed := o
	changed.Signal.Score = 1
	changed.EffectiveHTFTrend = "LONG"
	save(changed)
	var score float64
	var trend string
	if err := db.conn.QueryRow(`SELECT original_score,effective_htf_trend FROM htf_opportunities`).Scan(&score, &trend); err != nil {
		t.Fatal(err)
	}
	if score != 10 || trend != "" {
		t.Fatalf("first observation overwritten: %v %q", score, trend)
	}
	for _, mutate := range []func(*models.HTFOpportunity){
		func(o *models.HTFOpportunity) { o.Signal.Symbol = "OTHER" },
		func(o *models.HTFOpportunity) { o.Signal.Timeframe = "4H" },
		func(o *models.HTFOpportunity) { o.Signal.Rule = "other" },
		func(o *models.HTFOpportunity) { o.Signal.Direction = "LONG" },
		func(o *models.HTFOpportunity) { o.BarOpenTime = o.BarOpenTime.Add(time.Minute) },
	} {
		copy := o
		mutate(&copy)
		save(copy)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.conn.QueryRow(`SELECT count(*) FROM htf_opportunities`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Fatalf("identity/migration: got %d", count)
	}
	// Invalid second row rolls back the valid first row as well.
	for _, mutate := range []func(*models.HTFOpportunity){
		func(o *models.HTFOpportunity) { o.ATRPercentile = math.NaN() },
		func(o *models.HTFOpportunity) { o.ATRPercentile = -0.5 },
		func(o *models.HTFOpportunity) { o.Signal.Score = math.Inf(1) },
		func(o *models.HTFOpportunity) { o.Signal.Timeframe = "1D" },
		func(o *models.HTFOpportunity) { o.Version = 99 },
		func(o *models.HTFOpportunity) { o.Snapshot = "invalid" },
		func(o *models.HTFOpportunity) { o.BarOpenTime = now.Add(time.Hour) },
	} {
		valid := o
		valid.Signal.Symbol = "ROLLBACK"
		bad := o
		mutate(&bad)
		if err := db.SaveHTFOpportunities(context.Background(), []models.HTFOpportunity{valid, bad}); err == nil {
			t.Fatal("invalid observation accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.SaveHTFOpportunities(ctx, []models.HTFOpportunity{o}); err == nil {
		t.Fatal("cancelled write accepted")
	}
	if err := db.conn.QueryRow(`SELECT count(*) FROM htf_opportunities`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Fatalf("failed batch wrote rows: %d", count)
	}
}
