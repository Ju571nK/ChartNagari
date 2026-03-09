package storage

import (
	"os"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestSaveAndGetSignal_WithAIInterpretation(t *testing.T) {
	f, err := os.CreateTemp("", "signals_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	db, err := New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	sig := models.Signal{
		Symbol:           "BTCUSDT",
		Timeframe:        "4H",
		Rule:             "ict_order_block",
		Direction:        "LONG",
		Score:            8.5,
		Message:          "OB 진입 신호",
		AIInterpretation: "1. 시장구조: 상승 추세\n2. 진입근거: OB 재진입\n3. 위험요인: 전저점 이탈\n4. 결론: LONG",
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
	}

	if err := db.SaveSignal(sig); err != nil {
		t.Fatalf("SaveSignal: %v", err)
	}

	sigs, err := db.GetSignals("BTCUSDT", 10)
	if err != nil {
		t.Fatalf("GetSignals: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("want 1 signal, got %d", len(sigs))
	}

	got := sigs[0]
	if got.AIInterpretation != sig.AIInterpretation {
		t.Errorf("AIInterpretation mismatch\nwant: %q\ngot:  %q", sig.AIInterpretation, got.AIInterpretation)
	}
	if got.Symbol != sig.Symbol {
		t.Errorf("Symbol: want %q, got %q", sig.Symbol, got.Symbol)
	}
	if got.Score != sig.Score {
		t.Errorf("Score: want %v, got %v", sig.Score, got.Score)
	}
}
