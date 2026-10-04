package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateForexPair(t *testing.T) {
	for _, tc := range []struct {
		pair  string
		valid bool
	}{{"EURUSD", true}, {"USDJPY", true}, {"GBPAUD", true}, {"XAUUSD", true}, {"XAGUSD", true}, {"eurusd", false}, {"EUR/US", false}, {"ZZZUSD", false}, {"USDXAU", false}, {"USDUSD", false}} {
		err := ValidateForexPair(tc.pair)
		if (err == nil) != tc.valid {
			t.Errorf("%s valid=%v err=%v", tc.pair, tc.valid, err)
		}
	}
}

func TestForexSettingsAndLegacyWatchlist(t *testing.T) {
	var old WatchlistConfig
	if err := yaml.Unmarshal([]byte("symbols:\n  crypto:\n    - symbol: BTCUSDT\n      enabled: true\n  stocks:\n    - symbol: AAPL\n      enabled: true\n"), &old); err != nil {
		t.Fatal(err)
	}
	if len(old.Symbols.Forex) != 0 || old.AssetClass("BTCUSDT") != "crypto" || old.AssetClass("AAPL") != "stock" || old.AssetClass("UNKNOWN") != "stock" {
		t.Fatalf("old watchlist: %+v", old)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte("version: 1\nyahoo:\n  poll_interval: 60\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(filepath.Join(dir, "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Forex.Provider != "auto" || s.Forex.OANDA.Environment != "practice" {
		t.Fatalf("defaults: %+v", s.Forex)
	}
	s.Forex.OANDA.Token = "secret"
	s.Forex.Provider = "oanda"
	if err := SaveSettings(filepath.Join(dir, "settings.yaml"), s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSettings(filepath.Join(dir, "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Forex.OANDA.Token != "secret" || loaded.ToMap()["OANDA_TOKEN"] != "secret" {
		t.Fatal("token lost")
	}
}

func TestLegacyAlertDefaultsForexMultipliers(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"rules.yaml": "rules: []\n", "watchlist.yaml": "symbols:\n  stocks: []\n", "alert.yaml": "stock_tp_mult: 2\nstock_sl_mult: 1\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := Load(filepath.Join(dir, "missing.env"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alert.ForexTPMult != 1.5 || cfg.Alert.ForexSLMult != 1 {
		t.Fatalf("forex defaults: %+v", cfg.Alert)
	}
}
