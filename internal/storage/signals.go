package storage

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SaveSignal persists a generated signal to the database.
func (db *DB) SaveSignal(sig models.Signal) error {
	_, err := db.conn.Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sig.Symbol,
		sig.Timeframe,
		sig.Rule,
		sig.Direction,
		sig.Score,
		sig.Message,
		sig.AIInterpretation,
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

// GetSignalsByDate returns all signals for a symbol on the given calendar date (UTC).
func (db *DB) GetSignalsByDate(symbol string, date time.Time) ([]models.Signal, error) {
	start := time.Date(date.UTC().Year(), date.UTC().Month(), date.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	rows, err := db.conn.Query(`
		SELECT symbol, timeframe, rule, direction, score, message, ai_interpretation, created_at
		FROM signals
		WHERE symbol = ? AND created_at >= ? AND created_at < ?
		ORDER BY created_at DESC`,
		symbol, start.Unix(), end.Unix(),
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
			&s.Symbol, &s.Timeframe, &s.Rule, &s.Direction, &s.Score, &s.Message, &s.AIInterpretation, &createdAtUnix,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}

// GetSignalsFiltered returns signals with optional filters.
// symbol="ALL" → all symbols. direction="ALL" → all directions.
func (db *DB) GetSignalsFiltered(symbol, direction string, limit int) ([]models.Signal, error) {
	rows, err := db.conn.Query(`
		SELECT id, symbol, timeframe, rule, direction, score, message, ai_interpretation, created_at
		FROM signals
		WHERE (? = 'ALL' OR symbol = ?)
		  AND (? = 'ALL' OR direction = ?)
		ORDER BY created_at DESC
		LIMIT ?`,
		symbol, symbol, direction, direction, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sigs []models.Signal
	for rows.Next() {
		var s models.Signal
		var id int64
		var createdAtUnix int64
		if err := rows.Scan(
			&id, &s.Symbol, &s.Timeframe, &s.Rule, &s.Direction, &s.Score, &s.Message, &s.AIInterpretation, &createdAtUnix,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}

// GetSignals retrieves the N most recent signals for a given symbol.
func (db *DB) GetSignals(symbol string, limit int) ([]models.Signal, error) {
	rows, err := db.conn.Query(`
		SELECT symbol, timeframe, rule, direction, score, message, ai_interpretation, created_at
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
			&s.Symbol, &s.Timeframe, &s.Rule, &s.Direction, &s.Score, &s.Message, &s.AIInterpretation, &createdAtUnix,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}
