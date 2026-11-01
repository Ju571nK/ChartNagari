package pipeline

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/alertdrop"
	"github.com/Ju571nK/Chatter/internal/wyckoff"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func sig(rule, tf, dir string, score float64) models.Signal {
	return models.Signal{Symbol: "AAPL", Rule: rule, Timeframe: tf, Direction: dir, Score: score}
}

func TestRemoved(t *testing.T) {
	low := sig("rsi", "1H", "LONG", 2)
	high := sig("rsi", "1H", "LONG", 9)
	other := sig("macd", "4H", "SHORT", 8)
	dup := sig("ict_fvg", "1H", "SHORT", 7)

	// In-place filters reuse the backing array. Diff against a snapshot.
	batch := []models.Signal{low, high}
	snapshot := append([]models.Signal(nil), batch...)
	kept := filterMinScore(batch, 5)
	if got := removed(snapshot, kept); len(got) != 1 || got[0].Score != 2 {
		t.Fatalf("in-place drop = %+v", got)
	}

	// HTF score rewrite on a kept signal is not a removal.
	before := []models.Signal{high}
	after := []models.Signal{sig("rsi", "1H", "LONG", 4.5)}
	if got := removed(before, after); len(got) != 0 {
		t.Fatalf("score rewrite counted as drop: %+v", got)
	}

	// Two identical identities, one kept.
	both := []models.Signal{dup, dup}
	if got := removed(both, both[:1]); len(got) != 1 {
		t.Fatalf("multiset drop = %d", len(got))
	}

	if got := removed([]models.Signal{other}, nil); len(got) != 1 || got[0].Rule != "macd" {
		t.Fatalf("full removal = %+v", got)
	}
	if len(removed(nil, nil)) != 0 {
		t.Fatal("empty input reported a drop")
	}
}

func TestMTFHaltedOnlyWhenBatchBecomesEmpty(t *testing.T) {
	cases := []struct {
		before, after int
		want          bool
	}{
		{0, 0, false},
		{2, 2, false},
		{2, 1, false},
		{1, 0, true},
		{3, 0, true},
	}
	for _, tc := range cases {
		if got := mtfHalted(tc.before, tc.after); got != tc.want {
			t.Fatalf("mtfHalted(%d,%d)=%v want %v", tc.before, tc.after, got, tc.want)
		}
	}
}

func TestGateRecordsHardDropsOnly(t *testing.T) {
	mem := &memDrops{}
	p := &Pipeline{log: zerolog.Nop(), drops: mem}
	in := []models.Signal{sig("rsi", "1H", "LONG", 2), sig("rsi", "1H", "SHORT", 9)}
	out := p.gate("AAPL", alertdrop.Score, in, func(s []models.Signal) []models.Signal {
		return filterMinScore(s, 5)
	}, "threshold=5.0")
	if len(out) != 1 || out[0].Direction != "SHORT" {
		t.Fatalf("kept = %+v", out)
	}
	if len(mem.events) != 1 || mem.events[0].Reason != alertdrop.Score || mem.events[0].Direction != "LONG" {
		t.Fatalf("recorded = %+v", mem.events)
	}
	if mem.events[0].Detail != "threshold=5.0" || mem.events[0].Score != 2 {
		t.Fatalf("event attrs = %+v", mem.events[0])
	}

	// Partial HTF penalty keeps the signal. Full suppression records htf.
	bars := map[string][]models.OHLCV{"1D": {{Close: 120}}}
	indicators := map[string]float64{"1D:EMA_50": 110, "1D:EMA_200": 100, "1D:ADX_14": 25}
	counter := []models.Signal{sig("ict_fvg", "1H", "SHORT", 10)}
	mem.events = nil
	penalized := p.gate("AAPL", alertdrop.HTF, counter, func(s []models.Signal) []models.Signal {
		return penalizeHTFContext(s, indicators, bars, wyckoff.PhaseRanging, 50)
	}, "phase=ranging penalty=50")
	if len(penalized) != 1 || len(mem.events) != 0 {
		t.Fatalf("partial penalty kept=%d recorded=%d", len(penalized), len(mem.events))
	}
	if penalized[0].Score != 5 {
		t.Fatalf("penalized score = %v", penalized[0].Score)
	}
	removedHTF := p.gate("AAPL", alertdrop.HTF, counter, func(s []models.Signal) []models.Signal {
		return penalizeHTFContext(s, indicators, bars, wyckoff.PhaseRanging, 100)
	}, "phase=ranging penalty=100")
	if len(removedHTF) != 0 || len(mem.events) != 1 || mem.events[0].Reason != alertdrop.HTF || mem.events[0].Score != 10 {
		t.Fatalf("hard htf out=%d events=%+v", len(removedHTF), mem.events)
	}
}

type memDrops struct {
	events []alertdrop.Event
}

func (m *memDrops) Record(ev alertdrop.Event) { m.events = append(m.events, ev) }
func (m *memDrops) Recent(context.Context, alertdrop.Query) ([]alertdrop.Summary, error) {
	return nil, nil
}
