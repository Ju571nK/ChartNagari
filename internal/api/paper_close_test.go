package api

import (
	"math"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/paper"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestClosePaperPosition_UsesBodyPrice(t *testing.T) {
	db := openPaperDB(t)
	id := saveOpen(t, db, paper.PaperPosition{
		Symbol: "BTCUSDT", Timeframe: "1H", Rule: "rsi", Direction: "LONG",
		EntryPrice: 100, TP: 110, SL: 90, EntryTime: time.Now().UTC(),
	})
	srv := setupTest(t)
	srv.WithPaperStore(db)

	w := do(t, srv, http.MethodPost, "/api/paper/positions/"+strconv.FormatInt(id, 10)+"/close", map[string]float64{"exit_price": 110})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	closed, err := db.GetClosedPositions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(closed) != 1 || closed[0].Status != "CLOSED_MANUAL" || closed[0].ExitPrice != 110 {
		t.Fatalf("closed row: %+v", closed)
	}
	if math.Abs(closed[0].PnLPct-10) > 1e-9 {
		t.Fatalf("pnl_pct = %v, want 10", closed[0].PnLPct)
	}

	again := do(t, srv, http.MethodPost, "/api/paper/positions/"+strconv.FormatInt(id, 10)+"/close", map[string]float64{"exit_price": 110})
	if again.Code != http.StatusNotFound {
		t.Fatalf("second close status %d, want 404", again.Code)
	}
}

func TestClosePaperPosition_UsesLatestClose(t *testing.T) {
	db := openPaperDB(t)
	id := saveOpen(t, db, paper.PaperPosition{
		Symbol: "ETHUSDT", Timeframe: "1H", Rule: "ema", Direction: "SHORT",
		EntryPrice: 100, TP: 90, SL: 110, EntryTime: time.Now().UTC(),
	})
	if err := db.SaveOHLCV(models.OHLCV{
		Symbol: "ETHUSDT", Timeframe: "1H", OpenTime: time.Now().UTC(),
		Open: 100, High: 101, Low: 89, Close: 90, Volume: 1,
	}, "binance"); err != nil {
		t.Fatal(err)
	}
	srv := setupTest(t)
	srv.WithPaperStore(db)
	srv.WithChartStore(db)

	w := do(t, srv, http.MethodPost, "/api/paper/positions/"+strconv.FormatInt(id, 10)+"/close", map[string]float64{})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	closed, err := db.GetClosedPositions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(closed) != 1 || closed[0].ExitPrice != 90 || math.Abs(closed[0].PnLPct-10) > 1e-9 {
		t.Fatalf("closed row: %+v", closed)
	}
}

func TestClosePaperPosition_ForexPips(t *testing.T) {
	db := openPaperDB(t)
	id := saveOpen(t, db, paper.PaperPosition{
		Symbol: "EURUSD", Timeframe: "1H", Rule: "test", Direction: "LONG",
		EntryPrice: 1.1, TP: 1.102, SL: 1.098, EntryTime: time.Now().UTC(),
		AssetClass: models.AssetForex, PipSize: 0.0001, SpreadPips: 1,
		SourceIdentity: "yahoo:public:EURUSD=X",
	})
	srv := setupTest(t)
	srv.WithPaperStore(db)

	w := do(t, srv, http.MethodPost, "/api/paper/positions/"+strconv.FormatInt(id, 10)+"/close", map[string]float64{"exit_price": 1.102})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	closed, err := db.GetClosedPositions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(closed) != 1 || closed[0].Status != "CLOSED_MANUAL" || math.Abs(closed[0].PnLPips-19) > 1e-8 {
		t.Fatalf("closed row: %+v", closed)
	}
}

func TestClosePaperPosition_RejectsMissingPrice(t *testing.T) {
	db := openPaperDB(t)
	id := saveOpen(t, db, paper.PaperPosition{
		Symbol: "BTCUSDT", Timeframe: "1H", Rule: "rsi", Direction: "LONG",
		EntryPrice: 100, EntryTime: time.Now().UTC(),
	})
	srv := setupTest(t)
	srv.WithPaperStore(db)

	w := do(t, srv, http.MethodPost, "/api/paper/positions/"+strconv.FormatInt(id, 10)+"/close", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
	open, err := db.GetAllOpenPositions()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != id {
		t.Fatalf("position should stay open: %+v", open)
	}
}

func openPaperDB(t *testing.T) *storage.DB {
	t.Helper()
	f, err := os.CreateTemp("", "paper-close-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	db, err := storage.New(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(f.Name())
	})
	return db
}

func saveOpen(t *testing.T, db *storage.DB, pos paper.PaperPosition) int64 {
	t.Helper()
	id, err := db.SavePaperPosition(pos)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
