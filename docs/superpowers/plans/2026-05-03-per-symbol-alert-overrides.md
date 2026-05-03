# Per-Symbol Alert Overrides Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users tune `score_threshold`, `cooldown_hours`, `alert_limit_per_day`, `timeframes`, and `allowed_rules` per symbol via UI, hot-reloaded with no server restart.

**Architecture:** New SQLite table `symbol_alert_overrides` stores per-symbol nullable overrides. A merge function `EffectiveAlertConfig(symbol, profileHolder, overrideStore)` resolves profile + override at signal-evaluation time. The pipeline filter chain calls this on every tick, so DB writes from the UI become live on the next signal — no cache, no restart.

**Tech Stack:** Go 1.26+ (modernc.org/sqlite), zerolog, React 18 + TypeScript + Vite + react-i18next, Vitest + Testing Library.

**Spec:** `docs/superpowers/specs/2026-05-03-per-symbol-alert-overrides-design.md`

**Branch:** `feat/symbol-alert-overrides` (create from `main`)

---

## File Structure

**New files:**

| Path | Responsibility |
|---|---|
| `internal/storage/symbol_overrides.go` | `SymbolOverrideStore` — CRUD over `symbol_alert_overrides` table |
| `internal/storage/symbol_overrides_test.go` | Storage roundtrip tests |
| `internal/config/effective_config.go` | `EffectiveAlertConfig` merge function + `EffectiveConfig` struct |
| `internal/config/effective_config_test.go` | Table-driven merge tests |
| `internal/api/symbol_overrides_handler.go` | HTTP GET/PUT/DELETE handlers |
| `internal/api/symbol_overrides_handler_test.go` | Handler tests (auth, validation, hot-reload) |
| `web/src/SymbolOverrideEditor.tsx` | React editor component used in two surfaces |
| `web/src/SymbolOverrideEditor.test.tsx` | Vitest + Testing Library tests |

**Modified files:**

| Path | Change |
|---|---|
| `internal/storage/db.go` | Add `CREATE TABLE IF NOT EXISTS symbol_alert_overrides ...` to `migrate()` |
| `internal/config/symbol_profiles.go` | Add `Timeframes []string` field to `Profile` struct |
| `internal/pipeline/profile_filter.go` | New `applyEffectiveConfig` and `filterByTimeframe`; rewire `profileScoreThreshold`/`profileCooldownHours` callers |
| `internal/pipeline/pipeline.go:317-323` | Call `EffectiveAlertConfig` instead of `holder.GetProfile`; add timeframe filter step |
| `internal/api/server.go` | Add `overrideStore` field to `Server`, wire constructor, register 3 new routes |
| `cmd/chartnagari/main.go` (or wherever `api.New` is called) | Construct `SymbolOverrideStore` and pass into `api.New` and `pipeline.New` |
| `web/src/App.tsx` (`SymbolsTab`, `ChartTab`) | Add expand chevron + editor in row; add `⚙` button + modal trigger |
| `web/src/i18n/locales/en.json` | 11 new keys under `override.*` |
| `web/src/i18n/locales/ko.json` | Same keys, Korean strings |
| `web/src/i18n/locales/ja.json` | Same keys, Japanese strings |
| `CHANGELOG.md` | New entry under v2.8.0.0 |
| `VERSION` | Bump to 2.8.0.0 |

---

## Phase A — Storage & Merge Logic

### Task A1: Branch + DB schema migration

**Files:**
- Modify: `internal/storage/db.go` (extend `migrate()` schema string)

- [ ] **Step 1: Create branch from main**

```bash
git checkout main
git pull
git checkout -b feat/symbol-alert-overrides
```

- [ ] **Step 2: Add CREATE TABLE to migration string**

In `internal/storage/db.go`, find the `schema := \`...\`` block in `migrate()` (around line 65). Append the following table definition before the closing backtick:

```sql

	CREATE TABLE IF NOT EXISTS symbol_alert_overrides (
		symbol               TEXT PRIMARY KEY,
		score_threshold      REAL,
		cooldown_hours       INTEGER,
		alert_limit_per_day  INTEGER,
		timeframes           TEXT,
		allowed_rules        TEXT,
		updated_at           INTEGER NOT NULL
	);
```

- [ ] **Step 3: Verify schema applies**

Run: `go test ./internal/storage/... -run TestNew -count=1 -race`

Expected: PASS (existing test creates a fresh DB; if the new SQL is valid, it migrates without error).

- [ ] **Step 4: Commit**

```bash
git add internal/storage/db.go
git commit -m "feat(storage): add symbol_alert_overrides table

Empty rows on existing installs; idempotent migration."
```

---

### Task A2: SymbolOverrideStore — failing test

**Files:**
- Create: `internal/storage/symbol_overrides_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/storage/symbol_overrides_test.go` with:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/ -run TestSymbolOverrideStore -count=1`

Expected: FAIL with `undefined: NewSymbolOverrideStore` and `undefined: SymbolOverride`.

---

### Task A3: SymbolOverrideStore — implementation

**Files:**
- Create: `internal/storage/symbol_overrides.go`

- [ ] **Step 1: Write minimal implementation**

Create `internal/storage/symbol_overrides.go`:

```go
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SymbolOverride represents a per-symbol nullable override of alert config.
// Pointer fields are nullable: nil means "inherit from profile".
// Slice fields use nil for "inherit"; empty slices are normalized to nil.
type SymbolOverride struct {
	Symbol           string
	ScoreThreshold   *float64
	CooldownHours    *int
	AlertLimitPerDay *int
	Timeframes       []string
	AllowedRules     []string
	UpdatedAt        int64 // unix seconds; populated on Get
}

// IsEmpty reports whether every field is nil/empty (nothing to override).
// Used by handlers to decide between PUT-as-update and PUT-as-delete.
func (o SymbolOverride) IsEmpty() bool {
	return o.ScoreThreshold == nil &&
		o.CooldownHours == nil &&
		o.AlertLimitPerDay == nil &&
		len(o.Timeframes) == 0 &&
		len(o.AllowedRules) == 0
}

// SymbolOverrideStore performs CRUD on the symbol_alert_overrides table.
type SymbolOverrideStore struct {
	db *DB
}

// NewSymbolOverrideStore creates a store backed by the given DB.
func NewSymbolOverrideStore(db *DB) *SymbolOverrideStore {
	return &SymbolOverrideStore{db: db}
}

// Get returns the override for a symbol, or (nil, nil) when no row exists.
func (s *SymbolOverrideStore) Get(symbol string) (*SymbolOverride, error) {
	if symbol == "" {
		return nil, errors.New("symbol must not be empty")
	}
	row := s.db.conn.QueryRow(`
		SELECT score_threshold, cooldown_hours, alert_limit_per_day,
		       timeframes, allowed_rules, updated_at
		  FROM symbol_alert_overrides
		 WHERE symbol = ?`, symbol)

	var (
		score    sql.NullFloat64
		cooldown sql.NullInt64
		limit    sql.NullInt64
		tfJSON   sql.NullString
		rulesJSON sql.NullString
		updated  int64
	)
	err := row.Scan(&score, &cooldown, &limit, &tfJSON, &rulesJSON, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query symbol_alert_overrides: %w", err)
	}

	out := &SymbolOverride{Symbol: symbol, UpdatedAt: updated}
	if score.Valid {
		v := score.Float64
		out.ScoreThreshold = &v
	}
	if cooldown.Valid {
		v := int(cooldown.Int64)
		out.CooldownHours = &v
	}
	if limit.Valid {
		v := int(limit.Int64)
		out.AlertLimitPerDay = &v
	}
	if tfJSON.Valid && tfJSON.String != "" {
		if err := json.Unmarshal([]byte(tfJSON.String), &out.Timeframes); err != nil {
			return nil, fmt.Errorf("decode timeframes: %w", err)
		}
	}
	if rulesJSON.Valid && rulesJSON.String != "" {
		if err := json.Unmarshal([]byte(rulesJSON.String), &out.AllowedRules); err != nil {
			return nil, fmt.Errorf("decode allowed_rules: %w", err)
		}
	}
	return out, nil
}

