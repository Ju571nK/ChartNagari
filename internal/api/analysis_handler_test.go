package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Ju571nK/Chatter/internal/analyst"
	"github.com/Ju571nK/Chatter/internal/storage"
)

type analysisAnnouncer struct{ calls int }

func (a *analysisAnnouncer) Announce(context.Context, string) { a.calls++ }

type failedDirector struct{}

func (failedDirector) Analyze(context.Context, analyst.AnalystInput) analyst.ScenarioResult {
	return analyst.ScenarioResult{Symbol: "SPCX", Status: "failed", Final: "ERROR", AggregatorReason: "Model unavailable"}
}

func TestFailedAnalysisAPI(t *testing.T) {
	s := setupTest(t)
	db, err := storage.New(t.TempDir() + "/analysis.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s.WithFullStore(db)
	s.WithAnalystDirector(failedDirector{})
	rr := do(t, s, http.MethodPost, "/api/analysis/full", map[string]string{"symbol": "SPCX"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var result analyst.ScenarioResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.Final != "ERROR" || result.ID == 0 {
		t.Fatalf("unsafe response: %+v", result)
	}
	rr = do(t, s, http.MethodGet, "/api/analysis/history?symbol=SPCX", nil)
	var rows []storage.AnalysisRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "failed" {
		t.Fatalf("unsafe history: %+v", rows)
	}
}

func TestAnalysisExportRejectsFailures(t *testing.T) {
	s := setupTest(t)
	a := &analysisAnnouncer{}
	s.WithAnnouncer(a)
	for _, result := range []analyst.ScenarioResult{
		{Status: "failed", Final: "ERROR"},
		{Final: "SIDEWAYS", Confidence: "LOW"},
	} {
		rr := do(t, s, http.MethodPost, "/api/analysis/export", map[string]any{"result": result})
		if rr.Code != http.StatusUnprocessableEntity || a.calls != 0 {
			t.Fatalf("failed result exported: %d, calls %d", rr.Code, a.calls)
		}
	}
	rr := do(t, s, http.MethodPost, "/api/analysis/export", map[string]any{"result": analyst.ScenarioResult{Final: "SIDEWAYS", Confidence: "HIGH", SidewaysPct: 100}})
	if rr.Code != http.StatusOK || a.calls != 1 {
		t.Fatalf("valid export rejected: %d, calls %d", rr.Code, a.calls)
	}
}
