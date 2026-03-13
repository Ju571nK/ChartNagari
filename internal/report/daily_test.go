package report

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ── mock implementations ──────────────────────────────────────────────────────

type mockStore struct {
	signals map[string][]models.Signal
	ohlcv   map[string][]models.OHLCV
}

func (m *mockStore) GetSignalsByDate(symbol string, _ time.Time) ([]models.Signal, error) {
	return m.signals[symbol], nil
}

func (m *mockStore) GetOHLCV(symbol, _ string, _ int) ([]models.OHLCV, error) {
	return m.ohlcv[symbol], nil
}

type mockNotifier struct {
	messages []string
}

func (m *mockNotifier) Announce(_ context.Context, msg string) {
	m.messages = append(m.messages, msg)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeSignal(sym, dir string, score float64) models.Signal {
	return models.Signal{
		Symbol:    sym,
		Timeframe: "1D",
		Rule:      "ict_order_block",
		Direction: dir,
		Score:     score,
		CreatedAt: time.Now(),
	}
}

func makeOHLCV(sym string, close, prev float64) []models.OHLCV {
	return []models.OHLCV{
		{Symbol: sym, Timeframe: "1D", Close: close, OpenTime: time.Now()},
		{Symbol: sym, Timeframe: "1D", Close: prev, OpenTime: time.Now().AddDate(0, 0, -1)},
	}
}

func defaultCfg() appconfig.DailyReportConfig {
	return appconfig.DailyReportConfig{
		Enabled:       true,
		Time:          "09:00",
		Timezone:      "Asia/Seoul",
		AIMinScore:    8.0,
		OnlyIfSignals: false,
		Compact:       false,
	}
}

// ── test cases ────────────────────────────────────────────────────────────────

// TC1: only_if_signals=false → 신호 없어도 발송
func TestGenerate_NoSignals_AlwaysSend(t *testing.T) {
	store := &mockStore{
		signals: map[string][]models.Signal{"AAPL": {}},
		ohlcv:   map[string][]models.OHLCV{"AAPL": makeOHLCV("AAPL", 200, 198)},
	}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, []string{"AAPL"}, zerolog.Nop())

	cfg := defaultCfg()
	cfg.OnlyIfSignals = false

	if err := reporter.Generate(context.Background(), cfg, time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(notif.messages) == 0 {
		t.Error("expected message to be sent when only_if_signals=false")
	}
}

// TC2: only_if_signals=true → 신호 없으면 스킵
func TestGenerate_NoSignals_Skip(t *testing.T) {
	store := &mockStore{
		signals: map[string][]models.Signal{"AAPL": {}},
		ohlcv:   map[string][]models.OHLCV{"AAPL": makeOHLCV("AAPL", 200, 198)},
	}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, []string{"AAPL"}, zerolog.Nop())

	cfg := defaultCfg()
	cfg.OnlyIfSignals = true

	if err := reporter.Generate(context.Background(), cfg, time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(notif.messages) != 0 {
		t.Error("expected no message when only_if_signals=true and no signals")
	}
}

// TC3: 신호 있는 날 — 정상 포맷 생성
func TestGenerate_WithSignals_NormalFormat(t *testing.T) {
	store := &mockStore{
		signals: map[string][]models.Signal{
			"AAPL": {makeSignal("AAPL", "LONG", 12.0), makeSignal("AAPL", "LONG", 8.0)},
		},
		ohlcv: map[string][]models.OHLCV{"AAPL": makeOHLCV("AAPL", 214.50, 210.74)},
	}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, []string{"AAPL"}, zerolog.Nop())

	if err := reporter.Generate(context.Background(), defaultCfg(), time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(notif.messages) == 0 {
		t.Fatal("no messages sent")
	}
	msg := notif.messages[0]
	if !strings.Contains(msg, "AAPL") {
		t.Error("message should contain symbol AAPL")
	}
	if !strings.Contains(msg, "일일 리포트") {
		t.Error("message should contain '일일 리포트'")
	}
	if !strings.Contains(msg, "BULL") {
		t.Error("message should contain BULL count")
	}
}

// TC4: only_if_signals=true 이고 신호 있는 날 — 발송
func TestGenerate_WithSignals_OnlyIfSignalsTrue(t *testing.T) {
	store := &mockStore{
		signals: map[string][]models.Signal{
			"AAPL": {makeSignal("AAPL", "LONG", 12.0)},
		},
		ohlcv: map[string][]models.OHLCV{"AAPL": makeOHLCV("AAPL", 214.50, 210.74)},
	}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, []string{"AAPL"}, zerolog.Nop())

	cfg := defaultCfg()
	cfg.OnlyIfSignals = true

	if err := reporter.Generate(context.Background(), cfg, time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(notif.messages) == 0 {
		t.Error("expected message to be sent when there are signals and only_if_signals=true")
	}
}

// TC5: COMPACT 모드 — 축약 포맷 생성
func TestGenerate_CompactMode(t *testing.T) {
	store := &mockStore{
		signals: map[string][]models.Signal{
			"AAPL": {makeSignal("AAPL", "LONG", 10.0)},
			"MSFT": {makeSignal("MSFT", "SHORT", 9.0)},
		},
		ohlcv: map[string][]models.OHLCV{
			"AAPL": makeOHLCV("AAPL", 214.50, 210.74),
			"MSFT": makeOHLCV("MSFT", 380.0, 385.0),
		},
	}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, []string{"AAPL", "MSFT"}, zerolog.Nop())

	cfg := defaultCfg()
	cfg.Compact = true

	if err := reporter.Generate(context.Background(), cfg, time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(notif.messages) == 0 {
		t.Fatal("no messages sent")
	}
	msg := notif.messages[0]
	// compact format should have symbols on separate lines
	if !strings.Contains(msg, "AAPL") || !strings.Contains(msg, "MSFT") {
		t.Error("compact message should contain both symbols")
	}
	// compact format should NOT have *bold* markdown format
	if strings.Contains(msg, "━━━") {
		t.Error("compact message should not contain separator lines")
	}
}

// TC6: Telegram 길이 초과 — 분할 발송 (종목 6개 풀 리포트)
func TestGenerate_LongMessage_SplitChunks(t *testing.T) {
	syms := []string{"AAPL", "MSFT", "NVDA", "GOOGL", "AMZN", "TSLA"}
	signals := make(map[string][]models.Signal)
	ohlcv := make(map[string][]models.OHLCV)
	for _, sym := range syms {
		// each symbol gets lots of signals to make message long
		sigs := make([]models.Signal, 10)
		for i := range sigs {
			sigs[i] = models.Signal{
				Symbol:    sym,
				Timeframe: "1D",
				Rule:      strings.Repeat("ict_order_block_long_rule_name_here", 3),
				Direction: "LONG",
				Score:     float64(10 + i),
				Message:   strings.Repeat("This is a very long message for testing chunk splitting. ", 5),
				CreatedAt: time.Now(),
			}
		}
		signals[sym] = sigs
		ohlcv[sym] = makeOHLCV(sym, 200+float64(len(sym)), 195+float64(len(sym)))
	}

	store := &mockStore{signals: signals, ohlcv: ohlcv}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, syms, zerolog.Nop())

	if err := reporter.Generate(context.Background(), defaultCfg(), time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	// Verify all chunks are ≤4000 chars
	for i, msg := range notif.messages {
		if len(msg) > 4000 {
			t.Errorf("chunk %d exceeds 4000 chars: %d", i, len(msg))
		}
	}
}

// TC7: 주식 종목 필터 — 빈 주식 목록이면 아무것도 보내지 않음
func TestGenerate_EmptyStockSymbols(t *testing.T) {
	store := &mockStore{
		signals: map[string][]models.Signal{},
		ohlcv:   map[string][]models.OHLCV{},
	}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, []string{}, zerolog.Nop())

	if err := reporter.Generate(context.Background(), defaultCfg(), time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(notif.messages) != 0 {
		t.Error("expected no messages when stock symbols list is empty")
	}
}

// TC8: 스케줄러 시간 계산 — 내일 09:00 KST 정확히 계산
func TestNextFire_FutureSameDay(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Seoul")
	// 오늘 08:00 KST
	now := time.Date(2026, 3, 13, 8, 0, 0, 0, loc)
	dur, err := nextFire(now, "09:00", "Asia/Seoul")
	if err != nil {
		t.Fatalf("nextFire error: %v", err)
	}
	expected := time.Date(2026, 3, 13, 9, 0, 0, 0, loc)
	got := now.Add(dur)
	if got.Hour() != expected.Hour() || got.Minute() != expected.Minute() || got.Day() != expected.Day() {
		t.Errorf("expected fire at %v, got %v", expected, got)
	}
}

// TC9: 스케줄러 시간 계산 — 이미 지난 경우 다음날
func TestNextFire_AlreadyPassed(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Seoul")
	// 오늘 10:00 KST (09:00 지남)
	now := time.Date(2026, 3, 13, 10, 0, 0, 0, loc)
	dur, err := nextFire(now, "09:00", "Asia/Seoul")
	if err != nil {
		t.Fatalf("nextFire error: %v", err)
	}
	expected := time.Date(2026, 3, 14, 9, 0, 0, 0, loc)
	got := now.Add(dur)
	if got.Day() != expected.Day() || got.Hour() != expected.Hour() {
		t.Errorf("expected fire tomorrow at %v, got %v", expected, got)
	}
}

// TC10: 시간대 파싱 오류 — 명확한 에러 반환
func TestNextFire_InvalidTimezone(t *testing.T) {
	_, err := nextFire(time.Now(), "09:00", "Invalid/Zone")
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}

// TC11: 시간 포맷 오류 — 명확한 에러 반환
func TestNextFire_InvalidTimeFormat(t *testing.T) {
	_, err := nextFire(time.Now(), "9-00", "Asia/Seoul")
	if err == nil {
		t.Error("expected error for invalid time format")
	}
}

// TC12: OHLCV 데이터 없음 — graceful skip (에러 없이 메시지 발송)
func TestGenerate_MissingOHLCV_GracefulSkip(t *testing.T) {
	store := &mockStore{
		signals: map[string][]models.Signal{
			"AAPL": {makeSignal("AAPL", "LONG", 10.0)},
		},
		ohlcv: map[string][]models.OHLCV{
			"AAPL": {}, // 빈 OHLCV
		},
	}
	notif := &mockNotifier{}
	reporter := NewDailyReporter(store, notif, []string{"AAPL"}, zerolog.Nop())

	if err := reporter.Generate(context.Background(), defaultCfg(), time.Now()); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(notif.messages) == 0 {
		t.Error("expected message to be sent even without OHLCV data")
	}
	// message should indicate data issue gracefully
	if !strings.Contains(notif.messages[0], "AAPL") {
		t.Error("message should still contain symbol AAPL")
	}
}

// TC13: splitMessage — 짧은 메시지는 분할 안 됨
func TestSplitMessage_Short(t *testing.T) {
	msg := "Hello World"
	chunks := splitMessage(msg)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != msg {
		t.Errorf("expected %q, got %q", msg, chunks[0])
	}
}

// TC14: countDirections 방향 집계
func TestCountDirections(t *testing.T) {
	signals := []models.Signal{
		{Direction: "LONG"},
		{Direction: "LONG"},
		{Direction: "SHORT"},
		{Direction: "NEUTRAL"},
	}
	bull, bear := countDirections(signals)
	if bull != 2 {
		t.Errorf("expected 2 LONG, got %d", bull)
	}
	if bear != 1 {
		t.Errorf("expected 1 SHORT, got %d", bear)
	}
}
