// Package storage manages the SQLite database for OHLCV data persistence.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	-- Price target alerts (user-defined price level notifications)
	CREATE TABLE IF NOT EXISTS price_alerts (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol       TEXT    NOT NULL,
		target       REAL    NOT NULL,
		condition    TEXT    NOT NULL DEFAULT 'above',  -- 'above' | 'below'
		note         TEXT    NOT NULL DEFAULT '',
		triggered    INTEGER NOT NULL DEFAULT 0,        -- 0=active, 1=triggered
		created_at   INTEGER NOT NULL,
		triggered_at INTEGER NOT NULL DEFAULT 0         -- Unix seconds; 0=not yet
	);

	CREATE INDEX IF NOT EXISTS idx_price_alerts_active
		ON price_alerts(symbol, triggered);
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

// PriceAlert represents a user-defined price target alert.
type PriceAlert struct {
	ID          int64
	Symbol      string
	Target      float64
	Condition   string // "above" | "below"
	Note        string
	Triggered   bool
	CreatedAt   time.Time
	TriggeredAt *time.Time
}

// AddPriceAlert creates a new active price alert. Returns the new row ID.
func (db *DB) AddPriceAlert(symbol, condition string, target float64, note string) (int64, error) {
	res, err := db.conn.Exec(
		`INSERT INTO price_alerts (symbol, target, condition, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		symbol, target, condition, note, time.Now().Unix(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListPriceAlerts returns all alerts (active and triggered), newest first.
func (db *DB) ListPriceAlerts() ([]PriceAlert, error) {
	rows, err := db.conn.Query(
		`SELECT id, symbol, target, condition, note, triggered, created_at, triggered_at
		 FROM price_alerts ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPriceAlerts(rows)
}

// GetActivePriceAlerts returns only alerts that have not yet been triggered.
func (db *DB) GetActivePriceAlerts() ([]PriceAlert, error) {
	rows, err := db.conn.Query(
		`SELECT id, symbol, target, condition, note, triggered, created_at, triggered_at
		 FROM price_alerts WHERE triggered = 0`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPriceAlerts(rows)
}

// MarkAlertTriggered marks an alert as triggered at the given time.
func (db *DB) MarkAlertTriggered(id int64, at time.Time) error {
	_, err := db.conn.Exec(
		`UPDATE price_alerts SET triggered = 1, triggered_at = ? WHERE id = ?`,
		at.Unix(), id,
	)
	return err
}

// DeletePriceAlert removes an alert by ID.
func (db *DB) DeletePriceAlert(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM price_alerts WHERE id = ?`, id)
	return err
}

func scanPriceAlerts(rows *sql.Rows) ([]PriceAlert, error) {
	var out []PriceAlert
	for rows.Next() {
		var a PriceAlert
		var createdAt int64
		var triggeredAt int64
		var triggered int
		if err := rows.Scan(&a.ID, &a.Symbol, &a.Target, &a.Condition, &a.Note,
			&triggered, &createdAt, &triggeredAt); err != nil {
			return nil, err
		}
		a.Triggered = triggered == 1
		a.CreatedAt = time.Unix(createdAt, 0)
		if triggeredAt > 0 {
			t := time.Unix(triggeredAt, 0)
			a.TriggeredAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
