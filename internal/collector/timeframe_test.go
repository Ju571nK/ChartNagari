package collector

import (
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// 테스트용 1H 캔들 생성 헬퍼
func makeBar(symbol, tf string, hour int, o, h, l, c, v float64) models.OHLCV {
	base := time.Date(2024, 1, 1, hour, 0, 0, 0, time.UTC)
	return models.OHLCV{
		Symbol: symbol, Timeframe: tf,
		OpenTime: base,
		Open: o, High: h, Low: l, Close: c, Volume: v,
	}
}

func TestFloorTime_4H(t *testing.T) {
	cases := []struct {
		input    time.Time
		expected time.Time
	}{
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2024, 1, 1, 3, 59, 0, 0, time.UTC), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2024, 1, 1, 4, 0, 0, 0, time.UTC), time.Date(2024, 1, 1, 4, 0, 0, 0, time.UTC)},
		{time.Date(2024, 1, 1, 7, 30, 0, 0, time.UTC), time.Date(2024, 1, 1, 4, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		got := floorTime(tc.input, 4*time.Hour)
		if !got.Equal(tc.expected) {
			t.Errorf("floorTime(%v, 4H) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestRebuildHigherTF_4H_OHLCV(t *testing.T) {
	// 8개의 1H 캔들 → 2개의 4H 캔들
	bars1H := []models.OHLCV{
		// 첫 번째 4H 구간 (0~3시)
		makeBar("BTCUSDT", "1H", 0, 100, 110, 90, 105, 10),
		makeBar("BTCUSDT", "1H", 1, 105, 120, 100, 115, 20),
		makeBar("BTCUSDT", "1H", 2, 115, 125, 110, 120, 15),
		makeBar("BTCUSDT", "1H", 3, 120, 130, 115, 125, 12),
		// 두 번째 4H 구간 (4~7시)
		makeBar("BTCUSDT", "1H", 4, 125, 135, 120, 130, 18),
		makeBar("BTCUSDT", "1H", 5, 130, 140, 125, 135, 22),
		makeBar("BTCUSDT", "1H", 6, 135, 145, 130, 140, 25),
		makeBar("BTCUSDT", "1H", 7, 140, 150, 135, 145, 30),
	}

	result := RebuildHigherTF("BTCUSDT", bars1H)
	bars4H, ok := result["4H"]
	if !ok {
		t.Fatal("4H 결과 없음")
	}
	if len(bars4H) != 2 {
		t.Fatalf("4H 캔들 수: got %d, want 2", len(bars4H))
	}

	// 첫 번째 4H 캔들 검증
	b0 := bars4H[0]
	if b0.Open != 100 {
		t.Errorf("4H[0].Open = %v, want 100", b0.Open)
	}
	if b0.High != 130 {
		t.Errorf("4H[0].High = %v, want 130", b0.High)
	}
	if b0.Low != 90 {
		t.Errorf("4H[0].Low = %v, want 90", b0.Low)
	}
	if b0.Close != 125 {
		t.Errorf("4H[0].Close = %v, want 125", b0.Close)
	}
	expectedVol := 10.0 + 20 + 15 + 12
	if b0.Volume != expectedVol {
		t.Errorf("4H[0].Volume = %v, want %v", b0.Volume, expectedVol)
	}
}

func TestRebuildHigherTF_EmptyInput(t *testing.T) {
	result := RebuildHigherTF("BTCUSDT", nil)
	for _, tf := range []string{"4H", "1D", "1W"} {
		if bars, ok := result[tf]; ok && len(bars) > 0 {
			t.Errorf("빈 입력에서 %s 캔들이 생성됨", tf)
		}
	}
}

func TestRebuildHigherTF_SingleBar(t *testing.T) {
	bars := []models.OHLCV{
		makeBar("ETHUSDT", "1H", 0, 200, 210, 190, 205, 50),
	}
	result := RebuildHigherTF("ETHUSDT", bars)
	b4H := result["4H"]
	if len(b4H) != 1 {
		t.Fatalf("단일 1H → 4H 캔들 수: got %d, want 1", len(b4H))
	}
	if b4H[0].Open != 200 || b4H[0].High != 210 || b4H[0].Low != 190 {
		t.Errorf("단일 캔들 OHLC 불일치: %+v", b4H[0])
	}
}

func TestBinanceTFMap_AllTimeframes(t *testing.T) {
	required := []string{"1H", "4H", "1D", "1W"}
	for _, tf := range required {
		if _, ok := BinanceTFMap[tf]; !ok {
			t.Errorf("BinanceTFMap에 %s 누락", tf)
		}
	}
}

func TestBinanceIntervalToTF(t *testing.T) {
	cases := map[string]string{
		"1h": "1H",
		"4h": "4H",
		"1d": "1D",
		"1w": "1W",
	}
	for interval, want := range cases {
		got := binanceIntervalToTF(interval)
		if got != want {
			t.Errorf("binanceIntervalToTF(%q) = %q, want %q", interval, got, want)
		}
	}
}
