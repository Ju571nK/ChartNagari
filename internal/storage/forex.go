package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (db *DB) ForexProvider() (string, error) {
	var provider string
	err := db.conn.QueryRow(`SELECT provider FROM forex_state WHERE id=1`).Scan(&provider)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return "", nil
	}
	return provider, err
}

// EnsureForexProvider serializes provider transitions with the OHLCV purge.
// It also removes rows from a prior version lacking the provider marker.
func (db *DB) EnsureForexProvider(provider string, symbols []string) (bool, error) {
	sources := make(map[string]string, len(symbols))
	for _, symbol := range symbols {
		sources[symbol] = provider
	}
	return db.EnsureForexSources(provider, sources)
}

// EnsureForexSources atomically invalidates bars when provider, environment or
// symbol mapping changes. Existing paper positions remain open and suspended.
func (db *DB) EnsureForexSources(provider string, sources map[string]string) (bool, error) {
	if provider != "yahoo" && provider != "oanda" {
		return false, fmt.Errorf("invalid forex provider %q", provider)
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS forex_state (id INTEGER PRIMARY KEY CHECK(id=1), provider TEXT NOT NULL)`); err != nil {
		return false, err
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS forex_symbol_sources (symbol TEXT PRIMARY KEY, identity TEXT NOT NULL)`); err != nil {
		return false, err
	}
	var previous string
	err = tx.QueryRow(`SELECT provider FROM forex_state WHERE id=1`).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	changed := previous != "" && previous != provider
	providerChanged := changed
	if changed {
		if _, err = tx.Exec(`DELETE FROM ohlcv WHERE source IN ('yahoo_fx','oanda')`); err != nil {
			return false, err
		}
	}
	for symbol, identity := range sources {
		var prior string
		err = tx.QueryRow(`SELECT identity FROM forex_symbol_sources WHERE symbol=?`, symbol).Scan(&prior)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		identityChanged := providerChanged || (prior != "" && prior != identity)
		if identityChanged || prior == "" {
			changed = changed || identityChanged
			if _, err = tx.Exec(`DELETE FROM ohlcv WHERE symbol=?`, symbol); err != nil {
				return false, err
			}
		} else {
			source := "yahoo_fx"
			if provider == "oanda" {
				source = "oanda"
			}
			if _, err = tx.Exec(`DELETE FROM ohlcv WHERE symbol=? AND source<>?`, symbol, source); err != nil {
				return false, err
			}
		}
		if _, err = tx.Exec(`UPDATE paper_positions SET suspended=1, suspension_reason=CASE WHEN source_identity='' THEN 'unknown_origin' ELSE 'source_mismatch' END WHERE symbol=? AND status='OPEN' AND (source_identity='' OR source_identity<>?)`, symbol, identity); err != nil {
			return false, err
		}
		if _, err = tx.Exec(`INSERT INTO forex_symbol_sources(symbol,identity) VALUES(?,?) ON CONFLICT(symbol) DO UPDATE SET identity=excluded.identity`, symbol, identity); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(`INSERT INTO forex_state(id,provider) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET provider=excluded.provider`, provider); err != nil {
		return false, err
	}
	return changed, tx.Commit()
}
