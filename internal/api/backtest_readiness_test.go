package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/backtest"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func preparationServer(t *testing.T) (*Server, *storage.DB) {
	t.Helper()
	s := setupTest(t)
	db, err := storage.New(t.TempDir() + "/preview.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s.WithChartStore(db)
	s.WithBacktestRunner(backtest.NewRunner(db, backtest.New(nil, engine.RuleConfig{}, backtest.DefaultConfig())))
	return s, db
}

func seedPreparationBars(t *testing.T, db *storage.DB, symbol string, count int) {
	t.Helper()
	bars := make([]models.OHLCV, count)
	for i := range bars {
		bars[i] = models.OHLCV{Symbol: symbol, Timeframe: "1H", OpenTime: time.Unix(1700000000+int64(i)*3600, 0), Open: 100, High: 102, Low: 99, Close: 101, Volume: 1000}
	}
	if err := db.SaveOHLCVBatch(bars, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestBacktestReadinessAndExecutionBoundary(t *testing.T) {
	s, db := preparationServer(t)
	for _, count := range []int{0, 201, 202} {
		seedPreparationBars(t, db, "SPCX", count)
		rr := do(t, s, "GET", "/api/backtest/readiness?symbol=SPCX&timeframe=1H", nil)
		var data backtest.Readiness
		if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if rr.Code != 200 || data.Bars != count || data.Ready != (count >= 202) {
			t.Fatalf("unexpected readiness: %d %+v", rr.Code, data)
		}
		rr = do(t, s, "POST", "/api/backtest", map[string]any{"symbol": "SPCX", "timeframe": "1H"})
		if count < 202 && rr.Code != 422 {
			t.Fatal("insufficient data produced normal results")
		}
		if count == 202 {
			if rr.Code != 200 {
				t.Fatal(rr.Body.String())
			}
			var result backtest.BacktestResult
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil || result.Bars != 202 || result.Trades != 0 {
				t.Fatalf("invalid zero trade response: %+v %v", result, err)
			}
		}
	}
	db.Close()
	if rr := do(t, s, "GET", "/api/backtest/readiness?symbol=SPCX&timeframe=1H", nil); rr.Code != 500 {
		t.Fatal("storage failure not reported")
	}
}
