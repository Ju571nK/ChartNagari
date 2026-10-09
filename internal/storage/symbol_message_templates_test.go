package storage

import (
	"path/filepath"
	"testing"
)

func TestSymbolMessageTemplatesRestartAndOverrideIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewSymbolMessageTemplateStore(db)
	if err := store.Put(SymbolMessageTemplates{Symbol: "EURUSD", Long: "매수 {종목}", Short: "Sell {symbol}"}); err != nil {
		t.Fatal(err)
	}
	if err := NewSymbolOverrideStore(db).Put(SymbolOverride{Symbol: "EURUSD", ScoreThreshold: ptr(15.0)}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// New migrates an existing database again without losing either setting.
	db, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewSymbolMessageTemplateStore(db)
	value, err := store.Get("EURUSD")
	if err != nil || value.Long != "매수 {종목}" || value.Short != "Sell {symbol}" {
		t.Fatalf("restart: %+v %v", value, err)
	}
	if err := NewSymbolOverrideStore(db).Delete("EURUSD"); err != nil {
		t.Fatal(err)
	}
	value, err = store.Get("EURUSD")
	if err != nil || value.Long == "" {
		t.Fatalf("override delete erased template: %+v %v", value, err)
	}
	if err := store.Put(SymbolMessageTemplates{Symbol: "EURUSD"}); err != nil {
		t.Fatal(err)
	}
	value, err = store.Get("EURUSD")
	if err != nil || value.Long != "" || value.Short != "" {
		t.Fatalf("reset: %+v %v", value, err)
	}
	other, err := store.Get("USDJPY")
	if err != nil || other.Symbol != "USDJPY" || other.Long != "" {
		t.Fatalf("other symbol: %+v %v", other, err)
	}
}

func ptr[T any](value T) *T { return &value }
