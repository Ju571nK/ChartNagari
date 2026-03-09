// Package storage manages the SQLite database for OHLCV data persistence.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO_ENABLED=0 호환)
)

// DB wraps the SQLite database connection.
type DB struct {
	conn *sql.DB
}

// New opens (or creates) the SQLite database at dbPath and applies the schema.
func New(dbPath string) (*DB, error) {
	// 데이터 디렉토리가 없으면 생성
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("DB 디렉토리 생성 실패: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("SQLite 연결 실패: %w", err)
	}

	// WAL 모드: 동시 읽기/쓰기 성능 향상
	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("WAL 모드 설정 실패: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("스키마 마이그레이션 실패: %w", err)
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
		open_time INTEGER NOT NULL,  -- Unix 밀리초
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

	-- 알림 쿨다운 추적 테이블 (Phase 1-7에서 사용)
	CREATE TABLE IF NOT EXISTS alert_history (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol     TEXT    NOT NULL,
		rule_name  TEXT    NOT NULL,
		sent_at    INTEGER NOT NULL  -- Unix 초
	);

	CREATE INDEX IF NOT EXISTS idx_alert_history_lookup
		ON alert_history(symbol, rule_name, sent_at DESC);

	-- 신호 영속성 테이블 (Phase 2-3 차트 대시보드용)
	CREATE TABLE IF NOT EXISTS signals (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol     TEXT    NOT NULL,
		timeframe  TEXT    NOT NULL,
		rule       TEXT    NOT NULL,
		direction  TEXT    NOT NULL,
		score      REAL    NOT NULL,
		message    TEXT    NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL  -- Unix 초
	);

	CREATE INDEX IF NOT EXISTS idx_signals_lookup
		ON signals(symbol, created_at DESC);

	-- 페이퍼 트레이딩 포지션 테이블
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
	`
	_, err := db.conn.Exec(schema)
	return err
}
