// Package storage manages the SQLite database for OHLCV data persistence.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO_ENABLED=0 compatible)
)

// DB wraps the SQLite database connection.
type DB struct {
	conn *sql.DB
}

// New opens (or creates) the SQLite database at dbPath and applies the schema.
func New(dbPath string) (*DB, error) {
	// Create data directory if absent
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create DB directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("SQLite connection failed: %w", err)
	}

	// WAL mode: improved concurrent read/write performance
	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("schema migration failed: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying *sql.DB for direct use.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// migrate applies the initial database schema.
func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS ohlcv (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol    TEXT    NOT NULL,
		timeframe TEXT    NOT NULL,
		open_time INTEGER NOT NULL,  -- Unix milliseconds
		open      REAL    NOT NULL,
		high      REAL    NOT NULL,
		low       REAL    NOT NULL,
		close     REAL    NOT NULL,
		volume    REAL    NOT NULL,
		source    TEXT    NOT NULL DEFAULT 'binance', -- 'binance' | 'yahoo'
		UNIQUE(symbol, timeframe, open_time)
	);

	CREATE INDEX IF NOT EXISTS idx_ohlcv_lookup
		ON ohlcv(symbol, timeframe, open_time DESC);

	-- Alert cooldown tracking table (used in Phase 1-7)
	CREATE TABLE IF NOT EXISTS alert_history (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol     TEXT    NOT NULL,
		rule_name  TEXT    NOT NULL,
		sent_at    INTEGER NOT NULL  -- Unix seconds
	);

	CREATE INDEX IF NOT EXISTS idx_alert_history_lookup
		ON alert_history(symbol, rule_name, sent_at DESC);

	-- Signal persistence table (Phase 2-3 chart dashboard)
	CREATE TABLE IF NOT EXISTS signals (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol             TEXT    NOT NULL,
		timeframe          TEXT    NOT NULL,
		rule               TEXT    NOT NULL,
		direction          TEXT    NOT NULL,
		score              REAL    NOT NULL,
		message            TEXT    NOT NULL DEFAULT '',
		ai_interpretation  TEXT    NOT NULL DEFAULT '',
		created_at         INTEGER NOT NULL  -- Unix seconds
	);

	CREATE INDEX IF NOT EXISTS idx_signals_lookup
		ON signals(symbol, created_at DESC);

	-- Paper trading positions table
	CREATE TABLE IF NOT EXISTS paper_positions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol      TEXT    NOT NULL,
		timeframe   TEXT    NOT NULL,
		rule        TEXT    NOT NULL,
		direction   TEXT    NOT NULL,
		entry_price REAL    NOT NULL,
		tp          REAL    NOT NULL,
		sl          REAL    NOT NULL,
		entry_time  INTEGER NOT NULL,
		exit_price  REAL    NOT NULL DEFAULT 0,
		exit_time   INTEGER NOT NULL DEFAULT 0,
		status      TEXT    NOT NULL DEFAULT 'OPEN',
		pnl_pct     REAL    NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_paper_positions_lookup
		ON paper_positions(symbol, status, entry_time DESC);

	-- AI analysis history table
	CREATE TABLE IF NOT EXISTS analysis_history (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol      TEXT    NOT NULL,
		final       TEXT    NOT NULL,  -- 'BULL' | 'BEAR' | 'SIDEWAYS'
		confidence  TEXT    NOT NULL,  -- 'HIGH' | 'MEDIUM' | 'LOW'
		bull_pct    REAL    NOT NULL,
		bear_pct    REAL    NOT NULL,
		sideways_pct REAL   NOT NULL,
		result_json TEXT    NOT NULL,  -- full ScenarioResult JSON
		created_at  INTEGER NOT NULL   -- Unix seconds
	);

	CREATE INDEX IF NOT EXISTS idx_analysis_history_lookup
		ON analysis_history(symbol, created_at DESC);
	`
	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}

	// Migrate existing DB: add ai_interpretation column
	if _, err := db.conn.Exec(
		`ALTER TABLE signals ADD COLUMN ai_interpretation TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("signals table migration failed: %w", err)
		}
	}

	return nil
}
