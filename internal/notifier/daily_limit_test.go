package notifier

import (
	"testing"
	"time"
)

func TestDailyLimit_NoLimit(t *testing.T) {
	l := NewDailyLimit()
	for i := 0; i < 100; i++ {
		if !l.Allow("BTCUSDT", 0) {
			t.Errorf("limit=0 should always allow, blocked at %d", i)
		}
	}
}

func TestDailyLimit_BlocksAtLimit(t *testing.T) {
	l := NewDailyLimit()
	for i := 0; i < 3; i++ {
		if !l.Allow("BTCUSDT", 3) {
			t.Errorf("call %d blocked but limit=3", i)
		}
	}
	if l.Allow("BTCUSDT", 3) {
		t.Errorf("4th call should be blocked")
	}
}

func TestDailyLimit_PerSymbol(t *testing.T) {
	l := NewDailyLimit()
	for i := 0; i < 3; i++ {
		l.Allow("BTCUSDT", 3)
	}
	if !l.Allow("ETHUSDT", 3) {
		t.Errorf("ETHUSDT bucket should be independent of BTCUSDT")
	}
}

func TestDailyLimit_DayRollover(t *testing.T) {
	l := NewDailyLimit()
	clock := time.Date(2026, 5, 8, 23, 59, 0, 0, time.UTC)
	l.setClock(func() time.Time { return clock })

	for i := 0; i < 3; i++ {
		l.Allow("BTCUSDT", 3)
	}
	if l.Allow("BTCUSDT", 3) {
		t.Fatal("expected block at day-end")
	}

	// Next day.
	clock = time.Date(2026, 5, 9, 0, 0, 1, 0, time.UTC)
	l.setClock(func() time.Time { return clock })
	if !l.Allow("BTCUSDT", 3) {
		t.Errorf("new day should reset bucket")
	}
}
