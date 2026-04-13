package alpaca

import (
	"strings"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	base := Config{
		AlpacaAPIURL:     "https://paper-api.alpaca.markets",
		AlpacaAPIKey:     "k",
		AlpacaAPISecret:  "s",
		FeedbackURL:      "http://127.0.0.1:8080/api/execution/feedback",
		PluginSecret:     "shh",
		PluginID:         "alpaca-paper",
		ListenAddr:       ":9100",
		NotionalPerTrade: 1000,
		DBPath:           "./x.db",
	}
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"ok", func(*Config) {}, ""},
		{"missing api key", func(c *Config) { c.AlpacaAPIKey = "" }, "ALPACA_API_KEY"},
		{"missing api secret", func(c *Config) { c.AlpacaAPISecret = "" }, "ALPACA_API_SECRET"},
		{"missing feedback url", func(c *Config) { c.FeedbackURL = "" }, "CHARTNAGARI_FEEDBACK_URL"},
		{"missing plugin secret", func(c *Config) { c.PluginSecret = "" }, "CHARTNAGARI_PLUGIN_SECRET"},
		{"zero notional", func(c *Config) { c.NotionalPerTrade = 0 }, "NOTIONAL"},
		{"live url rejected", func(c *Config) { c.AlpacaAPIURL = "https://live-api.alpaca.markets" }, "non-paper host"},
		{"bad scheme", func(c *Config) { c.AlpacaAPIURL = "ftp://paper-api.alpaca.markets" }, "http(s)"},
		{"localhost allowed for tests", func(c *Config) { c.AlpacaAPIURL = "http://127.0.0.1:1234" }, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := base
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestLoadConfigFromEnv_Defaults(t *testing.T) {
	// Not parallel — manipulates os env.
	t.Setenv("ALPACA_API_KEY", "k")
	t.Setenv("ALPACA_API_SECRET", "s")
	t.Setenv("CHARTNAGARI_FEEDBACK_URL", "http://127.0.0.1:8080/api/execution/feedback")
	t.Setenv("CHARTNAGARI_PLUGIN_SECRET", "shh")
	t.Setenv("ALPACA_API_URL", "")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("ALPACA_DB_PATH", "")
	t.Setenv("CHARTNAGARI_PLUGIN_ID", "")
	t.Setenv("ALPACA_NOTIONAL_PER_TRADE", "")
	t.Setenv("ALPACA_TIMESTAMP_SKEW_SEC", "")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.AlpacaAPIURL != "https://paper-api.alpaca.markets" {
		t.Errorf("default alpaca url = %q", cfg.AlpacaAPIURL)
	}
	if cfg.ListenAddr != ":9100" {
		t.Errorf("default listen addr = %q", cfg.ListenAddr)
	}
	if cfg.DBPath != "./plugin-alpaca.db" {
		t.Errorf("default db path = %q", cfg.DBPath)
	}
	if cfg.PluginID != "alpaca-paper" {
		t.Errorf("default plugin id = %q", cfg.PluginID)
	}
	if cfg.NotionalPerTrade != 1000 {
		t.Errorf("default notional = %v", cfg.NotionalPerTrade)
	}
	if cfg.TimestampSkewSec != 300 {
		t.Errorf("default skew = %v", cfg.TimestampSkewSec)
	}
}

func TestLoadConfigFromEnv_MissingRequired(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "")
	t.Setenv("ALPACA_API_SECRET", "s")
	t.Setenv("CHARTNAGARI_FEEDBACK_URL", "http://x")
	t.Setenv("CHARTNAGARI_PLUGIN_SECRET", "shh")
	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatal("expected error for missing ALPACA_API_KEY")
	}
}

func TestIsTestHost(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"localhost", "127.0.0.1", "127.0.0.1:8080", "::1", "[::1]:8080", "127.10.20.30"} {
		if !isTestHost(host) {
			t.Errorf("isTestHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"paper-api.alpaca.markets", "example.com"} {
		if isTestHost(host) {
			t.Errorf("isTestHost(%q) = true, want false", host)
		}
	}
}

func TestParseFloatEnv_Fallbacks(t *testing.T) {
	// Bad value → default.
	t.Setenv("X_FLOAT", "not-a-number")
	if v := parseFloatEnv("X_FLOAT", 9.5); v != 9.5 {
		t.Errorf("bad float fallback = %v", v)
	}
	// Empty → default.
	t.Setenv("X_FLOAT", "")
	if v := parseFloatEnv("X_FLOAT", 9.5); v != 9.5 {
		t.Errorf("empty float fallback = %v", v)
	}
	// Good value → parsed.
	t.Setenv("X_FLOAT", "42.25")
	if v := parseFloatEnv("X_FLOAT", 9.5); v != 42.25 {
		t.Errorf("parsed float = %v", v)
	}
}

func TestParseIntEnv_Fallbacks(t *testing.T) {
	t.Setenv("X_INT", "abc")
	if v := parseIntEnv("X_INT", 7); v != 7 {
		t.Errorf("bad int fallback = %d", v)
	}
	t.Setenv("X_INT", "")
	if v := parseIntEnv("X_INT", 7); v != 7 {
		t.Errorf("empty int fallback = %d", v)
	}
	t.Setenv("X_INT", "99")
	if v := parseIntEnv("X_INT", 7); v != 99 {
		t.Errorf("parsed int = %d", v)
	}
}
