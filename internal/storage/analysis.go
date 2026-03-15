package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/internal/analyst"
)

// AnalysisRecord is a row from the analysis_history table.
type AnalysisRecord struct {
	ID          int64                  `json:"id"`
	Symbol      string                 `json:"symbol"`
	Final       string                 `json:"final"`
	Confidence  string                 `json:"confidence"`
	BullPct     float64                `json:"bull_pct"`
	BearPct     float64                `json:"bear_pct"`
	SidewaysPct float64                `json:"sideways_pct"`
	Result      *analyst.ScenarioResult `json:"result,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// SaveAnalysis persists a ScenarioResult to the analysis_history table.
func (db *DB) SaveAnalysis(result analyst.ScenarioResult) (int64, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return 0, fmt.Errorf("분석 결과 직렬화 실패: %w", err)
	}
	res, err := db.conn.Exec(
		`INSERT INTO analysis_history (symbol, final, confidence, bull_pct, bear_pct, sideways_pct, result_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		result.Symbol, result.Final, result.Confidence,
		result.BullPct, result.BearPct, result.SidewaysPct,
		string(data), time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("분석 결과 저장 실패: %w", err)
	}
	return res.LastInsertId()
}

// GetAnalysisHistory returns the most recent analyses, optionally filtered by symbol.
// Pass symbol="" to get all symbols.
func (db *DB) GetAnalysisHistory(symbol string, limit int) ([]AnalysisRecord, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if symbol != "" {
		rows, err = db.conn.Query(
			`SELECT id, symbol, final, confidence, bull_pct, bear_pct, sideways_pct, created_at
			 FROM analysis_history WHERE symbol = ?
			 ORDER BY created_at DESC LIMIT ?`,
			symbol, limit,
		)
	} else {
		rows, err = db.conn.Query(
			`SELECT id, symbol, final, confidence, bull_pct, bear_pct, sideways_pct, created_at
			 FROM analysis_history ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AnalysisRecord
	for rows.Next() {
		var r AnalysisRecord
		var ts int64
		if err := rows.Scan(&r.ID, &r.Symbol, &r.Final, &r.Confidence, &r.BullPct, &r.BearPct, &r.SidewaysPct, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(ts, 0).UTC()
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetAnalysisByID returns a single analysis record including the full result JSON.
func (db *DB) GetAnalysisByID(id int64) (*AnalysisRecord, error) {
	var r AnalysisRecord
	var ts int64
	var resultJSON string

	err := db.conn.QueryRow(
		`SELECT id, symbol, final, confidence, bull_pct, bear_pct, sideways_pct, result_json, created_at
		 FROM analysis_history WHERE id = ?`, id,
	).Scan(&r.ID, &r.Symbol, &r.Final, &r.Confidence, &r.BullPct, &r.BearPct, &r.SidewaysPct, &resultJSON, &ts)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = time.Unix(ts, 0).UTC()

	var result analyst.ScenarioResult
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return nil, fmt.Errorf("JSON 파싱 실패: %w", err)
	}
	r.Result = &result
	return &r, nil
}
