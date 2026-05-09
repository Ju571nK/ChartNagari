package storage

import (
	"strings"
	"testing"
	"time"
)

// FSM matrix from spec §3.2 / §9.1
//
// from \ action | took | skip | win | loss | be | undo
// (no row)      | TOOK | SKIP | err | err  | err| err
// TOOK          | err  | err  | WIN | LOSS | BE | (delete)
// SKIPPED       | err  | err  | err | err  | err| (delete)
// WIN           | err  | err  | WIN | LOSS | BE | TOOK
// LOSS          | err  | err  | WIN | LOSS | BE | TOOK
// BE            | err  | err  | WIN | LOSS | BE | TOOK

type fsmCase struct {
	from   string // "" means no-row
	action string
	want   string // expected new status, "" for "row deleted", "ERR:..." for expected error substring
}

var fsmCases = []fsmCase{
	{from: "", action: "took", want: "TOOK"},
	{from: "", action: "skip", want: "SKIPPED"},
	{from: "", action: "win", want: "ERR:invalid"},
	{from: "", action: "loss", want: "ERR:invalid"},
	{from: "", action: "be", want: "ERR:invalid"},
	{from: "", action: "undo", want: "ERR:invalid"},

	{from: "TOOK", action: "took", want: "ERR:invalid"},
	{from: "TOOK", action: "skip", want: "ERR:invalid"},
	{from: "TOOK", action: "win", want: "WIN"},
	{from: "TOOK", action: "loss", want: "LOSS"},
	{from: "TOOK", action: "be", want: "BE"},
	{from: "TOOK", action: "undo", want: ""},

	{from: "SKIPPED", action: "took", want: "ERR:invalid"},
	{from: "SKIPPED", action: "skip", want: "ERR:invalid"},
	{from: "SKIPPED", action: "win", want: "ERR:invalid"},
	{from: "SKIPPED", action: "loss", want: "ERR:invalid"},
	{from: "SKIPPED", action: "be", want: "ERR:invalid"},
	{from: "SKIPPED", action: "undo", want: ""},

	{from: "WIN", action: "win", want: "WIN"},
	{from: "WIN", action: "loss", want: "LOSS"},
	{from: "WIN", action: "be", want: "BE"},
	{from: "WIN", action: "undo", want: "TOOK"},
	{from: "LOSS", action: "win", want: "WIN"},
	{from: "LOSS", action: "loss", want: "LOSS"},
	{from: "LOSS", action: "be", want: "BE"},
	{from: "LOSS", action: "undo", want: "TOOK"},
	{from: "BE", action: "win", want: "WIN"},
	{from: "BE", action: "loss", want: "LOSS"},
	{from: "BE", action: "be", want: "BE"},
	{from: "BE", action: "undo", want: "TOOK"},

	{from: "PENDING", action: "took", want: "TOOK"},
	{from: "PENDING", action: "skip", want: "SKIPPED"},
	{from: "PENDING", action: "win", want: "ERR:invalid"},
	{from: "PENDING", action: "loss", want: "ERR:invalid"},
	{from: "PENDING", action: "be", want: "ERR:invalid"},
	{from: "PENDING", action: "undo", want: "ERR:invalid"},
}