// Put upserts an override row.
// Empty slices are normalized to NULL on disk.
func (s *SymbolOverrideStore) Put(o SymbolOverride) error {
	if o.Symbol == "" {
		return errors.New("symbol must not be empty")
	}

	var tfArg, rulesArg interface{}
	if len(o.Timeframes) > 0 {
		b, err := json.Marshal(o.Timeframes)
		if err != nil {
			return fmt.Errorf("encode timeframes: %w", err)
		}
		tfArg = string(b)
	} else {
		tfArg = nil
	}
	if len(o.AllowedRules) > 0 {
		b, err := json.Marshal(o.AllowedRules)
		if err != nil {
			return fmt.Errorf("encode allowed_rules: %w", err)
		}
		rulesArg = string(b)
	} else {
		rulesArg = nil
	}

	var scoreArg, cooldownArg, limitArg interface{}
	if o.ScoreThreshold != nil {
		scoreArg = *o.ScoreThreshold
	}
	if o.CooldownHours != nil {
		cooldownArg = *o.CooldownHours
	}
	if o.AlertLimitPerDay != nil {
		limitArg = *o.AlertLimitPerDay
	}

	_, err := s.db.conn.Exec(`
		INSERT INTO symbol_alert_overrides
		  (symbol, score_threshold, cooldown_hours, alert_limit_per_day,
		   timeframes, allowed_rules, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(symbol) DO UPDATE SET
		  score_threshold     = excluded.score_threshold,
		  cooldown_hours      = excluded.cooldown_hours,
		  alert_limit_per_day = excluded.alert_limit_per_day,
		  timeframes          = excluded.timeframes,
		  allowed_rules       = excluded.allowed_rules,
		  updated_at          = excluded.updated_at`,
		o.Symbol, scoreArg, cooldownArg, limitArg, tfArg, rulesArg, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("upsert symbol_alert_overrides: %w", err)
	}
	return nil
}

