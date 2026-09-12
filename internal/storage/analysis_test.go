package storage

import (
	"testing"

	"github.com/Ju571nK/Chatter/internal/analyst"
)

func TestLegacyFailedAnalysisRead(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// Insert the historical representation directly, without rewriting user data.
	_, err = db.Conn().Exec(`INSERT INTO analysis_history (symbol, final, confidence, bull_pct, bear_pct, sideways_pct, result_json, created_at) VALUES ('SPCX', 'SIDEWAYS', 'LOW', 0, 0, 0, '{"symbol":"SPCX","final":"SIDEWAYS","confidence":"LOW"}', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.GetAnalysisHistory("SPCX", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("history: %v %v", rows, err)
	}
	if rows[0].Status != "failed" || rows[0].Final != "ERROR" || rows[0].Confidence != "" {
		t.Fatalf("unsafe summary: %+v", rows[0])
	}
	detail, err := db.GetAnalysisByID(rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Result.Status != "failed" || detail.Result.Final != "ERROR" || detail.Result.ID != rows[0].ID {
		t.Fatalf("unsafe detail: %+v", detail.Result)
	}
	var stored string
	if err := db.Conn().QueryRow(`SELECT final FROM analysis_history WHERE id = ?`, rows[0].ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "SIDEWAYS" {
		t.Fatal("read changed stored history")
	}
}

func TestAnalysisStatusRoundTrip(t *testing.T) {
	db, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, result := range []analyst.ScenarioResult{
		{Symbol: "SPCX", Status: "failed", Final: "ERROR"},
		{Symbol: "SPCX", Final: "SIDEWAYS", Confidence: "HIGH", SidewaysPct: 100},
	} {
		id, err := db.SaveAnalysis(result)
		if err != nil {
			t.Fatal(err)
		}
		detail, err := db.GetAnalysisByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Result.Failed() != result.Failed() {
			t.Fatalf("status changed: %+v", detail)
		}
	}
}
