package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SaveOHLCV inserts or replaces an OHLCV bar.
// Uses INSERT OR REPLACE to handle duplicate (symbol, timeframe, open_time).
func (db *DB) SaveOHLCV(bar models.OHLCV, source string) error {
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO ohlcv
			(symbol, timeframe, open_time, open, high, low, close, volume, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		bar.Symbol,
		bar.Timeframe,
		bar.OpenTime.UnixMilli(),
		bar.Open,
		bar.High,
		bar.Low,
		bar.Close,
		bar.Volume,
		source,
	)
	if err != nil {
		return fmt.Errorf("OHLCV 저장 실패 [%s %s]: %w", bar.Symbol, bar.Timeframe, err)
	}
	return nil
}

// SaveOHLCVBatch inserts multiple bars in a single transaction.
func (db *DB) SaveOHLCVBatch(bars []models.OHLCV, source string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO ohlcv
			(symbol, timeframe, open_time, open, high, low, close, volume, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, bar := range bars {
		if _, err = stmt.Exec(
			bar.Symbol, bar.Timeframe, bar.OpenTime.UnixMilli(),
			bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, source,
		); err != nil {
			return fmt.Errorf("배치 저장 실패 [%s %s]: %w", bar.Symbol, bar.Timeframe, err)
		}
	}

	return tx.Commit()
}

// GetOHLCV retrieves the N most recent closed bars for a symbol+timeframe.
func (db *DB) GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	rows, err := db.conn.Query(`
		SELECT symbol, timeframe, open_time, open, high, low, close, volume
		FROM ohlcv
		WHERE symbol = ? AND timeframe = ?
		ORDER BY open_time DESC
		LIMIT ?`,
		symbol, timeframe, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanOHLCVRows(rows)
}

// GetOHLCVSince retrieves bars after (and including) the given time.
func (db *DB) GetOHLCVSince(symbol, timeframe string, since time.Time) ([]models.OHLCV, error) {
	rows, err := db.conn.Query(`
		SELECT symbol, timeframe, open_time, open, high, low, close, volume
		FROM ohlcv
		WHERE symbol = ? AND timeframe = ? AND open_time >= ?
		ORDER BY open_time ASC`,
		symbol, timeframe, since.UnixMilli(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanOHLCVRows(rows)
}

func scanOHLCVRows(rows *sql.Rows) ([]models.OHLCV, error) {
	var bars []models.OHLCV
	for rows.Next() {
		var b models.OHLCV
		var openTimeMs int64
		if err := rows.Scan(
			&b.Symbol, &b.Timeframe, &openTimeMs,
			&b.Open, &b.High, &b.Low, &b.Close, &b.Volume,
		); err != nil {
			return nil, err
		}
		b.OpenTime = time.UnixMilli(openTimeMs).UTC()
		bars = append(bars, b)
	}
	return bars, rows.Err()
}
