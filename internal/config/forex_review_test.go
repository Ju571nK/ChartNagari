package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeDXYFromExistingYAMLAndHonorOptOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchlist.yaml")
	wl := WatchlistConfig{}
	wl.Symbols.Forex = []SymbolEntry{{Symbol: "EURUSD", Enabled: true}}
	if changed, err := InitializeDXY(path, &wl); err != nil || !changed || len(wl.Symbols.Indices) != 1 {
		t.Fatalf("init: %+v %v %v", wl, changed, err)
	}
	wl.Symbols.Indices = nil // User removed the index after initialization.
	if changed, err := InitializeDXY(path, &wl); err != nil || changed || len(wl.Symbols.Indices) != 0 {
		t.Fatalf("opt-out: %+v %v %v", wl, changed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "dxy_auto_initialized: true") {
		t.Fatalf("marker missing: %s %v", data, err)
	}
}

func TestForexMacroWindowDefaultAndSpreadValidation(t *testing.T) {
	t.Setenv("CALENDAR_FX_ALERT_WINDOW", "")
	settings, err := LoadSettings(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || settings.Finnhub.ForexAlertWindowMinutes != 60 {
		t.Fatalf("settings: %+v %v", settings, err)
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		cfg := SymbolProfilesConfig{SymbolOverrides: map[string]SymbolOverride{"EURUSD": {SpreadPips: &bad}}}
		if err := SaveSymbolProfiles(filepath.Join(t.TempDir(), "profiles.yaml"), cfg); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}
