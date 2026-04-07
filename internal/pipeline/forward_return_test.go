package pipeline

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── mock forward return DB ──────────────────────────────────────────────────

type mockFRDB struct {
	signals []storage.SignalForForwardReturn
	updated map[int64][4]float64
}

func (m *mockFRDB) GetSignalsNeedingForwardReturn(minAgeDays int) ([]storage.SignalForForwardReturn, error) {
	return m.signals, nil
}

func (m *mockFRDB) UpdateForwardReturns(signalID int64, r5, r10, r20, r40 float64) error {
	if m.updated == nil {
		m.updated = make(map[int64][4]float64)
	}
	m.updated[signalID] = [4]float64{r5, r10, r20, r40}
	return nil
}

// ── mock OHLCV reader ───────────────────────────────────────────────────────

type mockFROHLCV struct {
	bars map[string][]models.OHLCV
}

func (m *mockFROHLCV) GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	return m.bars[symbol+"|"+timeframe], nil
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestUpdateForwardReturns_ComputesReturns(t *testing.T) {
	now := time.Now()
	signalTime := now.Add(-45 * 24 * time.Hour) // 45 days ago

	db := &mockFRDB{
		signals: []storage.SignalForForwardReturn{
			{
				ID:         1,
				Symbol:     "AAPL",
				Timeframe:  "1D",
				EntryPrice: 100.0,
				CreatedAt:  signalTime,
			},
		},
	}

	// Create bars at 5, 10, 20, 40 days after signal
	ohlcv := &mockFROHLCV{
		bars: map[string][]models.OHLCV{
			"AAPL|1D": {
				// DESC order: newest first
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(40 * 24 * time.Hour), Close: 120.0}, // +20%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(20 * 24 * time.Hour), Close: 115.0}, // +15%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(10 * 24 * time.Hour), Close: 110.0}, // +10%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime.Add(5 * 24 * time.Hour), Close: 105.0},  // +5%
				{Symbol: "AAPL", Timeframe: "1D", OpenTime: signalTime, Close: 100.0},
			},
		},
	}

	UpdateForwardReturns(db, ohlcv, zerolog.Nop())

	if len(db.updated) != 1 {
		t.Fatalf("expected 1 updated signal, got %d", len(db.updated))
	}

	returns := db.updated[1]
	// r5d = (105 - 100) / 100 * 100 = 5.0%
	if returns[0] < 4.9 || returns[0] > 5.1 {
		t.Errorf("expected r5d ~5.0%%, got %.2f%%", returns[0])
	}
	// r10d = (110 - 100) / 100 * 100 = 10.0%
	if returns[1] < 9.9 || returns[1] > 10.1 {
		t.Errorf("expected r10d ~10.0%%, got %.2f%%", returns[1])
	}
	// r20d = 15%
	if returns[2] < 14.9 || returns[2] > 15.1 {
		t.Errorf("expected r20d ~15.0%%, got %.2f%%", returns[2])
	}
	// r40d = 20%
	if returns[3] < 19.9 || returns[3] > 20.1 {
		t.Errorf("expected r40d ~20.0%%, got %.2f%%", returns[3])
	}
}

func TestUpdateForwardReturns_PartialData(t *testing.T) {
	now := time.Now()
	signalTime := now.Add(-8 * 24 * time.Hour) // 8 days ago — only 5d should be computed

	db := &mockFRDB{
		signals: []storage.SignalForForwardReturn{
			{
				ID:         2,
				Symbol:     "BTCUSDT",
				Timeframe:  "1D",
				EntryPrice: 50000.0,
				CreatedAt:  signalTime,
			},
		},
	}

	ohlcv := &mockFROHLCV{
		bars: map[string][]models.OHLCV{
			"BTCUSDT|1D": {
				{Symbol: "BTCUSDT", Timeframe: "1D", OpenTime: signalTime.Add(5 * 24 * time.Hour), Close: 52000.0},
				{Symbol: "BTCUSDT", Timeframe: "1D", OpenTime: signalTime, Close: 50000.0},
			},
		},
	}

	UpdateForwardReturns(db, ohlcv, zerolog.Nop())

	if len(db.updated) != 1 {
		t.Fatalf("expected 1 updated signal, got %d", len(db.updated))
	}

	returns := db.updated[2]
	// r5d = (52000 - 50000) / 50000 * 100 = 4.0%
	if returns[0] < 3.9 || returns[0] > 4.1 {
		t.Errorf("expected r5d ~4.0%%, got %.2f%%", returns[0])
	}
	// r10d, r20d, r40d should remain 0 (not enough time elapsed)
	if returns[1] != 0 || returns[2] != 0 || returns[3] != 0 {
		t.Errorf("expected r10d/r20d/r40d = 0, got %.2f/%.2f/%.2f", returns[1], returns[2], returns[3])
	}
}

func TestUpdateForwardReturns_NoEntryPrice(t *testing.T) {
	db := &mockFRDB{
		signals: []storage.SignalForForwardReturn{
			{
				ID:         3,
				Symbol:     "TSLA",
				Timeframe:  "1D",
				EntryPrice: 0, // no entry price
				CreatedAt:  time.Now().Add(-10 * 24 * time.Hour),
			},
		},
	}

	ohlcv := &mockFROHLCV{bars: map[string][]models.OHLCV{}}

	UpdateForwardReturns(db, ohlcv, zerolog.Nop())

	if len(db.updated) != 0 {
		t.Errorf("expected no updates for signal without entry price, got %d", len(db.updated))
	}
}

func TestFindCloseNearDate(t *testing.T) {
	target := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	bars := []models.OHLCV{
		{OpenTime: time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC), Close: 102.0},
		{OpenTime: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC), Close: 101.0}, // closest: 1 day off
		{OpenTime: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), Close: 99.0},
	}

	got := findCloseNearDate(bars, target)
	if got != 101.0 {
		t.Errorf("expected 101.0 (closest bar), got %.1f", got)
	}

	// Test out of tolerance
	farTarget := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	got = findCloseNearDate(bars, farTarget)
	if got != 0 {
		t.Errorf("expected 0 (no bar in tolerance), got %.1f", got)
	}
}
