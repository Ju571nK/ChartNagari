package storage

import (
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSymbolOverrideStore_GetMissing(t *testing.T) {
	db := newTestDB(t)
	store := NewSymbolOverrideStore(db)

	got, err := store.Get("MISSING")
	if err != nil {
		t.Fatalf("Get(MISSING): unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("Get(MISSING) = %#v, want nil", got)
	}
}

func TestSymbolOverrideStore_PutThenGet(t *testing.T) {
	db := newTestDB(t)
	store := NewSymbolOverrideStore(db)

	score := 14.0
	cooldown := 12
	tf := []string{"1D", "1W"}
	in := SymbolOverride{
		Symbol:           "TSLA",
		ScoreThreshold:   &score,
		CooldownHours:    &cooldown,
		AlertLimitPerDay: nil,
		Timeframes:       tf,
		AllowedRules:     nil,
	}
	if err := store.Put(in); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Get("TSLA")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get(TSLA) = nil, want override row")
	}
	if got.ScoreThreshold == nil || *got.ScoreThreshold != 14.0 {
		t.Errorf("ScoreThreshold = %v, want 14.0", got.ScoreThreshold)
	}
	if got.CooldownHours == nil || *got.CooldownHours != 12 {
		t.Errorf("CooldownHours = %v, want 12", got.CooldownHours)
	}
	if got.AlertLimitPerDay != nil {
		t.Errorf("AlertLimitPerDay = %v, want nil", got.AlertLimitPerDay)
	}
	if len(got.Timeframes) != 2 || got.Timeframes[0] != "1D" || got.Timeframes[1] != "1W" {
		t.Errorf("Timeframes = %v, want [1D 1W]", got.Timeframes)
	}
	if got.AllowedRules != nil {
		t.Errorf("AllowedRules = %v, want nil", got.AllowedRules)
	}
	if got.UpdatedAt == 0 {
		t.Error("UpdatedAt = 0, want non-zero unix seconds")
	}
}

func TestSymbolOverrideStore_PutUpsert(t *testing.T) {
	db := newTestDB(t)
	store := NewSymbolOverrideStore(db)

	score1 := 10.0
	score2 := 20.0
	if err := store.Put(SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score1}); err != nil {
		t.Fatalf("Put #1: %v", err)
	}
	if err := store.Put(SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score2}); err != nil {
		t.Fatalf("Put #2: %v", err)
	}
	got, _ := store.Get("TSLA")
	if got.ScoreThreshold == nil || *got.ScoreThreshold != 20.0 {
		t.Errorf("after upsert ScoreThreshold = %v, want 20.0", got.ScoreThreshold)
	}
}

func TestSymbolOverrideStore_Delete(t *testing.T) {
	db := newTestDB(t)
	store := NewSymbolOverrideStore(db)

	score := 14.0
	_ = store.Put(SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score})
	if err := store.Delete("TSLA"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ := store.Get("TSLA")
	if got != nil {
		t.Errorf("Get after Delete = %#v, want nil", got)
	}
}

func TestSymbolOverrideStore_DeleteMissing(t *testing.T) {
	db := newTestDB(t)
	store := NewSymbolOverrideStore(db)

	if err := store.Delete("MISSING"); err != nil {
		t.Errorf("Delete(MISSING): unexpected error %v", err)
	}
}

func TestSymbolOverrideStore_EmptyArraysNormalizedToNil(t *testing.T) {
	db := newTestDB(t)
	store := NewSymbolOverrideStore(db)

	in := SymbolOverride{
		Symbol:       "TSLA",
		Timeframes:   []string{},
		AllowedRules: []string{},
	}
	if err := store.Put(in); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, _ := store.Get("TSLA")
	if got == nil {
		t.Fatal("Get = nil; expected row even when arrays are empty (other fields could be set later)")
	}
	if got.Timeframes != nil {
		t.Errorf("Timeframes = %v, want nil (empty array normalized)", got.Timeframes)
	}
	if got.AllowedRules != nil {
		t.Errorf("AllowedRules = %v, want nil (empty array normalized)", got.AllowedRules)
	}
}
