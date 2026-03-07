package storage

import (
	"os"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "test_*.db")
	if err != nil {
		t.Fatalf("임시 DB 파일 생성 실패: %v", err)
	}
	f.Close()

	db, err := New(f.Name())
	if err != nil {
		t.Fatalf("DB 초기화 실패: %v", err)
	}
	return db, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

func makeTestBar(symbol, tf string, hoursAgo int, close float64) models.OHLCV {
	t := time.Now().Add(-time.Duration(hoursAgo) * time.Hour).Truncate(time.Hour).UTC()
	return models.OHLCV{
		Symbol: symbol, Timeframe: tf,
		OpenTime: t,
		Open: close - 5, High: close + 5, Low: close - 10, Close: close, Volume: 100,
	}
}

func TestSaveAndGetOHLCV(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	bar := makeTestBar("BTCUSDT", "1H", 1, 50000)
	if err := db.SaveOHLCV(bar, "binance"); err != nil {
		t.Fatalf("SaveOHLCV 실패: %v", err)
	}

	bars, err := db.GetOHLCV("BTCUSDT", "1H", 10)
	if err != nil {
		t.Fatalf("GetOHLCV 실패: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("캔들 수: got %d, want 1", len(bars))
	}
	if bars[0].Close != 50000 {
		t.Errorf("Close: got %v, want 50000", bars[0].Close)
	}
}

func TestSaveOHLCV_DuplicateReplace(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	bar := makeTestBar("BTCUSDT", "1H", 1, 50000)
	_ = db.SaveOHLCV(bar, "binance")

	// 같은 시간 데이터를 다른 close로 저장 → 교체되어야 함
	bar.Close = 55000
	if err := db.SaveOHLCV(bar, "binance"); err != nil {
		t.Fatalf("중복 저장 실패: %v", err)
	}

	bars, _ := db.GetOHLCV("BTCUSDT", "1H", 10)
	if len(bars) != 1 {
		t.Fatalf("중복 저장 후 캔들 수: got %d, want 1", len(bars))
	}
	if bars[0].Close != 55000 {
		t.Errorf("교체 후 Close: got %v, want 55000", bars[0].Close)
	}
}

func TestSaveOHLCVBatch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	bars := []models.OHLCV{
		makeTestBar("ETHUSDT", "1H", 3, 3000),
		makeTestBar("ETHUSDT", "1H", 2, 3100),
		makeTestBar("ETHUSDT", "1H", 1, 3200),
	}
	if err := db.SaveOHLCVBatch(bars, "binance"); err != nil {
		t.Fatalf("SaveOHLCVBatch 실패: %v", err)
	}

	result, err := db.GetOHLCV("ETHUSDT", "1H", 10)
	if err != nil {
		t.Fatalf("GetOHLCV 실패: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("배치 저장 후 캔들 수: got %d, want 3", len(result))
	}
}

func TestGetOHLCV_MultipleSymbols(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_ = db.SaveOHLCV(makeTestBar("BTCUSDT", "1H", 1, 50000), "binance")
	_ = db.SaveOHLCV(makeTestBar("ETHUSDT", "1H", 1, 3000), "binance")

	btcBars, _ := db.GetOHLCV("BTCUSDT", "1H", 10)
	ethBars, _ := db.GetOHLCV("ETHUSDT", "1H", 10)

	if len(btcBars) != 1 || len(ethBars) != 1 {
		t.Errorf("심볼 분리 오류: BTC=%d ETH=%d", len(btcBars), len(ethBars))
	}
}

func TestGetOHLCVSince(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	for i := 5; i >= 1; i-- {
		_ = db.SaveOHLCV(makeTestBar("BTCUSDT", "1H", i, float64(50000+i*100)), "binance")
	}

	since := time.Now().Add(-3 * time.Hour).Truncate(time.Hour).UTC()
	bars, err := db.GetOHLCVSince("BTCUSDT", "1H", since)
	if err != nil {
		t.Fatalf("GetOHLCVSince 실패: %v", err)
	}
	// since 이후 캔들: 3시간 전, 2시간 전, 1시간 전 = 3개
	if len(bars) != 3 {
		t.Errorf("GetOHLCVSince 캔들 수: got %d, want 3", len(bars))
	}
}