// Delete removes the override row for a symbol.
// Returns nil even when no row exists (idempotent).
func (s *SymbolOverrideStore) Delete(symbol string) error {
	if symbol == "" {
		return errors.New("symbol must not be empty")
	}
	_, err := s.db.conn.Exec(`DELETE FROM symbol_alert_overrides WHERE symbol = ?`, symbol)
	if err != nil {
		return fmt.Errorf("delete symbol_alert_overrides: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/storage/ -run TestSymbolOverrideStore -count=1 -race -v`

Expected: All 6 subtests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/storage/symbol_overrides.go internal/storage/symbol_overrides_test.go
git commit -m "feat(storage): SymbolOverrideStore CRUD with nullable fields

Pointer fields and slices both convey 'inherit from profile' via nil.
Empty arrays are normalized to NULL on write to avoid the
'fully-deselected list silently mutes alerts' trap."
```

---

### Task A4: Profile.Timeframes field + EffectiveConfig — failing test

**Files:**
- Modify: `internal/config/symbol_profiles.go` (add `Timeframes` to `Profile`)
- Create: `internal/config/effective_config_test.go`

- [ ] **Step 1: Add field to Profile struct**

In `internal/config/symbol_profiles.go`, modify the `Profile` struct (lines 13–20). Add `Timeframes` between `AllowedRules` and `AlertLimitPerDay`:

```go
type Profile struct {
	AllowedMethodologies []string `yaml:"allowed_methodologies" json:"allowed_methodologies"`
	BlockedMethodologies []string `yaml:"blocked_methodologies" json:"blocked_methodologies"`
	AllowedRules         []string `yaml:"allowed_rules" json:"allowed_rules"`
	Timeframes           []string `yaml:"timeframes,omitempty" json:"timeframes,omitempty"`
	AlertLimitPerDay     int      `yaml:"alert_limit_per_day" json:"alert_limit_per_day"`
	CooldownHours        int      `yaml:"cooldown_hours" json:"cooldown_hours"`
	ScoreThreshold       float64  `yaml:"score_threshold" json:"score_threshold"`
}
```

- [ ] **Step 2: Verify existing tests still pass**

Run: `go test ./internal/config/... -count=1`

Expected: PASS (the new field is optional, so existing YAMLs unmarshal unchanged).

- [ ] **Step 3: Write the failing merge test**

Create `internal/config/effective_config_test.go`:

```go
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
		ScoreThreshold:   14,                  // overridden
		CooldownHours:    6,                   // inherited
		AlertLimitPerDay: 3,                   // inherited
		Timeframes:       []string{"1W"},      // overridden
		AllowedRules:     []string{"r1"},      // inherited
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
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestEffectiveAlertConfig -count=1`

Expected: FAIL with `undefined: EffectiveAlertConfig` and `undefined: EffectiveConfig`.

---

### Task A5: EffectiveAlertConfig implementation

**Files:**
- Create: `internal/config/effective_config.go`

- [ ] **Step 1: Write the implementation**

Create `internal/config/effective_config.go`:

```go
// Package config — effective_config.go
//
// EffectiveAlertConfig merges a YAML profile (system default) with a
// SQLite per-symbol override (user customization) into the final
// AlertConfig that the pipeline filter chain consults on each tick.
package config

import "github.com/Ju571nK/Chatter/internal/storage"

// EffectiveConfig is the resolved alert configuration for a single symbol.
// Empty slices mean "no constraint" (allow all).
type EffectiveConfig struct {
	ScoreThreshold       float64
	CooldownHours        int
	AlertLimitPerDay     int
	Timeframes           []string
	AllowedRules         []string
	AllowedMethodologies []string
	BlockedMethodologies []string
}

// OverrideGetter is the minimal interface EffectiveAlertConfig needs.
// *storage.SymbolOverrideStore satisfies it; tests inject a fake.
type OverrideGetter interface {
	Get(symbol string) (*storage.SymbolOverride, error)
}

// EffectiveAlertConfig resolves the final alert config for a symbol.
// Resolution: start from the profile, then for each non-nil override field,
// replace the profile value with the override value.
//
// Defensive behavior:
//   - holder == nil → returns zero EffectiveConfig (everything passes filters).
//   - store == nil  → profile-only resolution.
//   - store.Get error → logged-and-ignored at the call site, profile-only result.
func EffectiveAlertConfig(symbol string, holder *SymbolProfilesHolder, store OverrideGetter) EffectiveConfig {
	if holder == nil {
		return EffectiveConfig{}
	}
	p := holder.GetProfile(symbol)

	cfg := EffectiveConfig{
		ScoreThreshold:       p.ScoreThreshold,
		CooldownHours:        p.CooldownHours,
		AlertLimitPerDay:     p.AlertLimitPerDay,
		Timeframes:           p.Timeframes,
		AllowedRules:         p.AllowedRules,
		AllowedMethodologies: p.AllowedMethodologies,
		BlockedMethodologies: p.BlockedMethodologies,
	}

	if store == nil {
		return cfg
	}
	ov, err := store.Get(symbol)
	if err != nil || ov == nil {
		return cfg
	}

	if ov.ScoreThreshold != nil {
		cfg.ScoreThreshold = *ov.ScoreThreshold
	}
	if ov.CooldownHours != nil {
		cfg.CooldownHours = *ov.CooldownHours
	}
	if ov.AlertLimitPerDay != nil {
		cfg.AlertLimitPerDay = *ov.AlertLimitPerDay
	}
	if ov.Timeframes != nil {
		cfg.Timeframes = ov.Timeframes
	}
	if ov.AllowedRules != nil {
		cfg.AllowedRules = ov.AllowedRules
	}
	return cfg
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestEffectiveAlertConfig -count=1 -race -v`

Expected: All 6 subtests PASS.

- [ ] **Step 3: Run full config + storage suite to confirm no regression**

Run: `go test ./internal/config/... ./internal/storage/... -count=1 -race`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/config/symbol_profiles.go internal/config/effective_config.go internal/config/effective_config_test.go
git commit -m "feat(config): EffectiveAlertConfig merges profile + override

Profile.Timeframes added (optional YAML field).
Defensive: nil store falls through to profile-only; nil holder
returns zero config so pipelines without profile setup still work."
```

---

## Phase B — Pipeline Integration & Hot-reload

### Task B1: Filter chain integration — failing test

**Files:**
- Modify: `internal/pipeline/profile_filter_test.go` (extend with override-aware cases)

- [ ] **Step 1: Open the existing test file**

Read the current `internal/pipeline/profile_filter_test.go` to understand its helpers (`TestProfileScoreThreshold` etc.). The test you're adding lives in the same package, so it can reuse internal helpers.

- [ ] **Step 2: Append the new tests at the end of the file**

Add these tests after the existing `TestProfileCooldownHours` block:

```go
import (
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// stubOverrideStore mirrors fakeOverrideStore from internal/config tests.
type stubOverrideStore struct {
	rows map[string]*storage.SymbolOverride
}

func (s *stubOverrideStore) Get(symbol string) (*storage.SymbolOverride, error) {
	if s == nil || s.rows == nil {
		return nil, nil
	}
	return s.rows[symbol], nil
}

func TestEffectiveScoreThreshold_OverrideWins(t *testing.T) {
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]appconfig.Profile{
			"p1": {ScoreThreshold: 10},
		},
	})
	score := 14.0
	store := &stubOverrideStore{rows: map[string]*storage.SymbolOverride{
		"TSLA": {Symbol: "TSLA", ScoreThreshold: &score},
	}}

	cfg := appconfig.EffectiveAlertConfig("TSLA", holder, store)
	if cfg.ScoreThreshold != 14.0 {
		t.Errorf("TSLA score threshold = %v, want 14.0 (override)", cfg.ScoreThreshold)
	}

	cfg2 := appconfig.EffectiveAlertConfig("AAPL", holder, store)
	if cfg2.ScoreThreshold != 10.0 {
		t.Errorf("AAPL score threshold = %v, want 10.0 (profile)", cfg2.ScoreThreshold)
	}
}

func TestFilterByTimeframe_DropsDisallowed(t *testing.T) {
	signals := []models.Signal{
		{Rule: "r1", Timeframe: "1H"},
		{Rule: "r1", Timeframe: "4H"},
		{Rule: "r1", Timeframe: "1D"},
		{Rule: "r1", Timeframe: "1W"},
	}
	out := filterByTimeframe(signals, []string{"1D", "1W"})
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].Timeframe != "1D" || out[1].Timeframe != "1W" {
		t.Errorf("out = %v, want [1D, 1W] only", out)
	}
}

func TestFilterByTimeframe_EmptyAllowsAll(t *testing.T) {
	signals := []models.Signal{
		{Timeframe: "1H"}, {Timeframe: "4H"}, {Timeframe: "1D"},
	}
	out := filterByTimeframe(signals, nil)
	if len(out) != 3 {
		t.Errorf("nil filter: len(out) = %d, want 3", len(out))
	}
	out = filterByTimeframe(signals, []string{})
	if len(out) != 3 {
		t.Errorf("empty filter: len(out) = %d, want 3", len(out))
	}
}

func TestEffectiveScoreThreshold_HotReload(t *testing.T) {
	// Simulates: pipeline reads → user changes via UI → pipeline reads again.
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]appconfig.Profile{
			"p1": {ScoreThreshold: 5},
		},
	})

	// Use a real DB so we can mutate it mid-test.
	db, err := storage.New(t.TempDir() + "/hr.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := storage.NewSymbolOverrideStore(db)

	first := appconfig.EffectiveAlertConfig("TSLA", holder, store)
	if first.ScoreThreshold != 5 {
		t.Fatalf("initial score = %v, want 5", first.ScoreThreshold)
	}

	score := 18.0
	if err := store.Put(storage.SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score}); err != nil {
		t.Fatal(err)
	}

	second := appconfig.EffectiveAlertConfig("TSLA", holder, store)
	if second.ScoreThreshold != 18.0 {
		t.Errorf("after override: score = %v, want 18.0 (hot-reload failed)", second.ScoreThreshold)
	}
}
```

(If imports already exist in the file, only add the new ones; otherwise the test file will declare them all at the top.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/pipeline/ -count=1`

Expected: FAIL with `undefined: filterByTimeframe`.

---

### Task B2: filterByTimeframe + pipeline rewire

**Files:**
- Modify: `internal/pipeline/profile_filter.go` (add `filterByTimeframe`)
- Modify: `internal/pipeline/pipeline.go` (use `EffectiveAlertConfig`, wire override store)

- [ ] **Step 1: Add filterByTimeframe to profile_filter.go**

Append to `internal/pipeline/profile_filter.go` (do not remove existing functions yet — we'll prune in step 3):

```go
// filterByTimeframe keeps only signals whose Timeframe is in the allowed list.
// An empty or nil allowed list means "allow all" (no filtering).
func filterByTimeframe(signals []models.Signal, allowed []string) []models.Signal {
	if len(allowed) == 0 {
		return signals
	}
	allowSet := make(map[string]struct{}, len(allowed))
	for _, tf := range allowed {
		allowSet[tf] = struct{}{}
	}
	out := make([]models.Signal, 0, len(signals))
	for _, sig := range signals {
		if _, ok := allowSet[sig.Timeframe]; ok {
			out = append(out, sig)
		}
	}
	return out
}
```

- [ ] **Step 2: Inspect pipeline.go to find the override store wiring point**

Run: `grep -n "p.profileHolder\|profileHolder\|filterByProfile" internal/pipeline/pipeline.go`

Note the line numbers. Then read the `Pipeline` struct definition (typically near the top of the file) and the `New`/constructor function.

- [ ] **Step 3: Add overrideStore field to Pipeline struct**

In `internal/pipeline/pipeline.go`, find the `type Pipeline struct { ... }` definition. Add a new field:

```go
overrideStore appconfig.OverrideGetter // nil-safe; nil → profile-only
```

Find the `Pipeline` constructor (likely `func New(...)` or similar). Add a new parameter `overrideStore appconfig.OverrideGetter` and store it in the struct.

If there is a builder/setter pattern instead, add a `WithOverrideStore(store appconfig.OverrideGetter) *Pipeline` method.

- [ ] **Step 4: Replace profile filter call site**

In `internal/pipeline/pipeline.go` around lines 317–323 (the `// Profile filter:` block), replace:

```go
		// Profile filter: remove signals not allowed by the symbol's profile.
		if p.profileHolder != nil {
			beforeProfile := len(signals)
			signals = filterByProfile(signals, p.profileHolder, sym)
			if filtered := beforeProfile - len(signals); filtered > 0 {
				p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Msg("profile filter removed disallowed signals")
			}
		}
```

with:

```go
		// Effective alert config (profile + per-symbol override).
		effCfg := appconfig.EffectiveAlertConfig(sym, p.profileHolder, p.overrideStore)

		// Profile/override filter: remove signals not allowed by methodology/rule.
		if p.profileHolder != nil {
			beforeProfile := len(signals)
			signals = filterByProfile(signals, p.profileHolder, sym)
			if filtered := beforeProfile - len(signals); filtered > 0 {
				p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Msg("profile filter removed disallowed signals")
			}
		}

		// Timeframe filter (override-driven; empty list = allow all).
		if len(effCfg.Timeframes) > 0 {
			beforeTF := len(signals)
			signals = filterByTimeframe(signals, effCfg.Timeframes)
			if filtered := beforeTF - len(signals); filtered > 0 {
				p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Strs("allowed_tf", effCfg.Timeframes).Msg("timeframe filter removed signals")
			}
		}

		// Override-aware allowed_rules filter (when override sets a non-empty list).
		if len(effCfg.AllowedRules) > 0 && p.overrideHasRulesOverride(sym) {
			beforeRules := len(signals)
			allowedSet := make(map[string]struct{}, len(effCfg.AllowedRules))
			for _, r := range effCfg.AllowedRules {
				allowedSet[r] = struct{}{}
			}
			kept := signals[:0]
			for _, sig := range signals {
				if _, ok := allowedSet[sig.Rule]; ok {
					kept = append(kept, sig)
				}
			}
			signals = kept
			if filtered := beforeRules - len(signals); filtered > 0 {
				p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Msg("override allowed_rules filter removed signals")
			}
		}
```

Add a small helper method on `Pipeline`:

```go
// overrideHasRulesOverride reports whether the symbol has an explicit allowed_rules
// override in the DB (so we don't double-filter when only the profile defines it,
// since filterByProfile already handles that case).
func (p *Pipeline) overrideHasRulesOverride(symbol string) bool {
	if p.overrideStore == nil {
		return false
	}
	ov, err := p.overrideStore.Get(symbol)
	if err != nil || ov == nil {
		return false
	}
	return ov.AllowedRules != nil
}
```

(The two-step `filterByProfile` + `overrideHasRulesOverride` exists because `filterByProfile` already considers the profile's `allowed_rules`. The override-side filter only fires when the user explicitly set a non-null list, so it cleanly *replaces* the profile's restriction.)

- [ ] **Step 5: Update existing callers of profileScoreThreshold / profileCooldownHours**

Run: `grep -n "profileScoreThreshold\|profileCooldownHours" internal/pipeline/`

For every non-test occurrence, replace the call with the corresponding `effCfg.ScoreThreshold` or `effCfg.CooldownHours` from the closest `EffectiveAlertConfig` call. Hoist the `effCfg` computation if needed so it is visible at the call site.

- [ ] **Step 6: Update Pipeline construction site**

Run: `grep -rn "pipeline.New\b" cmd/ internal/ | grep -v _test.go`

For each construction site, pass the new `overrideStore` argument. The argument value is constructed in the main entry point — see Task B3.

- [ ] **Step 7: Run pipeline tests**

Run: `go test ./internal/pipeline/ -count=1 -race -v`

Expected: All tests PASS, including the four new ones from Task B1.

- [ ] **Step 8: Commit**

```bash
git add internal/pipeline/profile_filter.go internal/pipeline/profile_filter_test.go internal/pipeline/pipeline.go
git commit -m "feat(pipeline): EffectiveAlertConfig + timeframe/rules override filters

Per-tick DB read for hot-reload. Override allowed_rules filter only
runs when user explicitly set a non-null list, avoiding double-work
with the existing profile-side filter."
```

---

### Task B3: Wire SymbolOverrideStore in main entry point

**Files:**
- Modify: the `main.go` (or equivalent) that constructs `Pipeline` and `api.Server`

- [ ] **Step 1: Locate the construction site**

Run: `grep -rn "api.New\|pipeline.New\b\|storage.New(" cmd/`

Identify the file where both `*storage.DB` is created and `Pipeline`/`Server` are wired. This is typically `cmd/chartnagari/main.go`.

- [ ] **Step 2: Construct SymbolOverrideStore once and inject**

After the line where `db, err := storage.New(...)` succeeds, add:

```go
overrideStore := storage.NewSymbolOverrideStore(db)
```

Pass `overrideStore` to the pipeline constructor (new arg added in Task B2) and to the API server constructor (new arg added in Task C2).

- [ ] **Step 3: Build the binary to confirm wiring compiles**

Run: `go build ./...`

Expected: no errors.

- [ ] **Step 4: Run full Go test suite**

Run: `go test ./... -count=1 -race`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat(main): wire SymbolOverrideStore into pipeline + api"
```

---

## Phase C — HTTP API

### Task C1: Handler test scaffolding

**Files:**
- Create: `internal/api/symbol_overrides_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/symbol_overrides_handler_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
)

// newTestServer builds a minimal Server wired with a real DB + override store.
// apiToken=="" disables auth.
func newTestServer(t *testing.T, apiToken string) (*Server, *storage.SymbolOverrideStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := storage.NewSymbolOverrideStore(db)
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]appconfig.Profile{
			"p1": {ScoreThreshold: 10, CooldownHours: 6, AlertLimitPerDay: 3,
				Timeframes: []string{"1D"}, AllowedRules: []string{"rsi_overbought_oversold"}},
		},
	})

	s := &Server{
		apiToken:               apiToken,
		profileHolder:          holder,
		overrideStore:          store,
		validRuleNames:         map[string]struct{}{"rsi_overbought_oversold": {}, "ict_order_block": {}},
	}
	return s, store
}

func TestGetSymbolOverride_Empty(t *testing.T) {
	s, _ := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/symbol-overrides/TSLA", nil)
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.getSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["symbol"] != "TSLA" {
		t.Errorf("symbol = %v, want TSLA", got["symbol"])
	}
	if got["score_threshold"] != nil {
		t.Errorf("score_threshold = %v, want nil", got["score_threshold"])
	}
}

func TestPutSymbolOverride_ValidThenGet(t *testing.T) {
	s, store := newTestServer(t, "")
	body := []byte(`{
		"score_threshold": 14.0,
		"cooldown_hours": null,
		"alert_limit_per_day": null,
		"timeframes": ["1D","1W"],
		"allowed_rules": null
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	// Confirm DB row was persisted.
	got, _ := store.Get("TSLA")
	if got == nil || got.ScoreThreshold == nil || *got.ScoreThreshold != 14.0 {
		t.Errorf("DB row mismatch: %#v", got)
	}

	// Response shape: {field: {value, source}, ...}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	scoreEntry, ok := resp["score_threshold"].(map[string]any)
	if !ok {
		t.Fatalf("score_threshold entry missing or wrong shape: %v", resp["score_threshold"])
	}
	if scoreEntry["source"] != "override" {
		t.Errorf("score_threshold.source = %v, want override", scoreEntry["source"])
	}
	if scoreEntry["value"].(float64) != 14.0 {
		t.Errorf("score_threshold.value = %v, want 14.0", scoreEntry["value"])
	}
	cooldownEntry := resp["cooldown_hours"].(map[string]any)
	if cooldownEntry["source"] != "profile" {
		t.Errorf("cooldown_hours.source = %v, want profile", cooldownEntry["source"])
	}
}

func TestPutSymbolOverride_AllNullDeletesRow(t *testing.T) {
	s, store := newTestServer(t, "")
	score := 14.0
	_ = store.Put(storage.SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score})

	body := []byte(`{
		"score_threshold": null, "cooldown_hours": null, "alert_limit_per_day": null,
		"timeframes": null, "allowed_rules": null
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	got, _ := store.Get("TSLA")
	if got != nil {
		t.Errorf("row not deleted on all-null PUT: %#v", got)
	}
}

func TestPutSymbolOverride_ScoreOutOfRange(t *testing.T) {
	s, _ := newTestServer(t, "")
	body := []byte(`{"score_threshold": 999.0}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Errorf("missing error message in response")
	}
}

func TestPutSymbolOverride_UnknownTimeframe(t *testing.T) {
	s, _ := newTestServer(t, "")
	body := []byte(`{"timeframes": ["1H","30m"]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPutSymbolOverride_UnknownRule(t *testing.T) {
	s, _ := newTestServer(t, "")
	body := []byte(`{"allowed_rules": ["foo_bar"]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPutSymbolOverride_AuthRequired(t *testing.T) {
	s, _ := newTestServer(t, "secret-token")
	body := []byte(`{"score_threshold": 14.0}`)

	// No header → 401
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", w.Code)
	}

	// Valid header → 200
	req = httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	s.putSymbolOverride(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("with token: status = %d, want 200", w.Code)
	}
}

func TestDeleteSymbolOverride(t *testing.T) {
	s, store := newTestServer(t, "")
	score := 14.0
	_ = store.Put(storage.SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score})

	req := httptest.NewRequest(http.MethodDelete, "/api/symbol-overrides/TSLA", nil)
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.deleteSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	if got, _ := store.Get("TSLA"); got != nil {
		t.Errorf("row still present after delete")
	}

	// Response: every field must have source=profile.
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	scoreEntry := resp["score_threshold"].(map[string]any)
	if scoreEntry["source"] != "profile" {
		t.Errorf("after delete: source = %v, want profile", scoreEntry["source"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestGetSymbolOverride -count=1`

Expected: FAIL with `s.getSymbolOverride undefined`, `s.overrideStore undefined`, `s.validRuleNames undefined`, `s.putSymbolOverride undefined`, `s.deleteSymbolOverride undefined`.

---

### Task C2: Server struct fields + handler implementation

**Files:**
- Modify: `internal/api/server.go` (add struct fields, constructor params, route registration)
- Create: `internal/api/symbol_overrides_handler.go`

- [ ] **Step 1: Add fields to Server struct**

In `internal/api/server.go`, find `type Server struct { ... }`. Add these fields:

```go
overrideStore  *storage.SymbolOverrideStore
validRuleNames map[string]struct{} // populated at New() from rules.yaml registry
```

Also ensure `import "github.com/Ju571nK/Chatter/internal/storage"` is present.

- [ ] **Step 2: Add a parameter to the Server constructor**

Locate `func New(...)` in `internal/api/server.go`. Add `overrideStore *storage.SymbolOverrideStore` and `validRuleNames map[string]struct{}` as parameters. Store them on the returned `*Server`.

If the constructor signature is already long, prefer a small functional-options pattern: add `WithOverrideStore` and `WithValidRules` setters and call them from `cmd/chartnagari/main.go`.

- [ ] **Step 3: Create the handler file**

Create `internal/api/symbol_overrides_handler.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
)

// symbolOverrideRequest is the wire format for PUT.
// All fields are pointers to distinguish "not in body" from "explicit null".
// In Go's encoding/json, a JSON null decodes to a nil pointer; a missing key
// also leaves a nil pointer. We accept both as "no override / inherit".
type symbolOverrideRequest struct {
	ScoreThreshold   *float64  `json:"score_threshold"`
	CooldownHours    *int      `json:"cooldown_hours"`
	AlertLimitPerDay *int      `json:"alert_limit_per_day"`
	Timeframes       *[]string `json:"timeframes"`
	AllowedRules     *[]string `json:"allowed_rules"`
}

// fieldSource is the wire format for each field in the merged response.
type fieldSource struct {
	Value  any    `json:"value"`
	Source string `json:"source"` // "override" | "profile"
}

// effectiveResponse is the body returned by PUT and DELETE.
type effectiveResponse struct {
	Symbol            string      `json:"symbol"`
	ScoreThreshold    fieldSource `json:"score_threshold"`
	CooldownHours     fieldSource `json:"cooldown_hours"`
	AlertLimitPerDay  fieldSource `json:"alert_limit_per_day"`
	Timeframes        fieldSource `json:"timeframes"`
	AllowedRules      fieldSource `json:"allowed_rules"`
}

// validTimeframes is the closed set of accepted timeframe values.
var validTimeframes = map[string]struct{}{
	"1H": {}, "4H": {}, "1D": {}, "1W": {},
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// getSymbolOverride handles GET /api/symbol-overrides/{symbol}.
func (s *Server) getSymbolOverride(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol path parameter required")
		return
	}

	ov, err := s.overrideStore.Get(symbol)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("get override: %v", err))
		return
	}

	out := map[string]any{"symbol": symbol}
	if ov == nil {
		out["score_threshold"] = nil
		out["cooldown_hours"] = nil
		out["alert_limit_per_day"] = nil
		out["timeframes"] = nil
		out["allowed_rules"] = nil
		out["updated_at"] = nil
	} else {
		if ov.ScoreThreshold != nil { out["score_threshold"] = *ov.ScoreThreshold } else { out["score_threshold"] = nil }
		if ov.CooldownHours != nil  { out["cooldown_hours"] = *ov.CooldownHours }   else { out["cooldown_hours"] = nil }
		if ov.AlertLimitPerDay != nil { out["alert_limit_per_day"] = *ov.AlertLimitPerDay } else { out["alert_limit_per_day"] = nil }
		out["timeframes"] = ov.Timeframes   // nil → JSON null
		out["allowed_rules"] = ov.AllowedRules
		out["updated_at"] = ov.UpdatedAt
	}
	jsonOK(w, out)
}

// putSymbolOverride handles PUT /api/symbol-overrides/{symbol}.
func (s *Server) putSymbolOverride(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol path parameter required")
		return
	}

	var req symbolOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ov, err := s.requestToOverride(symbol, req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	// All-null body → DELETE the row instead of writing one with no values.
	if ov.IsEmpty() {
		if err := s.overrideStore.Delete(symbol); err != nil {
			writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("delete override: %v", err))
			return
		}
	} else {
		if err := s.overrideStore.Put(ov); err != nil {
			writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("put override: %v", err))
			return
		}
	}

	jsonOK(w, s.buildEffectiveResponse(symbol))
}

// deleteSymbolOverride handles DELETE /api/symbol-overrides/{symbol}.
func (s *Server) deleteSymbolOverride(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol path parameter required")
		return
	}
	if err := s.overrideStore.Delete(symbol); err != nil {
		writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("delete override: %v", err))
		return
	}
	jsonOK(w, s.buildEffectiveResponse(symbol))
}

// requestToOverride validates and converts the wire request into a SymbolOverride.
// Empty arrays in the request are normalized to nil (== inherit) before storage.
func (s *Server) requestToOverride(symbol string, req symbolOverrideRequest) (storage.SymbolOverride, error) {
	out := storage.SymbolOverride{Symbol: symbol}

	if req.ScoreThreshold != nil {
		v := *req.ScoreThreshold
		if v < 0 || v > 50 {
			return out, errors.New("invalid score_threshold: must be 0~50")
		}
		out.ScoreThreshold = &v
	}
	if req.CooldownHours != nil {
		v := *req.CooldownHours
		if v < 0 || v > 168 {
			return out, errors.New("invalid cooldown_hours: must be 0~168")
		}
		out.CooldownHours = &v
	}
	if req.AlertLimitPerDay != nil {
		v := *req.AlertLimitPerDay
		if v < 0 || v > 100 {
			return out, errors.New("invalid alert_limit_per_day: must be 0~100")
		}
		out.AlertLimitPerDay = &v
	}
	if req.Timeframes != nil && len(*req.Timeframes) > 0 {
		seen := make(map[string]struct{}, len(*req.Timeframes))
		for _, tf := range *req.Timeframes {
			if _, ok := validTimeframes[tf]; !ok {
				return out, fmt.Errorf("invalid timeframe: %q (must be one of 1H,4H,1D,1W)", tf)
			}
			if _, dup := seen[tf]; dup {
				return out, fmt.Errorf("duplicate timeframe: %q", tf)
			}
			seen[tf] = struct{}{}
		}
		out.Timeframes = *req.Timeframes
	}
	if req.AllowedRules != nil && len(*req.AllowedRules) > 0 {
		for _, r := range *req.AllowedRules {
			if _, ok := s.validRuleNames[r]; !ok {
				return out, fmt.Errorf("unknown rule: %q", r)
			}
		}
		out.AllowedRules = *req.AllowedRules
	}
	return out, nil
}

// buildEffectiveResponse computes the merged effective config and emits it
// with per-field provenance ("override" | "profile").
func (s *Server) buildEffectiveResponse(symbol string) effectiveResponse {
	cfg := appconfig.EffectiveAlertConfig(symbol, s.profileHolder, s.overrideStore)
	ov, _ := s.overrideStore.Get(symbol) // ignore err — provenance reverts to "profile"

	src := func(overrideHasIt bool) string {
		if overrideHasIt {
			return "override"
		}
		return "profile"
	}

	hasScore := ov != nil && ov.ScoreThreshold != nil
	hasCooldown := ov != nil && ov.CooldownHours != nil
	hasLimit := ov != nil && ov.AlertLimitPerDay != nil
	hasTF := ov != nil && ov.Timeframes != nil
	hasRules := ov != nil && ov.AllowedRules != nil

	return effectiveResponse{
		Symbol:           symbol,
		ScoreThreshold:   fieldSource{Value: cfg.ScoreThreshold, Source: src(hasScore)},
		CooldownHours:    fieldSource{Value: cfg.CooldownHours, Source: src(hasCooldown)},
		AlertLimitPerDay: fieldSource{Value: cfg.AlertLimitPerDay, Source: src(hasLimit)},
		Timeframes:       fieldSource{Value: cfg.Timeframes, Source: src(hasTF)},
		AllowedRules:     fieldSource{Value: cfg.AllowedRules, Source: src(hasRules)},
	}
}
```

- [ ] **Step 4: Register the routes**

In `internal/api/server.go`, locate the `Handler()` function (around line 384) where `mux.HandleFunc(...)` calls live. Add three new lines next to the symbol/profile group:

```go
mux.HandleFunc("GET /api/symbol-overrides/{symbol}",    s.getSymbolOverride)
mux.HandleFunc("PUT /api/symbol-overrides/{symbol}",    s.putSymbolOverride)
mux.HandleFunc("DELETE /api/symbol-overrides/{symbol}", s.deleteSymbolOverride)
```

- [ ] **Step 5: Populate validRuleNames at startup**

In `cmd/chartnagari/main.go` (or wherever `api.New` is called), build the `validRuleNames` set from the loaded `engine.RuleConfig`:

```go
validRules := make(map[string]struct{}, len(ruleCfg.Rules))
for name := range ruleCfg.Rules {
    validRules[name] = struct{}{}
}
```

Pass `validRules` (and `overrideStore` from Task B3) into the `api.New(...)` call.

- [ ] **Step 6: Run handler tests**

Run: `go test ./internal/api/ -run "TestGetSymbolOverride|TestPutSymbolOverride|TestDeleteSymbolOverride" -count=1 -race -v`

Expected: All 8 subtests PASS.

- [ ] **Step 7: Run full Go suite**

Run: `go test ./... -count=1 -race`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/api/server.go internal/api/symbol_overrides_handler.go internal/api/symbol_overrides_handler_test.go cmd/
git commit -m "feat(api): GET/PUT/DELETE /api/symbol-overrides/{symbol}

Validates ranges + rule names + timeframe set. Auto-deletes the row
when every field is null. Response includes per-field provenance so
the UI can show 'override active' vs 'profile default' badges."
```

---

## Phase D — Frontend + i18n

### Task D1: i18n keys (en/ko/ja)

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/ko.json`
- Modify: `web/src/i18n/locales/ja.json`

- [ ] **Step 1: Add to en.json**

Insert into the existing JSON object (alphabetical order if the file uses it; otherwise append before the closing brace):

```json
  "override": {
    "score_threshold": "Score threshold",
    "cooldown_hours": "Cooldown (hours)",
    "alert_limit": "Daily alert limit",
    "timeframes": "Timeframes",
    "allowed_rules": "Allowed rules",
    "profile_default": "Profile default",
    "profile_default_value": "Profile default: {{value}}",
    "reset": "Reset",
    "reset_all": "Reset all to profile",
    "saved_ago": "Saved ✓ {{n}}s ago",
    "save_failed": "Save failed",
    "tooltip_blank": "Leave blank to use the profile default."
  },
```

- [ ] **Step 2: Add to ko.json**

```json
  "override": {
    "score_threshold": "점수 임계값",
    "cooldown_hours": "쿨다운 (시간)",
    "alert_limit": "일일 알람 한도",
    "timeframes": "타임프레임",
    "allowed_rules": "사용 룰",
    "profile_default": "프로파일 기본값",
    "profile_default_value": "프로파일 기본값: {{value}}",
    "reset": "초기화",
    "reset_all": "프로파일로 전체 초기화",
    "saved_ago": "{{n}}초 전 저장됨",
    "save_failed": "저장 실패",
    "tooltip_blank": "잘 모르겠으면 비워두세요 — 프로파일 기본값을 따릅니다."
  },
```

- [ ] **Step 3: Add to ja.json**

```json
  "override": {
    "score_threshold": "スコア閾値",
    "cooldown_hours": "クールダウン (時間)",
    "alert_limit": "1日あたりのアラート上限",
    "timeframes": "タイムフレーム",
    "allowed_rules": "使用ルール",
    "profile_default": "プロファイルのデフォルト",
    "profile_default_value": "プロファイルのデフォルト: {{value}}",
    "reset": "リセット",
    "reset_all": "プロファイルに全リセット",
    "saved_ago": "{{n}}秒前に保存",
    "save_failed": "保存に失敗",
    "tooltip_blank": "わからない場合は空欄のままにしてください — プロファイルのデフォルトに従います。"
  },
```

- [ ] **Step 4: Verify JSON validity**

Run: `node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/en.json','utf8'))" && node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/ko.json','utf8'))" && node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/ja.json','utf8'))"`

Expected: no output, exit 0.

- [ ] **Step 5: Verify key parity**

Run: `bash scripts/check_i18n.sh` if it exists; otherwise `node -e "const a=Object.keys(require('./web/src/i18n/locales/en.json').override).sort();const b=Object.keys(require('./web/src/i18n/locales/ko.json').override).sort();const c=Object.keys(require('./web/src/i18n/locales/ja.json').override).sort();console.log(JSON.stringify({en:a,ko:b,ja:c}));"`

Expected: all three arrays identical.

- [ ] **Step 6: Commit**

```bash
git add web/src/i18n/locales/
git commit -m "i18n(override): add 12 keys for symbol override editor (en/ko/ja)"
```

---

### Task D2: SymbolOverrideEditor component — failing test

**Files:**
- Create: `web/src/SymbolOverrideEditor.test.tsx`

- [ ] **Step 1: Write the failing tests**

Create `web/src/SymbolOverrideEditor.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import i18n from './i18n'
import { SymbolOverrideEditor } from './SymbolOverrideEditor'

const profile = {
  name: 'large_cap_stock',
  allowed_methodologies: [],
  blocked_methodologies: [],
  allowed_rules: ['ict_order_block'],
  alert_limit_per_day: 2,
  cooldown_hours: 8,
  score_threshold: 10,
}

const wrap = (ui: React.ReactElement) => (
  <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
)

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(global, 'fetch').mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({
      symbol: 'TSLA',
      score_threshold: { value: 10, source: 'profile' },
      cooldown_hours: { value: 8, source: 'profile' },
      alert_limit_per_day: { value: 2, source: 'profile' },
      timeframes: { value: [], source: 'profile' },
      allowed_rules: { value: ['ict_order_block'], source: 'profile' },
    }),
  } as Response)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('SymbolOverrideEditor', () => {
  it('renders profile default values when no override exists', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    // Wait for the initial GET to resolve.
    await vi.runAllTimersAsync()
    expect(screen.getByText(/Score threshold/i)).toBeDefined()
    // The score slider should reflect 10 (profile default).
    const slider = screen.getByLabelText(/Score threshold/i) as HTMLInputElement
    expect(Number(slider.value)).toBe(10)
  })

  it('debounces PUT 500ms after slider change', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    await vi.runAllTimersAsync()

    const slider = screen.getByLabelText(/Score threshold/i) as HTMLInputElement
    fireEvent.change(slider, { target: { value: '14' } })

    // No PUT yet (still inside debounce window).
    expect(global.fetch).toHaveBeenCalledTimes(1) // initial GET only
    vi.advanceTimersByTime(499)
    expect(global.fetch).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(2)
    await vi.runAllTimersAsync()

    expect(global.fetch).toHaveBeenCalledTimes(2)
    const putCall = (global.fetch as ReturnType<typeof vi.fn>).mock.calls[1]
    expect(putCall[0]).toBe('/api/symbol-overrides/TSLA')
    expect(putCall[1].method).toBe('PUT')
    const body = JSON.parse(putCall[1].body as string)
    expect(body.score_threshold).toBe(14)
  })

  it('reset button sends null for that field via PUT', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    await vi.runAllTimersAsync()

    // Set a value first so reset has effect.
    const slider = screen.getByLabelText(/Score threshold/i) as HTMLInputElement
    fireEvent.change(slider, { target: { value: '14' } })
    vi.advanceTimersByTime(501)
    await vi.runAllTimersAsync()

    // Click the reset button next to the score field.
    const resetBtn = screen.getByTestId('reset-score_threshold')
    fireEvent.click(resetBtn)
    vi.advanceTimersByTime(501)
    await vi.runAllTimersAsync()

    const lastCall = (global.fetch as ReturnType<typeof vi.fn>).mock.calls.at(-1)!
    const body = JSON.parse(lastCall[1].body as string)
    expect(body.score_threshold).toBeNull()
  })

  it('reset-all button sends DELETE', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    await vi.runAllTimersAsync()

    const resetAll = screen.getByTestId('reset-all')
    fireEvent.click(resetAll)
    await vi.runAllTimersAsync()

    const lastCall = (global.fetch as ReturnType<typeof vi.fn>).mock.calls.at(-1)!
    expect(lastCall[1].method).toBe('DELETE')
    expect(lastCall[0]).toBe('/api/symbol-overrides/TSLA')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bun test SymbolOverrideEditor`

Expected: FAIL — `SymbolOverrideEditor` not found / module missing.

---

### Task D3: SymbolOverrideEditor component — implementation

**Files:**
- Create: `web/src/SymbolOverrideEditor.tsx`

- [ ] **Step 1: Implement the component**

Create `web/src/SymbolOverrideEditor.tsx`:

```tsx
import { useEffect, useRef, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

const TIMEFRAMES = ['1H', '4H', '1D', '1W'] as const
type Timeframe = typeof TIMEFRAMES[number]

interface ProfileInfo {
  name: string
  allowed_methodologies: string[]
  blocked_methodologies: string[]
  allowed_rules: string[]
  alert_limit_per_day: number
  cooldown_hours: number
  score_threshold: number
}

interface FieldSource<T> {
  value: T
  source: 'override' | 'profile'
}

interface EffectiveResponse {
  symbol: string
  score_threshold: FieldSource<number>
  cooldown_hours: FieldSource<number>
  alert_limit_per_day: FieldSource<number>
  timeframes: FieldSource<string[] | null>
  allowed_rules: FieldSource<string[] | null>
}

interface OverrideState {
  score_threshold: number | null
  cooldown_hours: number | null
  alert_limit_per_day: number | null
  timeframes: Timeframe[] | null
  allowed_rules: string[] | null
}

interface Props {
  symbol: string
  profile: ProfileInfo
  apiToken?: string
}

const DEBOUNCE_MS = 500

async function apiFetch<T>(path: string, init?: RequestInit, token?: string): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> | undefined),
  }
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(path, { ...init, headers })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json() as Promise<T>
}

export function SymbolOverrideEditor({ symbol, profile, apiToken }: Props) {
  const { t } = useTranslation()
  const [state, setState] = useState<OverrideState>({
    score_threshold: null,
    cooldown_hours: null,
    alert_limit_per_day: null,
    timeframes: null,
    allowed_rules: null,
  })
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingFlush = useRef<OverrideState | null>(null)

  // Initial GET — populate state from current override row.
  useEffect(() => {
    let cancelled = false
    apiFetch<EffectiveResponse>(`/api/symbol-overrides/${encodeURIComponent(symbol)}`)
      .then(eff => {
        if (cancelled) return
        setState({
          score_threshold: eff.score_threshold.source === 'override' ? eff.score_threshold.value : null,
          cooldown_hours: eff.cooldown_hours.source === 'override' ? eff.cooldown_hours.value : null,
          alert_limit_per_day: eff.alert_limit_per_day.source === 'override' ? eff.alert_limit_per_day.value : null,
          timeframes: eff.timeframes.source === 'override' ? (eff.timeframes.value as Timeframe[]) : null,
          allowed_rules: eff.allowed_rules.source === 'override' ? eff.allowed_rules.value : null,
        })
      })
      .catch(() => { /* leave defaults */ })
    return () => { cancelled = true }
  }, [symbol])

  // Schedule a debounced PUT whenever state changes.
  const scheduleSave = useCallback((next: OverrideState) => {
    pendingFlush.current = next
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    debounceTimer.current = setTimeout(async () => {
      const payload = pendingFlush.current
      if (!payload) return
      try {
        await apiFetch<EffectiveResponse>(
          `/api/symbol-overrides/${encodeURIComponent(symbol)}`,
          { method: 'PUT', body: JSON.stringify(payload) },
          apiToken,
        )
        setSavedAt(Date.now())
        setError(null)
      } catch (e) {
        setError(e instanceof Error ? e.message : 'unknown')
      }
    }, DEBOUNCE_MS)
  }, [symbol, apiToken])

  // Flush on unmount with pending changes.
  useEffect(() => () => {
    if (debounceTimer.current) {
      clearTimeout(debounceTimer.current)
      const payload = pendingFlush.current
      if (payload) {
        // Best-effort synchronous send via sendBeacon if available, else fire-and-forget fetch.
        const url = `/api/symbol-overrides/${encodeURIComponent(symbol)}`
        const body = JSON.stringify(payload)
        if (navigator.sendBeacon) {
          navigator.sendBeacon(url, new Blob([body], { type: 'application/json' }))
        } else {
          fetch(url, { method: 'PUT', body, headers: { 'Content-Type': 'application/json' } })
        }
      }
    }
  }, [symbol])

  const updateField = <K extends keyof OverrideState>(key: K, value: OverrideState[K]) => {
    setState(prev => {
      const next = { ...prev, [key]: value }
      scheduleSave(next)
      return next
    })
  }

  const resetField = (key: keyof OverrideState) => {
    updateField(key, null as never)
  }

  const resetAll = async () => {
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    pendingFlush.current = null
    try {
      await apiFetch<EffectiveResponse>(
        `/api/symbol-overrides/${encodeURIComponent(symbol)}`,
        { method: 'DELETE' },
        apiToken,
      )
      setState({
        score_threshold: null, cooldown_hours: null,
        alert_limit_per_day: null, timeframes: null, allowed_rules: null,
      })
      setSavedAt(Date.now())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'unknown')
    }
  }

  // Render helpers --------------------------------------------------------------

  const sliderField = (
    label: string,
    field: 'score_threshold' | 'cooldown_hours' | 'alert_limit_per_day',
    min: number,
    max: number,
    step: number,
    profileDefault: number,
  ) => {
    const overrideVal = state[field]
    const effective = overrideVal ?? profileDefault
    return (
      <div style={{ marginBottom: 12 }}>
        <label htmlFor={`f-${field}`} style={{ display: 'block', fontSize: '0.78rem', color: 'var(--muted)' }}>
          {label}
        </label>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <input
            id={`f-${field}`}
            type="range"
            min={min} max={max} step={step}
            value={effective}
            onChange={e => updateField(field, Number(e.target.value))}
            style={{ flex: 1 }}
          />
          <span style={{ minWidth: 48, textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
            {effective}
          </span>
          {overrideVal !== null && (
            <button
              data-testid={`reset-${field}`}
              onClick={() => resetField(field)}
              style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--muted)' }}
              title={t('override.reset')}
            >↺</button>
          )}
        </div>
        <span style={{ fontSize: '0.7rem', color: 'var(--muted)' }}>
          {overrideVal !== null
            ? t('override.profile_default_value', { value: profileDefault })
            : t('override.profile_default')}
        </span>
      </div>
    )
  }

  const timeframeField = () => {
    const active = new Set(state.timeframes ?? [])
    return (
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontSize: '0.78rem', color: 'var(--muted)', marginBottom: 4 }}>
          {t('override.timeframes')}
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          {TIMEFRAMES.map(tf => {
            const on = active.has(tf)
            return (
              <button
                key={tf}
                onClick={() => {
                  const next = new Set(active)
                  if (on) next.delete(tf)
                  else next.add(tf)
                  const arr = Array.from(next) as Timeframe[]
                  updateField('timeframes', arr.length > 0 ? arr : null)
                }}
                style={{
                  padding: '4px 10px',
                  border: `1px solid ${on ? 'var(--mint)' : 'var(--muted)'}`,
                  background: on ? 'var(--mint)' : 'transparent',
                  color: on ? 'var(--bg)' : 'var(--muted)',
                  borderRadius: 4, cursor: 'pointer', fontSize: '0.78rem',
                }}
              >{tf}</button>
            )
          })}
          {state.timeframes !== null && (
            <button
              data-testid="reset-timeframes"
              onClick={() => resetField('timeframes')}
              style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--muted)' }}
              title={t('override.reset')}
            >↺</button>
          )}
        </div>
      </div>
    )
  }

  return (
    <div style={{ padding: 12, background: 'rgba(91,146,121,0.04)', border: '1px solid rgba(91,146,121,0.15)', borderRadius: 6 }}>
      {sliderField(t('override.score_threshold'), 'score_threshold', 0, 50, 0.5, profile.score_threshold)}
      {sliderField(t('override.cooldown_hours'), 'cooldown_hours', 0, 168, 1, profile.cooldown_hours)}
      {sliderField(t('override.alert_limit'), 'alert_limit_per_day', 0, 20, 1, profile.alert_limit_per_day)}
      {timeframeField()}

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 12, fontSize: '0.72rem' }}>
        <span style={{ color: error ? 'var(--danger)' : 'var(--muted)' }}>
          {error ? `${t('override.save_failed')}: ${error}`
            : savedAt ? t('override.saved_ago', { n: Math.max(0, Math.round((Date.now() - savedAt) / 1000)) })
            : ''}
        </span>
        <button
          data-testid="reset-all"
          onClick={resetAll}
          style={{ background: 'transparent', border: '1px solid var(--muted)', borderRadius: 4, padding: '4px 8px', color: 'var(--muted)', cursor: 'pointer', fontSize: '0.7rem' }}
        >
          {t('override.reset_all')}
        </button>
      </div>
    </div>
  )
}
```

(Note: `allowed_rules` editor is intentionally omitted from the v1 component — the rule registry list and grouping requires loading from the backend and adds visual complexity. It can be added as a follow-up; the spec section 11 lists this as a v1.1 candidate.)

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd web && bun test SymbolOverrideEditor`

Expected: 4/4 tests PASS.

- [ ] **Step 3: Run full frontend suite**

Run: `cd web && bun test && bun run build`

Expected: all vitest tests pass; vite build succeeds.

- [ ] **Step 4: Commit**

```bash
git add web/src/SymbolOverrideEditor.tsx web/src/SymbolOverrideEditor.test.tsx
git commit -m "feat(ui): SymbolOverrideEditor — sliders, TF chips, debounced auto-save

500ms debounce, sendBeacon flush on unmount, ↺ reset per field,
'Reset all to profile' triggers DELETE."
```

---

### Task D4: Wire editor into SymbolsTab

**Files:**
- Modify: `web/src/App.tsx` (find `function SymbolsTab()`)

- [ ] **Step 1: Import the editor at the top of App.tsx**

Find the top imports block in `web/src/App.tsx`. Add:

```tsx
import { SymbolOverrideEditor } from './SymbolOverrideEditor'
```

- [ ] **Step 2: Add expand state and chevron rendering**

In `function SymbolsTab()`, add a piece of state to track which symbol row is expanded:

```tsx
const [expandedSymbol, setExpandedSymbol] = useState<string | null>(null)
```

In the symbol row JSX (around line 479–506 — the `filteredSymbols.map((sym) => ...)` block), wrap the existing row content in a `<div>` and append a chevron button + a conditional editor block. Replace the row JSX (`<div key={sym.symbol} className="item">...</div>`) with:

```tsx
<div key={sym.symbol}>
  <div className="item">
    <div>
      <div className="item-name">
        <span className={`badge badge-${sym.type}`}>{sym.type}</span>
        {sym.symbol}
      </div>
      <div className="item-meta">{sym.exchange}</div>
    </div>
    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
      {profiles.length > 0 && (
        <select
          className="chart-select"
          style={{ fontSize: '0.8rem', padding: '2px 6px', minWidth: 120 }}
          value={symbolProfiles[sym.symbol] ?? ''}
          onChange={(e) => changeProfile(sym.symbol, e.target.value)}
          disabled={profileUpdating === sym.symbol}
          title={t('profile')}
        >
          {profiles.map(p => (
            <option key={p.name} value={p.name}>{t(`profile_${p.name}`, p.name)}</option>
          ))}
        </select>
      )}
      <Toggle checked={sym.enabled} onChange={(v) => toggle(sym, v)} />
      <button
        onClick={() => setExpandedSymbol(prev => prev === sym.symbol ? null : sym.symbol)}
        style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--muted)', fontSize: '1rem' }}
        title="Custom alert overrides"
        aria-expanded={expandedSymbol === sym.symbol}
      >
        {expandedSymbol === sym.symbol ? '▼' : '▶'}
      </button>
      <button className="remove-btn" onClick={() => remove(sym)} title={t('delete')}>✕</button>
    </div>
  </div>
  {expandedSymbol === sym.symbol && (() => {
    const profileObj = profiles.find(p => p.name === (symbolProfiles[sym.symbol] ?? ''))
    if (!profileObj) return null
    return (
      <div style={{ padding: '8px 16px 16px' }}>
        <SymbolOverrideEditor symbol={sym.symbol} profile={profileObj} />
      </div>
    )
  })()}
</div>
```

- [ ] **Step 3: Type-check and build**

Run: `cd web && bun run build`

Expected: vite build succeeds.

- [ ] **Step 4: Run frontend tests**

Run: `cd web && bun test`

Expected: PASS (no SymbolsTab tests should regress).

- [ ] **Step 5: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat(ui): SymbolsTab row expand — embeds SymbolOverrideEditor"
```

---

### Task D5: Wire editor into ChartTab via ⚙ modal

**Files:**
- Modify: `web/src/App.tsx` (`function ChartTab(...)`)

- [ ] **Step 1: Add modal state**

In `function ChartTab(...)`, add:

```tsx
const [overrideModalOpen, setOverrideModalOpen] = useState(false)
const [profilesForChart, setProfilesForChart] = useState<ProfileInfo[]>([])

useEffect(() => {
  apiFetch<ProfileInfo[]>('/profiles').then(setProfilesForChart).catch(() => {})
}, [])
```

(`ProfileInfo` is already declared elsewhere in `App.tsx`.)

- [ ] **Step 2: Add ⚙ button next to the symbol selector**

Find the symbol selector `<select>` block in `ChartTab` (search for `combobox` / the option mapping over the symbol list). Append a small button **after** the closing `</select>`:

```tsx
<button
  onClick={() => setOverrideModalOpen(true)}
  title="Alert override"
  style={{
    background: 'transparent', border: '1px solid rgba(91,146,121,0.3)',
    borderRadius: 4, padding: '2px 8px', cursor: 'pointer',
    color: 'var(--muted)', marginLeft: 4,
  }}
>⚙</button>
```

- [ ] **Step 3: Render the modal at the bottom of ChartTab return JSX**

Just before the closing `</>` (or root element) of `ChartTab`'s return, add:

```tsx
{overrideModalOpen && (() => {
  // Look up the profile assigned to the current symbol; fall back to first profile.
  const profileObj = profilesForChart[0] ?? null
  if (!profileObj) return null
  return (
    <div
      onClick={() => setOverrideModalOpen(false)}
      style={{
        position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.55)',
        display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
      }}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          background: 'var(--bg)', border: '1px solid var(--mint)', borderRadius: 8,
          padding: 20, minWidth: 400, maxWidth: 600, maxHeight: '80vh', overflow: 'auto',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <strong>{symbol}</strong>
          <button
            onClick={() => setOverrideModalOpen(false)}
            style={{ background: 'transparent', border: 'none', color: 'var(--muted)', cursor: 'pointer', fontSize: '1.2rem' }}
          >✕</button>
        </div>
        <SymbolOverrideEditor symbol={symbol} profile={profileObj} />
      </div>
    </div>
  )
})()}
```

(`symbol` is already a state variable in `ChartTab`.)

- [ ] **Step 4: Build + type-check**

Run: `cd web && bun run build`

Expected: vite build succeeds.

- [ ] **Step 5: Manual smoke test (optional but recommended)**

Run: `make dev` (or the project's standard dev startup) and verify in a browser:
1. Symbols tab → expand TSLA chevron → editor appears.
2. Drag score slider → after ~500ms a "Saved ✓" appears.
3. Chart tab → click ⚙ → modal opens.
4. Refresh page → values persist.

- [ ] **Step 6: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat(ui): Chart tab ⚙ modal exposes SymbolOverrideEditor"
```

---

### Task D6: Release housekeeping

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Bump VERSION**

Read current `VERSION` (expected `2.7.0.0`). Replace contents with:

```
2.8.0.0
```

- [ ] **Step 2: Prepend CHANGELOG entry**

In `CHANGELOG.md`, insert this block immediately above the existing top-most `## [` heading:

```markdown
## [2.8.0.0] - 2026-05-03

### Added
- Per-symbol alert overrides: tune `score_threshold`, `cooldown_hours`, `alert_limit_per_day`, and `timeframes` per symbol from the UI without touching YAML or restarting the server.
- New SQLite table `symbol_alert_overrides` with nullable fields (NULL = inherit from profile).
- New API endpoints: `GET/PUT/DELETE /api/symbol-overrides/{symbol}`.
- New React component `SymbolOverrideEditor` mounted in Symbols tab (row expand) and Chart tab (⚙ modal).
- New `Profile.Timeframes` field in `config/symbol_profiles.yaml` (optional; existing YAMLs unaffected).

### Changed
- Pipeline filter chain now consults `EffectiveAlertConfig(symbol)` for each tick — overrides take effect on the next signal evaluation, no restart required.

### Notes
- Empty arrays for `timeframes`/`allowed_rules` in PUT bodies are normalized to NULL on disk to avoid silently muting all alerts.
```

- [ ] **Step 3: Run full test sweep**

```bash
go test ./... -count=1 -race
cd web && bun test && bun run build
```

Expected: both green.

- [ ] **Step 4: Commit**

```bash
git add VERSION CHANGELOG.md
git commit -m "chore(release): bump VERSION to 2.8.0.0 + changelog for symbol overrides"
```

---

## Self-Review

### Spec coverage check

- §1 Goal — covered by tasks D2–D5 (UI) and B2 (timeframe filter).
- §2 Architecture diagram — implemented across A2/A3 (storage), A5 (merge), B2 (pipeline), C2 (HTTP), D3 (UI).
- §3 Data model — table created in A1, field added in A4, store in A3.
- §4 Backend — A5/B2/B3 cover merge, filter chain, hot-reload, defensive nil handling.
- §5 API — C1/C2 cover GET/PUT/DELETE + validation + auth.
- §6 Frontend — D2/D3 (component), D4 (Symbols tab), D5 (Chart modal), D1 (i18n).
- §7 Testing — every Phase has a TDD failing-test step before implementation.
- §8 Migration — A1 idempotent CREATE TABLE; A4 optional YAML field.
- §9 Error handling — covered in handler tests (C1) and `EffectiveAlertConfig` defensive paths (A5).
- §10 Observability — logged in B2 step 4 (Debug logs on filter drops).
- §11 Out of scope — `allowed_rules` editor explicitly deferred in D3 step 1 note.
- §12 Success criteria — bullets 1–4 are exercisable after D5; bullet 5 is satisfied by Task A1+D6 (no YAML edit required).

### Placeholder scan

No `TBD`/`TODO`/`fill in details`/`similar to Task N` strings. Every step that touches code shows the exact code or exact command. The one explicit deferral (`allowed_rules` editor) is called out as scope, not a placeholder.

### Type consistency

- `SymbolOverride` struct fields used identically in storage (A3), merge (A5), handler (C2).
- `EffectiveConfig` fields match across config (A5), pipeline call (B2), and handler response shape (C2's `effectiveResponse`).
- `OverrideGetter` interface defined in A5, satisfied by `*storage.SymbolOverrideStore` (Get method) and used in pipeline (B2) and handler (C2).
- Test helper struct `stubOverrideStore` (B1) and `fakeOverrideStore` (A4) both implement `OverrideGetter`.
- Frontend `OverrideState` keys (D3) match wire keys in handler request (C2).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-03-per-symbol-alert-overrides.md`. Two execution options:

**1. Subagent-Driven (recommended)** — Dispatch a fresh subagent per task with two-stage review (spec compliance, then code quality) between tasks. Fast iteration, isolated context per task, prevents drift on a 17-step plan.

**2. Inline Execution** — Execute tasks in this session using `superpowers:executing-plans` with checkpoints between phases.

**Which approach?**
