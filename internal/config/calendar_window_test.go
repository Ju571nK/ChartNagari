package config

import "testing"

// The economic-calendar alert window must be clamped to [5, 1440] minutes so a
// typo can neither disable alerts (too small) nor schedule them days out (too large).

func TestLoad_CalendarAlertWindowDefault(t *testing.T) {
	t.Setenv("CALENDAR_ALERT_WINDOW", "")
	cfg, err := Load("", ollamaMinimalConfigDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Finnhub.AlertWindowMinutes != 30 {
		t.Fatalf("expected default 30, got %d", cfg.Finnhub.AlertWindowMinutes)
	}
}

func TestLoad_CalendarAlertWindowInRange(t *testing.T) {
	t.Setenv("CALENDAR_ALERT_WINDOW", "60")
	cfg, err := Load("", ollamaMinimalConfigDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Finnhub.AlertWindowMinutes != 60 {
		t.Fatalf("expected 60, got %d", cfg.Finnhub.AlertWindowMinutes)
	}
}

func TestLoad_CalendarAlertWindowClampLow(t *testing.T) {
	t.Setenv("CALENDAR_ALERT_WINDOW", "2")
	cfg, err := Load("", ollamaMinimalConfigDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Finnhub.AlertWindowMinutes != 5 {
		t.Fatalf("expected clamp to 5, got %d", cfg.Finnhub.AlertWindowMinutes)
	}
}

func TestLoad_CalendarAlertWindowClampHigh(t *testing.T) {
	t.Setenv("CALENDAR_ALERT_WINDOW", "5000")
	cfg, err := Load("", ollamaMinimalConfigDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Finnhub.AlertWindowMinutes != 1440 {
		t.Fatalf("expected clamp to 1440, got %d", cfg.Finnhub.AlertWindowMinutes)
	}
}
