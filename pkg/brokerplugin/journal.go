package brokerplugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"

	_ "modernc.org/sqlite"
)

// Journal reserves write IDs durably before invoking a broker. Never delete or
// rotate it while broker orders might still exist. It is not a portfolio ledger.
type Journal struct{ db *sql.DB }

func OpenJournal(path string) (*Journal, error) {
	if path == "" {
		return nil, errors.New("journal path required")
	}
	if path != ":memory:" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		_ = f.Close()
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{`PRAGMA busy_timeout=5000`, `PRAGMA journal_mode=WAL`, `PRAGMA synchronous=FULL`, `CREATE TABLE IF NOT EXISTS plugin_requests (key TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, result BLOB)`} {
		if _, err = db.Exec(q); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return &Journal{db: db}, nil
}

func (j *Journal) Close() error { return j.db.Close() }

// reserve returns fresh=false for in-flight, interrupted and completed writes.
// All three MUST avoid a second broker invocation, including after restart.
func (j *Journal) reserve(ctx context.Context, key, fingerprint string) (fresh bool, result *Order, conflict bool, err error) {
	r, err := j.db.ExecContext(ctx, `INSERT OR IGNORE INTO plugin_requests(key,fingerprint) VALUES(?,?)`, key, fingerprint)
	if err != nil {
		return false, nil, false, err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return false, nil, false, err
	}
	if n == 1 {
		return true, nil, false, nil
	}
	var previous string
	var raw []byte
	err = j.db.QueryRowContext(ctx, `SELECT fingerprint,result FROM plugin_requests WHERE key=?`, key).Scan(&previous, &raw)
	if err != nil {
		return false, nil, false, err
	}
	if previous != fingerprint {
		return false, nil, true, nil
	}
	if len(raw) > 0 {
		var o Order
		err = json.Unmarshal(raw, &o)
		return false, &o, false, err
	}
	return false, nil, false, nil
}

func (j *Journal) finish(ctx context.Context, key string, result Order) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = j.db.ExecContext(ctx, `UPDATE plugin_requests SET result=? WHERE key=?`, raw, key)
	return err
}
