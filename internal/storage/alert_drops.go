package storage

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/alertdrop"
)

const (
	dropTimeLayout    = "2006-01-02T15:04:05.000Z"
	dropDetailMax     = 200
	dropBatchMax      = 64
	dropBatchWait     = 50 * time.Millisecond
	defaultDropBuf    = 512
	defaultDropCap    = 200
	defaultDropTTL    = 7 * 24 * time.Hour
	defaultPruneEvery = 10 * time.Minute
)

// AlertDropOptions tunes the writer. Zero values pick defaults.
type AlertDropOptions struct {
	Buffer       int
	PerSymbolCap int
	TTL          time.Duration
	PruneEvery   time.Duration
	Log          zerolog.Logger
}

func (o AlertDropOptions) withDefaults() AlertDropOptions {
	if o.Buffer <= 0 {
		o.Buffer = defaultDropBuf
	}
	if o.PerSymbolCap <= 0 {
		o.PerSymbolCap = defaultDropCap
	}
	if o.TTL <= 0 {
		o.TTL = defaultDropTTL
	}
	if o.PruneEvery <= 0 {
		o.PruneEvery = defaultPruneEvery
	}
	return o
}

// AlertDropStore implements alertdrop.Store.
// One goroutine owns every write. Callers only enqueue, so the hot path shares no mutable row state.
type AlertDropStore struct {
	db        *DB
	opts      AlertDropOptions
	ch        chan alertdrop.Event
	overflow  atomic.Int64
	closed    atomic.Bool
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	log       zerolog.Logger
}

var _ alertdrop.Store = (*AlertDropStore)(nil)

// NewAlertDropStore starts the writer goroutine.
// It returns nil when db is nil so the caller can keep a Nop store instead of a typed nil.
func NewAlertDropStore(db *DB, opts AlertDropOptions) *AlertDropStore {
	if db == nil {
		return nil
	}
	opts = opts.withDefaults()
	s := &AlertDropStore{
		db:   db,
		opts: opts,
		ch:   make(chan alertdrop.Event, opts.Buffer),
		stop: make(chan struct{}),
		done: make(chan struct{}),
		log:  opts.Log,
	}
	go s.run()
	return s
}

// Record validates the event, then does a non-blocking send.
// A full buffer increments overflow and returns. A nil receiver is a no-op.
func (s *AlertDropStore) Record(ev alertdrop.Event) {
	if s == nil || s.closed.Load() {
		return
	}
	if !ev.Reason.Valid() {
		s.log.Warn().Msg("alert drop ignored: invalid reason")
		return
	}
	ev.Symbol = strings.ToUpper(strings.TrimSpace(ev.Symbol))
	if ev.Symbol == "" {
		s.log.Warn().Str("reason", ev.Reason.Code()).Msg("alert drop ignored: empty symbol")
		return
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	} else {
		ev.At = ev.At.UTC()
	}
	ev.Detail = clipDetail(ev.Detail)
	select {
	case s.ch <- ev:
	default:
		s.overflow.Add(1)
	}
}

