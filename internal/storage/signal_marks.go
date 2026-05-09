package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SignalMark is one row of the signal_marks table.
type SignalMark struct {
	SignalID     int64
	Status       string   // PENDING | TOOK | SKIPPED | WIN | LOSS | BE
	TookAt       *int64   // unix sec
	OutcomeAt    *int64
	OutcomePnLR  *float64
	Notes        *string
	TgMessageID  *int64
	UpdatedAt    int64
}

// SignalMarkStore performs CRUD on signal_marks with FSM enforcement.
type SignalMarkStore struct {
	db *DB
}

// NewSignalMarkStore creates a store backed by the given DB.
func NewSignalMarkStore(db *DB) *SignalMarkStore {
	return &SignalMarkStore{db: db}
}

// Get returns the mark for a signal, or (nil, nil) if no row exists.
func (s *SignalMarkStore) Get(signalID int64) (*SignalMark, error) {
	row := s.db.conn.QueryRow(`
		SELECT signal_id, status, took_at, outcome_at, outcome_pnl_r, notes, tg_message_id, updated_at
		  FROM signal_marks WHERE signal_id = ?`, signalID)
	var (
		out   SignalMark
		took  sql.NullInt64
		outc  sql.NullInt64
		pnl   sql.NullFloat64
		notes sql.NullString
		msg   sql.NullInt64
	)
	err := row.Scan(&out.SignalID, &out.Status, &took, &outc, &pnl, &notes, &msg, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query signal_marks: %w", err)
	}
	if took.Valid {
		v := took.Int64
		out.TookAt = &v
	}
	if outc.Valid {
		v := outc.Int64
		out.OutcomeAt = &v
	}
	if pnl.Valid {
		v := pnl.Float64
		out.OutcomePnLR = &v
	}
	if notes.Valid {
		v := notes.String
		out.Notes = &v
	}
	if msg.Valid {
		v := msg.Int64
		out.TgMessageID = &v
	}
	return &out, nil
}

// Mark applies an action to a signal. Returns the new status, or "" if the row was deleted.
// Validates the FSM transition; invalid transitions return an error containing "invalid".
func (s *SignalMarkStore) Mark(signalID int64, action string) (string, error) {
	cur, err := s.Get(signalID)
	if err != nil {
		return "", err
	}
	from := ""
	if cur != nil {
		from = cur.Status
	}
	newStatus, deleteRow, err := nextFSMState(from, action)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()

	if deleteRow {
		_, err := s.db.conn.Exec(`DELETE FROM signal_marks WHERE signal_id = ?`, signalID)
		if err != nil {
			return "", fmt.Errorf("delete signal_marks: %w", err)
		}
		return "", nil
	}

	// Compute took_at / outcome_at based on transition.
	tookAt := sql.NullInt64{}
	outcomeAt := sql.NullInt64{}
	if cur != nil && cur.TookAt != nil {
		tookAt = sql.NullInt64{Int64: *cur.TookAt, Valid: true}
	}
	switch newStatus {
	case "TOOK":
		if !tookAt.Valid {
			tookAt = sql.NullInt64{Int64: now, Valid: true}
		}
	case "WIN", "LOSS", "BE":
		if !tookAt.Valid {
			tookAt = sql.NullInt64{Int64: now, Valid: true}
		}
		outcomeAt = sql.NullInt64{Int64: now, Valid: true}
	}

	_, err = s.db.conn.Exec(`
		INSERT INTO signal_marks (signal_id, status, took_at, outcome_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(signal_id) DO UPDATE SET
		  status     = excluded.status,
		  took_at    = excluded.took_at,
		  outcome_at = excluded.outcome_at,
		  updated_at = excluded.updated_at`,
		signalID, newStatus, tookAt, outcomeAt, now)
	if err != nil {
		return "", fmt.Errorf("upsert signal_marks: %w", err)
	}
	return newStatus, nil
}

// SetMessageID records the Telegram message_id for a signal so the bot can
// editMessageReplyMarkup later. Caller usually invokes this immediately after
// the alert is sent. Creates a stub PENDING row if no mark exists yet.
func (s *SignalMarkStore) SetMessageID(signalID int64, msgID int64) error {
	now := time.Now().Unix()
	_, err := s.db.conn.Exec(`
		INSERT INTO signal_marks (signal_id, status, tg_message_id, updated_at)
		VALUES (?, 'PENDING', ?, ?)
		ON CONFLICT(signal_id) DO UPDATE SET
		  tg_message_id = excluded.tg_message_id,
		  updated_at    = excluded.updated_at`,
		signalID, msgID, now)
	if err != nil {
		return fmt.Errorf("set message_id: %w", err)
	}
	return nil
}

// SignalExists returns true if a signal with the given id is in the signals table.
// Used by the API layer to validate FK before attempting Mark.
func (s *SignalMarkStore) SignalExists(signalID int64) (bool, error) {
	var n int
	err := s.db.conn.QueryRow(`SELECT 1 FROM signals WHERE id = ?`, signalID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("signal exists: %w", err)
	}
	return true, nil
}

