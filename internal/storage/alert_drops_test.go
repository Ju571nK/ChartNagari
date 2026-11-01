package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/alertdrop"
)

func openDropDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "drops.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newTestStore(t *testing.T, db *DB, opts AlertDropOptions) *AlertDropStore {
	t.Helper()
	if opts.PruneEvery == 0 {
		opts.PruneEvery = time.Hour
	}
	s := NewAlertDropStore(db, opts)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.Close(ctx)
	})
	return s
}

func flushStore(t *testing.T, s *AlertDropStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func dropEvent(symbol, rule string, reason alertdrop.Reason, at time.Time) alertdrop.Event {
	return alertdrop.Event{
		Reason:    reason,
		Symbol:    symbol,
		Rule:      rule,
		Timeframe: "1H",
		Direction: "LONG",
		Score:     8,
		At:        at,
	}
}

func TestNewAlertDropStoreNilDB(t *testing.T) {
	if NewAlertDropStore(nil, AlertDropOptions{}) != nil {
		t.Fatal("nil db returned a store")
	}
	var s *AlertDropStore
	s.Record(dropEvent("BTCUSDT", "ict_fvg", alertdrop.Cooldown, time.Now()))
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recent(context.Background(), alertdrop.Query{}); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertCoalesces(t *testing.T) {
	db := openDropDB(t)
	s := newTestStore(t, db, AlertDropOptions{})
	firstAt := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	thirdAt := secondAt.Add(time.Minute)

	s.Record(dropEvent("btcusdt", "ict_fvg", alertdrop.Cooldown, firstAt).WithDetail("first").WithThreshold(1))
	s.Record(alertdrop.Event{
		Reason: alertdrop.Cooldown, Symbol: "BTCUSDT", Rule: "ict_fvg",
		Timeframe: "1H", Direction: "LONG", Score: 21, Threshold: 4,
		SignalID: 7, Detail: "second", Sender: "", At: secondAt,
	})
	s.Record(alertdrop.Event{
		Reason: alertdrop.Cooldown, Symbol: "BTCUSDT", Rule: "ict_fvg",
		Timeframe: "1H", Direction: "LONG", Score: 3, SignalID: 0,
		Detail: "third", At: thirdAt,
	})
	s.Record(dropEvent("BTCUSDT", "ict_fvg", alertdrop.Cooldown, thirdAt).WithSender("telegram").WithDetail("other sender"))
	flushStore(t, s)

	rows, err := s.Recent(context.Background(), alertdrop.Query{Symbol: "btcusdt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (sender splits identity)", len(rows))
	}
	var main alertdrop.Summary
	for _, row := range rows {
		if row.Sender == "" {
			main = row
		}
	}
	if main.Count != 3 {
		t.Fatalf("count = %d, want 3", main.Count)
	}
	if main.Score != 3 || main.Detail != "third" || main.Threshold != 0 {
		t.Fatalf("latest attrs = score %v detail %q threshold %v", main.Score, main.Detail, main.Threshold)
	}
	if main.SignalID != 7 {
		t.Fatalf("signal_id = %d, want 7 kept across a later zero id", main.SignalID)
	}
	if !main.FirstSeen.Equal(firstAt) || !main.LastSeen.Equal(thirdAt) {
		t.Fatalf("seen %s .. %s", main.FirstSeen, main.LastSeen)
	}
	if main.Symbol != "BTCUSDT" || main.Reason != alertdrop.Cooldown {
		t.Fatalf("identity %+v", main.Event)
	}
}

func TestRecordRejectsEmptySymbolAndInvalidReason(t *testing.T) {
	db := openDropDB(t)
	s := newTestStore(t, db, AlertDropOptions{})
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	s.Record(dropEvent("   ", "ict_fvg", alertdrop.Score, at))
	bad := dropEvent("ETHUSDT", "ict_fvg", alertdrop.Score, at)
	bad.Reason = 0
	s.Record(bad)
	kept := dropEvent("ETHUSDT", "ict_fvg", alertdrop.Score, time.Time{})
	s.Record(kept)
	long := strings.Repeat("a", 199) + "한"
	s.Record(dropEvent("ETHUSDT", "rsi", alertdrop.Score, at).WithDetail(long))
	flushStore(t, s)

	rows, err := s.Recent(context.Background(), alertdrop.Query{Symbol: "ETHUSDT", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the valid events only", len(rows))
	}
	for _, row := range rows {
		if row.Rule == "ict_fvg" {
			if row.LastSeen.IsZero() || time.Since(row.LastSeen) > 5*time.Second {
				t.Fatalf("zero At was not stamped: %s", row.LastSeen)
			}
		}
		if row.Rule == "rsi" {
			if strings.Contains(row.Detail, "한") || len(row.Detail) != 199 {
				t.Fatalf("detail len=%d %q", len(row.Detail), row.Detail)
			}
		}
	}
}

func TestRecentSkipsUnknownReason(t *testing.T) {
	db := openDropDB(t)
	s := newTestStore(t, db, AlertDropOptions{})
	at := formatDropTime(time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC))
	if _, err := db.Conn().Exec(`
		INSERT INTO alert_drops (
			symbol, rule, timeframe, direction, reason, sender,
			score, threshold, detail, count, first_seen, last_seen
		) VALUES ('BTCUSDT','old','1H','LONG','not_a_reason','',1,0,'',1,?,?)`, at, at); err != nil {
		t.Fatal(err)
	}
	s.Record(dropEvent("BTCUSDT", "ict_fvg", alertdrop.MTF, time.Date(2026, 11, 1, 8, 1, 0, 0, time.UTC)))
	flushStore(t, s)

	rows, err := s.Recent(context.Background(), alertdrop.Query{Symbol: "BTCUSDT"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Reason != alertdrop.MTF {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestPrunePerSymbolCap(t *testing.T) {
	db := openDropDB(t)
	s := newTestStore(t, db, AlertDropOptions{PerSymbolCap: 2})
	base := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	for i, rule := range []string{"a", "b", "c", "d"} {
		s.Record(dropEvent("BTCUSDT", rule, alertdrop.Timeframe, base.Add(time.Duration(i)*time.Minute)))
	}
	s.Record(dropEvent("ETHUSDT", "keep", alertdrop.Timeframe, base))
	flushStore(t, s)
	if err := s.prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Recent(context.Background(), alertdrop.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var btc, eth int
	for _, row := range rows {
		switch row.Symbol {
		case "BTCUSDT":
			btc++
			if row.Rule != "c" && row.Rule != "d" {
				t.Fatalf("kept old rule %s", row.Rule)
			}
		case "ETHUSDT":
			eth++
		}
	}
	if btc != 2 || eth != 1 {
		t.Fatalf("btc=%d eth=%d rows=%d", btc, eth, len(rows))
	}
}

func TestPruneTTL(t *testing.T) {
	db := openDropDB(t)
	s := newTestStore(t, db, AlertDropOptions{TTL: time.Hour})
	now := time.Now().UTC().Truncate(time.Millisecond)
	s.Record(dropEvent("BTCUSDT", "old", alertdrop.HTF, now.Add(-2*time.Hour)))
	s.Record(dropEvent("BTCUSDT", "new", alertdrop.HTF, now))
	flushStore(t, s)
	if err := s.prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Recent(context.Background(), alertdrop.Query{Symbol: "BTCUSDT", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Rule != "new" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestCloseDrains(t *testing.T) {
	db := openDropDB(t)
	s := NewAlertDropStore(db, AlertDropOptions{PruneEvery: time.Hour})
	at := time.Date(2026, 11, 1, 7, 0, 0, 0, time.UTC)
	s.Record(dropEvent("AAPL", "ema_cross", alertdrop.Profile, at))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatal("second close")
	}
	rows, err := s.Recent(context.Background(), alertdrop.Query{Symbol: "AAPL"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Count != 1 || !rows[0].LastSeen.Equal(at) {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRecordHotPath(t *testing.T) {
	db := openDropDB(t)
	s := newTestStore(t, db, AlertDropOptions{Buffer: 128})
	ev := dropEvent("BTCUSDT", "ict_fvg", alertdrop.ScoreFloor, time.Now().UTC())
	start := time.Now()
	s.Record(ev)
	if d := time.Since(start); d > 5*time.Millisecond {
		t.Fatalf("Record took %s, budget is 5ms", d)
	}
}

func TestRecordOverflowNonBlocking(t *testing.T) {
	db := openDropDB(t)
	tx, err := db.Conn().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO alert_drops (
		symbol, rule, timeframe, direction, reason, sender,
		score, threshold, detail, count, first_seen, last_seen
	) VALUES ('LOCK','r','1H','LONG','cooldown','',0,0,'',1,'2000-01-01T00:00:00.000Z','2000-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}

	s := NewAlertDropStore(db, AlertDropOptions{Buffer: 4, PruneEvery: time.Hour})
	ev := dropEvent("BTCUSDT", "ict_fvg", alertdrop.DailyLimit, time.Now().UTC())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.overflow.Load() == 0 {
		for i := 0; i < 16; i++ {
			ev.Rule = "r"
			start := time.Now()
			s.Record(ev)
			if d := time.Since(start); d > 5*time.Millisecond {
				t.Fatalf("Record blocked for %s while the buffer was filling", d)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = tx.Rollback()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if s.overflow.Load() == 0 {
		t.Fatal("full buffer did not discard a record")
	}
}

func BenchmarkRecord(b *testing.B) {
	db, err := New(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	s := NewAlertDropStore(db, AlertDropOptions{Buffer: 8192, PruneEvery: time.Hour})
	ev := dropEvent("BTCUSDT", "ict_fvg", alertdrop.Cooldown, time.Now().UTC())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Record(ev)
	}
	b.StopTimer()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		b.Fatal(err)
	}
}
