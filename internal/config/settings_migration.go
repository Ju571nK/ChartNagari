package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Standalone clients share this file, but are never started by saving it.
var ClientDefaults = map[string]string{
	"CHARTNAGARI_URL": "http://localhost:8080", "CHARTNAGARI_TOKEN": "",
	"ALPACA_API_URL": "https://paper-api.alpaca.markets", "ALPACA_API_KEY": "", "ALPACA_API_SECRET": "",
	"CHARTNAGARI_FEEDBACK_URL": "", "CHARTNAGARI_PLUGIN_SECRET": "", "CHARTNAGARI_PLUGIN_ID": "alpaca-paper",
	"LISTEN_ADDR": ":9100", "ALPACA_DB_PATH": "./plugin-alpaca.db",
	"ALPACA_NOTIONAL_PER_TRADE": "1000", "ALPACA_TIMESTAMP_SKEW_SEC": "300",
}

// MigrateSettings imports the old effective configuration once without changing
// process environment. Thereafter even blank YAML secrets override stale .env.
func MigrateSettings(envFile, path string) (*SettingsYAML, error) {
	s, err := LoadSettings(path)
	if err != nil {
		return nil, err
	}
	if s.Version >= 1 {
		return s, nil
	}
	legacy := map[string]string{}
	if envFile != "" {
		legacy, err = godotenv.Read(envFile)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot parse legacy env file")
		}
	}
	updates := map[string]string{}
	for key := range s.ToMap() {
		if value := legacy[key]; value != "" {
			updates[key] = value
		}
		if value := os.Getenv(key); value != "" {
			updates[key] = value
		}
	}
	s.ApplyMap(updates)
	s.Version = 1
	if err := SaveSettings(path, s); err != nil {
		return nil, err
	}
	return s, nil
}

// ValidateSettingsUpdates rejects malformed input before any file is changed.
// Errors name fields only; never echo credentials or credential-bearing URLs.
func ValidateSettingsUpdates(m map[string]string) error {
	allowed := (&SettingsYAML{}).ToMap()
	ranges := map[string][2]float64{
		"SERVER_PORT": {1, 65535}, "TIINGO_POLL_INTERVAL": {1, 86400}, "YAHOO_POLL_INTERVAL": {1, 86400},
		"OLLAMA_TIMEOUT_SEC": {1, 3600}, "CALENDAR_ALERT_WINDOW": {5, 1440},
		"ALERT_COOLDOWN_HOURS": {1, 8760}, "AI_MIN_SCORE": {0.01, 1000},
		"ALPACA_NOTIONAL_PER_TRADE": {0.01, 100000000}, "ALPACA_TIMESTAMP_SKEW_SEC": {1, 3600},
	}
	for key, value := range m {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown setting: %s", key)
		}
		if len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("invalid %s", key)
		}
		enums := map[string]string{
			"ENV": "development|production", "LOG_LEVEL": "debug|info|warn|error",
			"LLM_PROVIDER": "anthropic|openai|groq|gemini|ollama", "LLM_LANGUAGE": "en|ko|ja",
		}
		if choices, ok := enums[key]; ok && value != "" && !slices.Contains(strings.Split(choices, "|"), value) {
			return fmt.Errorf("invalid %s", key)
		}
		if value != "" && (key == "OLLAMA_HOST" || key == "CHARTNAGARI_URL" || key == "CHARTNAGARI_FEEDBACK_URL" || key == "ALPACA_API_URL" || key == "DISCORD_WEBHOOK_URL") {
			u, err := url.Parse(value)
			if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
				return fmt.Errorf("%s must be an HTTP(S) URL without embedded credentials", key)
			}
			if key == "ALPACA_API_URL" && (u.Scheme != "https" || u.Host != "paper-api.alpaca.markets" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "") {
				return fmt.Errorf("ALPACA_API_URL must use the paper endpoint")
			}
		}
		if limits, ok := ranges[key]; ok && value != "" {
			n, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < limits[0] || n > limits[1] {
				return fmt.Errorf("%s is outside its allowed range", key)
			}
			if key != "AI_MIN_SCORE" && key != "ALPACA_NOTIONAL_PER_TRADE" && n != math.Trunc(n) {
				return fmt.Errorf("%s must be an integer", key)
			}
		}
	}
	return nil
}