// directSetStatus is a test-only helper for seeding from-states in FSM tests.
func (s *SignalMarkStore) directSetStatus(signalID int64, status string) error {
	now := time.Now().Unix()
	_, err := s.db.conn.Exec(`
		INSERT INTO signal_marks (signal_id, status, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(signal_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`,
		signalID, status, now)
	return err
}

// SignalMarkRow joins a mark with its underlying signal for list queries.
// Used by My Trades tab Pending / History sections.
type SignalMarkRow struct {
	Signal models.Signal `json:"signal"` // ID, Symbol, Timeframe, Rule, Direction, Score, Message, CreatedAt populated
	Mark   *SignalMark   `json:"mark"`   // nil for pending rows (no signal_marks row yet)
}

// ListPending returns signals with NO mark row, created at or after `since`.
// Pass time.Time{} (zero) to disable the date filter.
// limit <= 0 or > 200 falls back to 50.
func (s *SignalMarkStore) ListPending(since time.Time, limit int) ([]SignalMarkRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sinceUnix := int64(0)
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}
	rows, err := s.db.conn.Query(`
		SELECT s.id, s.symbol, s.timeframe, s.rule, s.direction, s.score, s.message, s.created_at
		FROM signals s
		LEFT JOIN signal_marks m ON m.signal_id = s.id
		WHERE m.signal_id IS NULL AND s.created_at >= ?
		ORDER BY s.created_at DESC
		LIMIT ?`, sinceUnix, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	defer rows.Close()
	out := []SignalMarkRow{}
	for rows.Next() {
		var sig models.Signal
		var createdUnix int64
		if err := rows.Scan(&sig.ID, &sig.Symbol, &sig.Timeframe, &sig.Rule, &sig.Direction, &sig.Score, &sig.Message, &createdUnix); err != nil {
			return nil, err
		}
		sig.CreatedAt = time.Unix(createdUnix, 0).UTC()
		out = append(out, SignalMarkRow{Signal: sig, Mark: nil})
	}
	return out, rows.Err()
}

// ListMarked returns marked signals (any status) with updated_at >= since.
// limit <= 0 or > 200 falls back to 50.
func (s *SignalMarkStore) ListMarked(since time.Time, limit int) ([]SignalMarkRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sinceUnix := int64(0)
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}
	rows, err := s.db.conn.Query(`
		SELECT s.id, s.symbol, s.timeframe, s.rule, s.direction, s.score, s.message, s.created_at,
		       m.status, m.took_at, m.outcome_at, m.tg_message_id, m.updated_at
		FROM signal_marks m
		JOIN signals s ON s.id = m.signal_id
		WHERE m.updated_at >= ?
		ORDER BY m.updated_at DESC
		LIMIT ?`, sinceUnix, limit)
	if err != nil {
		return nil, fmt.Errorf("list marked: %w", err)
	}
	defer rows.Close()
	out := []SignalMarkRow{}
	for rows.Next() {
		var sig models.Signal
		var createdUnix int64
		var mark SignalMark
		var took, outc, msg sql.NullInt64
		if err := rows.Scan(&sig.ID, &sig.Symbol, &sig.Timeframe, &sig.Rule, &sig.Direction, &sig.Score, &sig.Message, &createdUnix, &mark.Status, &took, &outc, &msg, &mark.UpdatedAt); err != nil {
			return nil, err
		}
		sig.CreatedAt = time.Unix(createdUnix, 0).UTC()
		mark.SignalID = sig.ID
		if took.Valid {
			v := took.Int64
			mark.TookAt = &v
		}
		if outc.Valid {
			v := outc.Int64
			mark.OutcomeAt = &v
		}
		if msg.Valid {
			v := msg.Int64
			mark.TgMessageID = &v
		}
		out = append(out, SignalMarkRow{Signal: sig, Mark: &mark})
	}
	return out, rows.Err()
}

// nextFSMState returns (newStatus, deleteRow, error).
func nextFSMState(from, action string) (string, bool, error) {
	switch from {
	case "", "PENDING":
		switch action {
		case "took":
			return "TOOK", false, nil
		case "skip":
			return "SKIPPED", false, nil
		default:
			label := "PENDING"
			if from == "" {
				label = "(no row)"
			}
			return "", false, fmt.Errorf("invalid transition: %s → %q", label, action)
		}
	case "TOOK":
		switch action {
		case "win":
			return "WIN", false, nil
		case "loss":
			return "LOSS", false, nil
		case "be":
			return "BE", false, nil
		case "undo":
			return "", true, nil
		default:
			return "", false, fmt.Errorf("invalid transition: TOOK → %q", action)
		}
	case "SKIPPED":
		if action == "undo" {
			return "", true, nil
		}
		return "", false, fmt.Errorf("invalid transition: SKIPPED → %q", action)
	case "WIN", "LOSS", "BE":
		switch action {
		case "win":
			return "WIN", false, nil
		case "loss":
			return "LOSS", false, nil
		case "be":
			return "BE", false, nil
		case "undo":
			return "TOOK", false, nil
		default:
			return "", false, fmt.Errorf("invalid transition: %s → %q", from, action)
		}
	}
	return "", false, fmt.Errorf("invalid transition: unknown from-state %q", from)
}
