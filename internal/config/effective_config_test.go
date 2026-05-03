package config

import (
	"reflect"
	"testing"

	"github.com/Ju571nK/Chatter/internal/storage"
)

// fakeOverrideStore lets tests inject pretend override rows.
type fakeOverrideStore struct {
	rows map[string]*storage.SymbolOverride
}

func (f *fakeOverrideStore) Get(symbol string) (*storage.SymbolOverride, error) {
	if f == nil || f.rows == nil {
		return nil, nil
	}
	return f.rows[symbol], nil
}

func TestEffectiveAlertConfig_NoOverride(t *testing.T) {
	holder := NewSymbolProfilesHolder(SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]Profile{
			"p1": {ScoreThreshold: 10, CooldownHours: 6, AlertLimitPerDay: 3,
				Timeframes: []string{"1D"}, AllowedRules: []string{"r1"}},
		},
	})
	store := &fakeOverrideStore{}
	got := EffectiveAlertConfig("BTC", holder, store)

	want := EffectiveConfig{
		ScoreThreshold:   10,
		CooldownHours:    6,
		AlertLimitPerDay: 3,
		Timeframes:       []string{"1D"},
		AllowedRules:     []string{"r1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestEffectiveAlertConfig_PartialOverride(t *testing.T) {
	holder := NewSymbolProfilesHolder(SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]Profile{
			"p1": {ScoreThreshold: 10, CooldownHours: 6, AlertLimitPerDay: 3,
				Timeframes: []string{"1D"}, AllowedRules: []string{"r1"}},
		},
	})
	score := 14.0
	store := &fakeOverrideStore{rows: map[string]*storage.SymbolOverride{
		"BTC": {Symbol: "BTC", ScoreThreshold: &score, Timeframes: []string{"1W"}},
	}}
	got := EffectiveAlertConfig("BTC", holder, store)

	want := EffectiveConfig{
		ScoreThreshold:   14,
		CooldownHours:    6,
		AlertLimitPerDay: 3,
		Timeframes:       []string{"1W"},
		AllowedRules:     []string{"r1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestEffectiveAlertConfig_FullOverride(t *testing.T) {
	holder := NewSymbolProfilesHolder(SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]Profile{
			"p1": {ScoreThreshold: 10, CooldownHours: 6, AlertLimitPerDay: 3},
		},
	})
	score := 20.0
	cooldown := 24
	limit := 1
	store := &fakeOverrideStore{rows: map[string]*storage.SymbolOverride{
		"BTC": {
			Symbol: "BTC",
			ScoreThreshold: &score, CooldownHours: &cooldown, AlertLimitPerDay: &limit,
			Timeframes: []string{"1H", "4H"}, AllowedRules: []string{"x", "y"},
		},
	}}
	got := EffectiveAlertConfig("BTC", holder, store)

	want := EffectiveConfig{
		ScoreThreshold:   20, CooldownHours: 24, AlertLimitPerDay: 1,
		Timeframes:       []string{"1H", "4H"},
		AllowedRules:     []string{"x", "y"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestEffectiveAlertConfig_NilStore(t *testing.T) {
	holder := NewSymbolProfilesHolder(SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]Profile{
			"p1": {ScoreThreshold: 10},
		},
	})
	got := EffectiveAlertConfig("BTC", holder, nil)
	if got.ScoreThreshold != 10 {
		t.Errorf("nil store: got %v, want 10 (profile only)", got.ScoreThreshold)
	}
}

func TestEffectiveAlertConfig_NilHolder(t *testing.T) {
	got := EffectiveAlertConfig("BTC", nil, nil)
	if got.ScoreThreshold != 0 || got.CooldownHours != 0 {
		t.Errorf("nil holder: got %#v, want zero EffectiveConfig", got)
	}
}

func TestEffectiveAlertConfig_SymbolWithoutProfileUsesDefault(t *testing.T) {
	holder := NewSymbolProfilesHolder(SymbolProfilesConfig{
		DefaultProfile: "default",
		Profiles: map[string]Profile{
			"default": {ScoreThreshold: 7},
			"crypto":  {ScoreThreshold: 12},
		},
	})
	got := EffectiveAlertConfig("UNKNOWN", holder, nil)
	if got.ScoreThreshold != 7 {
		t.Errorf("UNKNOWN symbol: got %v, want 7 (default profile)", got.ScoreThreshold)
	}
}
