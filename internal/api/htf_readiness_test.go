package api

import (
	"encoding/json"
	"github.com/Ju571nK/Chatter/internal/storage"
	"testing"
)

func TestHTFReadinessAPI(t *testing.T) {
	s, db := preparationServer(t)
	for _, query := range []string{"", "?symbol=TEST", "?symbol=TEST&timeframe=1D", "?symbol=ALL&timeframe=1H"} {
		if rr := do(t, s, "GET", "/api/backtest/htf-readiness"+query, nil); rr.Code != 400 {
			t.Fatalf("%s: %d", query, rr.Code)
		}
	}
	url := "/api/backtest/htf-readiness?symbol=TEST&timeframe=1H"
	rr := do(t, s, "GET", url, nil)
	var result storage.HTFReadiness
	if rr.Code != 200 || json.Unmarshal(rr.Body.Bytes(), &result) != nil || result.Ready || len(result.Buckets) != 10 || len(result.Blockers) != 4 {
		t.Fatal(rr.Body.String())
	}
	db.Close()
	if rr := do(t, s, "GET", url, nil); rr.Code != 500 {
		t.Fatal("storage error hidden")
	}
	if rr := do(t, setupTest(t), "GET", url, nil); rr.Code != 503 {
		t.Fatal("missing storage hidden")
	}
}
