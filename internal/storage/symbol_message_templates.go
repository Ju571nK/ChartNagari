package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SymbolMessageTemplates holds optional Telegram wording for one symbol.
// An empty direction falls back to the unchanged standard alert.
type SymbolMessageTemplates struct {
	Symbol string `json:"symbol"`
	Long   string `json:"long"`
	Short  string `json:"short"`
}

type SymbolMessageTemplateStore struct{ db *DB }

func NewSymbolMessageTemplateStore(db *DB) *SymbolMessageTemplateStore {
	return &SymbolMessageTemplateStore{db: db}
}

func (s *SymbolMessageTemplateStore) Get(symbol string) (SymbolMessageTemplates, error) {
	out := SymbolMessageTemplates{Symbol: symbol}
	if symbol == "" {
		return out, errors.New("symbol must not be empty")
	}
	err := s.db.conn.QueryRow(`SELECT long_template, short_template FROM symbol_message_templates WHERE symbol = ?`, symbol).Scan(&out.Long, &out.Short)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("get symbol message templates: %w", err)
	}
	return out, nil
}

func (s *SymbolMessageTemplateStore) Put(value SymbolMessageTemplates) error {
	if value.Symbol == "" {
		return errors.New("symbol must not be empty")
	}
	if value.Long == "" && value.Short == "" {
		_, err := s.db.conn.Exec(`DELETE FROM symbol_message_templates WHERE symbol = ?`, value.Symbol)
		return err
	}
	_, err := s.db.conn.Exec(`INSERT INTO symbol_message_templates (symbol, long_template, short_template, updated_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(symbol) DO UPDATE SET long_template = excluded.long_template,
		short_template = excluded.short_template, updated_at = excluded.updated_at`,
		value.Symbol, value.Long, value.Short, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("put symbol message templates: %w", err)
	}
	return nil
}
