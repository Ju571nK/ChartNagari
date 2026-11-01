package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/alertdrop"
)

type scriptDrops struct {
	rows []alertdrop.Summary
	err  error
	q    alertdrop.Query
}

func (s *scriptDrops) Record(alertdrop.Event) {}
func (s *scriptDrops) Recent(_ context.Context, q alertdrop.Query) ([]alertdrop.Summary, error) {
	s.q = q
	return s.rows, s.err
}

func TestGetAlertDropsDisabledByDefault(t *testing.T) {
	s := New(t.TempDir(), "")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/alert-drops?symbol=btcusdt", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp alertDropsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Enabled || resp.Drops == nil || len(resp.Drops) != 0 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestGetAlertDropsRejectsBadLimit(t *testing.T) {
	s := New(t.TempDir(), "")
	for _, raw := range []string{"nope", "0", "-1"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/alert-drops?limit="+raw, nil)
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("limit %q status %d", raw, rr.Code)
		}
	}
}

func TestGetAlertDropsMapsRows(t *testing.T) {
	seen := time.Date(2026, 11, 1, 12, 15, 0, 0, time.UTC)
	script := &scriptDrops{rows: []alertdrop.Summary{{
		Event: alertdrop.Event{
			Reason:    alertdrop.Cooldown,
			Symbol:    "BTCUSDT",
			Rule:      "ict_fvg",
			Timeframe: "1H",
			Direction: "LONG",
			Score:     14.2,
			Detail:    "4h0m0s",
		},
		Count:     37,
		FirstSeen: seen.Add(-time.Hour),
		LastSeen:  seen,
	}}}
	s := New(t.TempDir(), "").WithAlertDropStore(script)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/alert-drops?symbol=btcusdt&limit=50", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	if script.q.Symbol != "btcusdt" || script.q.Limit != 50 {
		t.Fatalf("query passed to store = %+v", script.q)
	}
	var resp struct {
		Enabled bool `json:"enabled"`
		Drops   []struct {
			Reason string `json:"reason"`
			Stage  string `json:"stage"`
			Knob   string `json:"knob"`
			Count  int64  `json:"count"`
			Detail string `json:"detail"`
		} `json:"drops"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Enabled || len(resp.Drops) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	row := resp.Drops[0]
	if row.Reason != "cooldown" || row.Stage != "notifier" || row.Count != 37 || row.Detail != "4h0m0s" || row.Knob == "" {
		t.Fatalf("row = %+v", row)
	}
}
