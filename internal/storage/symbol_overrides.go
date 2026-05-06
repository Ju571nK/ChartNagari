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
		score     sql.NullFloat64
		cooldown  sql.NullInt64
		limit     sql.NullInt64
		tfJSON    sql.NullString
		rulesJSON sql.NullString
		updated   int64
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
