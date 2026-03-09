package storage

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SaveSignal persists a generated signal to the database.
func (db *DB) SaveSignal(sig models.Signal) error {
	_, err := db.conn.Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sig.Symbol,
		sig.Timeframe,
		sig.Rule,
		sig.Direction,
		sig.Score,
		sig.Message,
		sig.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("신호 저장 실패 [%s %s]: %w", sig.Symbol, sig.Rule, err)
	}
	return nil
}

// GetLatestSignalTime returns the Unix-second timestamp of the most recent signal,
// or 0 if no signals have been saved yet.
func (db *DB) GetLatestSignalTime() (int64, error) {
	var ts int64
	err := db.conn.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM signals`).Scan(&ts)
	return ts, err
}

// GetSignals retrieves the N most recent signals for a given symbol.
func (db *DB) GetSignals(symbol string, limit int) ([]models.Signal, error) {
	rows, err := db.conn.Query(`
		SELECT symbol, timeframe, rule, direction, score, message, created_at
		FROM signals
		WHERE symbol = ?
		ORDER BY created_at DESC
		LIMIT ?`,
		symbol, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sigs []models.Signal
	for rows.Next() {
		var s models.Signal
		var createdAtUnix int64
		if err := rows.Scan(
			&s.Symbol, &s.Timeframe, &s.Rule, &s.Direction, &s.Score, &s.Message, &createdAtUnix,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}