// Recent returns coalesced rows ordered by last_seen descending.
// Rows whose reason code is not in the registry are skipped.
func (s *AlertDropStore) Recent(ctx context.Context, q alertdrop.Query) ([]alertdrop.Summary, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	q = q.Normalize()
	rows, err := s.db.conn.QueryContext(ctx, `
		SELECT symbol, rule, timeframe, direction, reason, sender,
		       score, threshold, signal_id, detail, count, first_seen, last_seen
		  FROM alert_drops
		 WHERE (? = '' OR symbol = ?)
		 ORDER BY last_seen DESC, rowid DESC
		 LIMIT ?`, q.Symbol, q.Symbol, q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]alertdrop.Summary, 0)
	for rows.Next() {
		var (
			sum   alertdrop.Summary
			code  string
			sigID sql.NullInt64
			first string
			last  string
		)
		if err := rows.Scan(
			&sum.Symbol, &sum.Rule, &sum.Timeframe, &sum.Direction, &code, &sum.Sender,
			&sum.Score, &sum.Threshold, &sigID, &sum.Detail, &sum.Count, &first, &last,
		); err != nil {
			return nil, err
		}
		if err := sum.Reason.UnmarshalText([]byte(code)); err != nil {
			s.log.Warn().Str("reason", code).Msg("skip alert drop with unknown reason")
			continue
		}
		firstAt, err := parseDropTime(first)
		if err != nil {
			s.log.Warn().Str("first_seen", first).Msg("skip alert drop with bad time")
			continue
		}
		lastAt, err := parseDropTime(last)
		if err != nil {
			s.log.Warn().Str("last_seen", last).Msg("skip alert drop with bad time")
			continue
		}
		if sigID.Valid {
			sum.SignalID = sigID.Int64
		}
		sum.FirstSeen = firstAt
		sum.LastSeen = lastAt
		sum.At = lastAt
		out = append(out, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Close stops new events, drains the buffer, and waits for the writer.
// It is safe to call more than once.
func (s *AlertDropStore) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.stop)
	})
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *AlertDropStore) run() {
	defer close(s.done)
	ticker := time.NewTicker(s.opts.PruneEvery)
	defer ticker.Stop()

	var (
		batch  []alertdrop.Event
		timer  *time.Timer
		timerC <-chan time.Time
	)
	write := func() {
		if len(batch) == 0 {
			return
		}
		s.writeBatch(batch)
		batch = nil
	}
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	enqueue := func(ev alertdrop.Event) {
		batch = append(batch, ev)
		if len(batch) >= dropBatchMax {
			stopTimer()
			write()
			return
		}
		if timer == nil {
			timer = time.NewTimer(dropBatchWait)
			timerC = timer.C
		}
	}
	drain := func() {
		stopTimer()
		for {
			select {
			case ev := <-s.ch:
				batch = append(batch, ev)
				if len(batch) >= dropBatchMax {
					write()
				}
			default:
				write()
				return
			}
		}
	}

	for {
		select {
		case ev := <-s.ch:
			enqueue(ev)
		case <-timerC:
			timer = nil
			timerC = nil
			write()
		case <-ticker.C:
			if err := s.prune(context.Background()); err != nil {
				s.log.Warn().Err(err).Msg("alert drop prune failed")
			}
			if n := s.overflow.Swap(0); n > 0 {
				s.log.Warn().Int64("overflow", n).Msg("alert drop records discarded")
			}
		case <-s.stop:
			drain()
			return
		}
	}
}

func (s *AlertDropStore) writeBatch(batch []alertdrop.Event) {
	tx, err := s.db.conn.Begin()
	if err != nil {
		s.log.Error().Err(err).Int("n", len(batch)).Msg("alert drop batch begin failed")
		return
	}
	// busy_timeout is per connection. Set it on the connection this transaction owns.
	if _, err := tx.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		_ = tx.Rollback()
		s.log.Error().Err(err).Msg("alert drop busy_timeout failed")
		return
	}
	for _, ev := range batch {
		if err := s.upsert(tx, ev); err != nil {
			_ = tx.Rollback()
			s.log.Error().Err(err).Str("symbol", ev.Symbol).Str("reason", ev.Reason.Code()).Msg("alert drop upsert failed")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.log.Error().Err(err).Int("n", len(batch)).Msg("alert drop batch commit failed")
	}
}

func (s *AlertDropStore) upsert(tx *sql.Tx, ev alertdrop.Event) error {
	var sigID any
	if ev.SignalID != 0 {
		sigID = ev.SignalID
	}
	at := formatDropTime(ev.At)
	_, err := tx.Exec(`
		INSERT INTO alert_drops (
			symbol, rule, timeframe, direction, reason, sender,
			score, threshold, signal_id, detail, count, first_seen, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(symbol, rule, timeframe, direction, reason, sender) DO UPDATE SET
			count = count + 1,
			last_seen = MAX(last_seen, excluded.last_seen),
			score = excluded.score,
			threshold = excluded.threshold,
			signal_id = COALESCE(excluded.signal_id, alert_drops.signal_id),
			detail = excluded.detail`,
		ev.Symbol, ev.Rule, ev.Timeframe, ev.Direction, ev.Reason.Code(), ev.Sender,
		ev.Score, ev.Threshold, sigID, ev.Detail, at, at,
	)
	return err
}

func (s *AlertDropStore) prune(ctx context.Context) error {
	cutoff := formatDropTime(time.Now().UTC().Add(-s.opts.TTL))
	tx, err := s.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_drops WHERE last_seen < ?`, cutoff); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM alert_drops WHERE rowid IN (
			SELECT rowid FROM (
				SELECT rowid,
				       ROW_NUMBER() OVER (PARTITION BY symbol ORDER BY last_seen DESC, rowid DESC) AS rn
				  FROM alert_drops
			) WHERE rn > ?
		)`, s.opts.PerSymbolCap); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func formatDropTime(t time.Time) string {
	return t.UTC().Format(dropTimeLayout)
}

// parseDropTime accepts the stored layout and the shorter form the SQLite
// driver returns when it coerces a DATETIME column through time.Time.
func parseDropTime(s string) (time.Time, error) {
	if t, err := time.Parse(dropTimeLayout, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func clipDetail(s string) string {
	if len(s) <= dropDetailMax {
		return s
	}
	b := []byte(s)[:dropDetailMax]
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b)
}
