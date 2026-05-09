package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Ju571nK/Chatter/internal/marks"
	"github.com/Ju571nK/Chatter/internal/storage"
)

func newMarksTestServer(t *testing.T, apiToken string) (*Server, *storage.DB, *storage.SignalMarkStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "marks.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := storage.NewSignalMarkStore(db)
	agg := marks.NewAggregator(db, store)
	s := &Server{
		apiToken:   apiToken,
		markStore:  store,
		aggregator: agg,
	}
	return s, db, store
}

func seedSignalForAPI(t *testing.T, db *storage.DB) int64 {
	t.Helper()
	res, err := db.Conn().Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at)
		VALUES ('BTCUSDT','1H','ict_order_block','LONG',10.0,'','',0,0,'',0,0)`)
	if err != nil {
		t.Fatalf("seed signal: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func TestPostMark_Took(t *testing.T) {
	s, db, store := newMarksTestServer(t, "")
	id := seedSignalForAPI(t, db)
	body := []byte(`{"action":"took"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	got, _ := store.Get(id)
	if got == nil || got.Status != "TOOK" {
		t.Errorf("DB state = %#v, want status TOOK", got)
	}
}

func TestPostMark_InvalidAction(t *testing.T) {
	s, db, _ := newMarksTestServer(t, "")
	id := seedSignalForAPI(t, db)
	body := []byte(`{"action":"explode"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPostMark_UnknownSignal(t *testing.T) {
	s, _, _ := newMarksTestServer(t, "")
	body := []byte(`{"action":"took"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/99999", bytes.NewReader(body))
	req.SetPathValue("signal_id", "99999")
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestPostMark_AuthRequired(t *testing.T) {
	s, db, _ := newMarksTestServer(t, "secret-token")
	id := seedSignalForAPI(t, db)
	body := []byte(`{"action":"took"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.postMark(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", w.Code)
	}
	// With valid token.
	req2 := httptest.NewRequest(http.MethodPost, "/api/marks/"+strconv.FormatInt(id, 10), bytes.NewReader(body))
	req2.SetPathValue("signal_id", strconv.FormatInt(id, 10))
	req2.Header.Set("Authorization", "Bearer secret-token")
	w2 := httptest.NewRecorder()
	s.postMark(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("with token: status = %d, want 200", w2.Code)
	}
}

func TestGetPending(t *testing.T) {
	s, db, store := newMarksTestServer(t, "")
	id1 := seedSignalForAPI(t, db)
	id2 := seedSignalForAPI(t, db)
	_, _ = store.Mark(id1, "took") // id1 marked, id2 unmarked

	req := httptest.NewRequest(http.MethodGet, "/api/marks/pending?since="+strconv.FormatInt(0, 10), nil)
	w := httptest.NewRecorder()
	s.getPending(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Fatalf("len(pending) = %d, want 1", len(resp))
	}
	sig := resp[0]["signal"].(map[string]any)
	if int64(sig["id"].(float64)) != id2 {
		t.Errorf("expected unmarked id %d, got %v", id2, sig["id"])
	}
}

func TestGetRollup(t *testing.T) {
	s, db, store := newMarksTestServer(t, "")
	id := seedSignalForAPI(t, db)
	_, _ = store.Mark(id, "took")
	_, _ = store.Mark(id, "win")

	req := httptest.NewRequest(http.MethodGet, "/api/marks/rollup?by=rule", nil)
	w := httptest.NewRecorder()
	s.getRollup(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["by"] != "rule" {
		t.Errorf("by = %v, want rule", resp["by"])
	}
	rows := resp["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0].(map[string]any)
	if row["key"] != "ict_order_block" {
		t.Errorf("key = %v", row["key"])
	}
	if int(row["wins"].(float64)) != 1 {
		t.Errorf("wins = %v", row["wins"])
	}
}