// seedSignal inserts a signal so signal_marks FK is satisfied.
// Returns the signal_id.
func seedSignal(t *testing.T, db *DB) int64 {
	t.Helper()
	res, err := db.conn.Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at)
		VALUES ('BTCUSDT','1H','ict_test','LONG',10.0,'msg','',0,0,'',0,0)`)
	if err != nil {
		t.Fatalf("seed signal: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func TestSignalMarkStore_FSM(t *testing.T) {
	for _, tc := range fsmCases {
		name := tc.from
		if name == "" {
			name = "NOROW"
		}
		t.Run(name+"_"+tc.action, func(t *testing.T) {
			db := newTestDB(t)
			store := NewSignalMarkStore(db)
			id := seedSignal(t, db)

			// Seed the from-state if not no-row.
			if tc.from != "" {
				if err := store.directSetStatus(id, tc.from); err != nil {
					t.Fatalf("seed from-state %q: %v", tc.from, err)
				}
			}

			got, err := store.Mark(id, tc.action)
			if strings.HasPrefix(tc.want, "ERR:") {
				if err == nil {
					t.Fatalf("expected error containing %q, got newStatus=%q nil err", tc.want[4:], got)
				}
				if !strings.Contains(err.Error(), tc.want[4:]) {
					t.Errorf("error mismatch: got %q, want substring %q", err.Error(), tc.want[4:])
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("newStatus = %q, want %q", got, tc.want)
			}
			// For "deleted" case, verify row gone.
			if tc.want == "" {
				row, _ := store.Get(id)
				if row != nil {
					t.Errorf("expected deleted row, got %#v", row)
				}
			}
		})
	}
}

func TestSignalMarkStore_GetMissing(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	got, err := store.Get(99999)
	if err != nil {
		t.Fatalf("Get(missing): %v", err)
	}
	if got != nil {
		t.Errorf("Get(missing) = %#v, want nil", got)
	}
}

func TestSignalMarkStore_SetMessageID(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id := seedSignal(t, db)
	if _, err := store.Mark(id, "took"); err != nil {
		t.Fatalf("Mark took: %v", err)
	}
	if err := store.SetMessageID(id, 4242); err != nil {
		t.Fatalf("SetMessageID: %v", err)
	}
	row, _ := store.Get(id)
	if row == nil || row.TgMessageID == nil || *row.TgMessageID != 4242 {
		t.Errorf("TgMessageID mismatch: %#v", row)
	}
}

func TestSignalMarkStore_SetMessageID_StubCreates(t *testing.T) {
	// SetMessageID is sometimes called BEFORE the user marks the signal
	// (right after the alert is sent). It must create a stub PENDING row.
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id := seedSignal(t, db)
	if err := store.SetMessageID(id, 999); err != nil {
		t.Fatalf("SetMessageID stub: %v", err)
	}
	row, _ := store.Get(id)
	if row == nil || row.Status != "PENDING" || row.TgMessageID == nil || *row.TgMessageID != 999 {
		t.Errorf("stub PENDING row not created: %#v", row)
	}
}

func TestSignalMarkStore_StubThenMark(t *testing.T) {
	// Realistic flow: alert sent → SetMessageID (creates stub PENDING)
	// → user taps Took → Mark must accept the transition.
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id := seedSignal(t, db)

	if err := store.SetMessageID(id, 5555); err != nil {
		t.Fatalf("SetMessageID: %v", err)
	}

	got, err := store.Mark(id, "took")
	if err != nil {
		t.Fatalf("Mark took after stub: %v (this is the bug A3 amendment fixes)", err)
	}
	if got != "TOOK" {
		t.Errorf("got %q, want TOOK", got)
	}

	// tg_message_id must persist through the Mark.
	row, _ := store.Get(id)
	if row == nil || row.TgMessageID == nil || *row.TgMessageID != 5555 {
		t.Errorf("tg_message_id lost after Mark: %#v", row)
	}
}

func TestSignalMarkStore_SignalExists(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id := seedSignal(t, db)

	exists, err := store.SignalExists(id)
	if err != nil {
		t.Fatalf("SignalExists: %v", err)
	}
	if !exists {
		t.Errorf("SignalExists(%d) = false, want true", id)
	}

	exists, err = store.SignalExists(999999)
	if err != nil {
		t.Fatalf("SignalExists missing: %v", err)
	}
	if exists {
		t.Errorf("SignalExists(missing) = true, want false")
	}
}

func TestSignalMarkStore_ListPending(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id1 := seedSignal(t, db)
	id2 := seedSignal(t, db)
	id3 := seedSignal(t, db)
	// id1 is marked TOOK, id2/id3 unmarked.
	if _, err := store.Mark(id1, "took"); err != nil {
		t.Fatal(err)
	}
	_ = id2
	_ = id3

	rows, err := store.ListPending(time.Time{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(pending) = %d, want 2 (id2,id3)", len(rows))
	}
	// Each row must have a populated Signal.ID.
	for _, r := range rows {
		if r.Signal.ID == 0 {
			t.Errorf("Signal.ID = 0, expected populated")
		}
		if r.Mark != nil {
			t.Errorf("Mark should be nil for pending: %#v", r.Mark)
		}
	}
}

func TestSignalMarkStore_ListMarked(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	id1 := seedSignal(t, db)
	id2 := seedSignal(t, db)
	_, _ = store.Mark(id1, "took")
	_, _ = store.Mark(id1, "win")
	_, _ = store.Mark(id2, "skip")

	rows, err := store.ListMarked(time.Time{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(marked) = %d, want 2", len(rows))
	}
	statuses := map[string]bool{}
	for _, r := range rows {
		if r.Mark == nil {
			t.Fatalf("Mark must be non-nil for marked rows: %#v", r)
		}
		statuses[r.Mark.Status] = true
		if r.Signal.ID == 0 {
			t.Errorf("Signal.ID = 0, expected populated")
		}
	}
	if !statuses["WIN"] || !statuses["SKIPPED"] {
		t.Errorf("expected WIN and SKIPPED in results, got %v", statuses)
	}
}

func TestSignalMarkStore_ListPending_LimitClamp(t *testing.T) {
	db := newTestDB(t)
	store := NewSignalMarkStore(db)
	for i := 0; i < 10; i++ {
		seedSignal(t, db)
	}
	// limit=0 should fall back to default 50; we only have 10 so all return.
	rows, err := store.ListPending(time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 10 {
		t.Errorf("limit=0 default fallback: got %d, want 10", len(rows))
	}
	// limit=300 clamps to 200 (still all 10 since less than cap).
	rows, _ = store.ListPending(time.Time{}, 300)
	if len(rows) != 10 {
		t.Errorf("limit=300 clamped: got %d, want 10", len(rows))
	}
}
