// Package alpaca implements a standalone execution plugin adapter that
// translates ChartNagari TradeSignal webhooks into Alpaca paper-trading orders
// and reports lifecycle events back via the /api/execution/feedback callback.
//
// Phase 3 scope:
//   - Receive HMAC-signed TradeSignal POSTs on /webhook (same canonical string
//     format as internal/execution/hmac.go; we import that package directly).
//   - Enforce paper-only hard guard: refuse to start if ALPACA_API_URL is not
//     the Alpaca paper endpoint.
//   - Map TradeSignal.Direction → Alpaca order side:
//       LONG  → buy (market)
//       SHORT → sell (market; closes existing long position only in Phase 3)
//   - Idempotency on signal_id stored in a tiny SQLite file so restarts do not
//     re-submit an order for a signal that was already processed.
//   - POST OrderFeedback back to ChartNagari, HMAC-signed with the same shared
//     secret used for inbound verification.
package alpaca

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the runtime configuration resolved from environment variables. The
// adapter is a single-binary sidecar, so env vars are the simplest deployment
// surface — no YAML parser, no file discovery, no hot reload. Every field has
// a sensible default except the four authentication/URL primitives.
type Config struct {
	// Alpaca endpoint. Paper-only. Anything that is not the Alpaca paper host
	// is rejected at startup (hard guard).
	AlpacaAPIURL    string
	AlpacaAPIKey    string
	AlpacaAPISecret string

	// ChartNagari feedback sink. Must be the full URL including path, e.g.
	// "http://127.0.0.1:8080/api/execution/feedback".
	FeedbackURL string

	// Shared HMAC secret. Must match the plugin.secret configured in
	// config/execution.yaml on the ChartNagari side. Used for BOTH directions:
	//   - verifying inbound TradeSignal POSTs
	//   - signing outbound OrderFeedback POSTs
	PluginSecret string

	// Plugin ID — must match plugin.id in config/execution.yaml. Carried in the
	// X-ChartNagari-Plugin-Id header for outbound feedback.
	PluginID string

	// HTTP listener for the /webhook endpoint. Defaults to ":9100".
	ListenAddr string

	// Notional-per-trade in USD. Qty is computed as floor(Notional / entry_price).
	// Must be > 0.
	NotionalPerTrade float64

	// SQLite path for idempotency. Defaults to "./plugin-alpaca.db".
	DBPath string

	// Clock skew tolerance for inbound HMAC verification. Defaults to 300s.
	TimestampSkewSec int
}

// paperHosts is the allow-list of Alpaca paper endpoints. We deliberately do
// NOT accept live-api.alpaca.markets here — the binary refuses to start.
var paperHosts = map[string]struct{}{
	"paper-api.alpaca.markets": {},
}

// LoadConfigFromEnv resolves a Config from the process environment.
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		AlpacaAPIURL:     strings.TrimSpace(os.Getenv("ALPACA_API_URL")),
		AlpacaAPIKey:     os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret:  os.Getenv("ALPACA_API_SECRET"),
		FeedbackURL:      strings.TrimSpace(os.Getenv("CHARTNAGARI_FEEDBACK_URL")),
		PluginSecret:     os.Getenv("CHARTNAGARI_PLUGIN_SECRET"),
		PluginID:         strings.TrimSpace(os.Getenv("CHARTNAGARI_PLUGIN_ID")),
		ListenAddr:       strings.TrimSpace(os.Getenv("LISTEN_ADDR")),
		DBPath:           strings.TrimSpace(os.Getenv("ALPACA_DB_PATH")),
		NotionalPerTrade: parseFloatEnv("ALPACA_NOTIONAL_PER_TRADE", 1000.0),
		TimestampSkewSec: parseIntEnv("ALPACA_TIMESTAMP_SKEW_SEC", 300),
	}
	if cfg.AlpacaAPIURL == "" {
		cfg.AlpacaAPIURL = "https://paper-api.alpaca.markets"
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":9100"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "./plugin-alpaca.db"
	}
	if cfg.PluginID == "" {
		cfg.PluginID = "alpaca-paper"
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate enforces mandatory fields and the paper-only hard guard.
func (c Config) Validate() error {
	if c.AlpacaAPIKey == "" {
		return errors.New("ALPACA_API_KEY is required")
	}
	if c.AlpacaAPISecret == "" {
		return errors.New("ALPACA_API_SECRET is required")
	}
	if c.FeedbackURL == "" {
		return errors.New("CHARTNAGARI_FEEDBACK_URL is required")
	}
	if c.PluginSecret == "" {
		return errors.New("CHARTNAGARI_PLUGIN_SECRET is required")
	}
	if c.NotionalPerTrade <= 0 {
		return errors.New("ALPACA_NOTIONAL_PER_TRADE must be > 0")
	}
	// Paper-only hard guard — this is the single most important refusal.
	// Parse the URL; reject any host not in paperHosts (explicit allow-list).
	u, err := url.Parse(c.AlpacaAPIURL)
	if err != nil {
		return fmt.Errorf("invalid ALPACA_API_URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("ALPACA_API_URL must be http(s), got %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if _, ok := paperHosts[host]; !ok {
		// Permit localhost / 127.0.0.1 / any host beginning with "127." for tests
		// (httptest.Server) — production config must use paper-api.alpaca.markets.
		if !isTestHost(host) {
			return fmt.Errorf("refusing to start against non-paper host %q; ALPACA_API_URL must be https://paper-api.alpaca.markets", u.Host)
		}
	}
	// Feedback URL sanity check.
	if _, err := url.Parse(c.FeedbackURL); err != nil {
		return fmt.Errorf("invalid CHARTNAGARI_FEEDBACK_URL: %w", err)
	}
	return nil
}

// isTestHost returns true for loopback hosts used by unit/integration tests.
// Production must use paper-api.alpaca.markets.
func isTestHost(host string) bool {
	// Bracketed IPv6 literal: "[::1]" or "[::1]:8080".
	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end >= 0 {
			host = host[1:end]
		}
	} else if strings.Count(host, ":") == 1 {
		// Exactly one colon → "hostname:port" form. Strip the port.
		host = host[:strings.Index(host, ":")]
	}
	// After normalization, host is either a bare name, a bare IPv4, or a
	// bare IPv6 literal like "::1".
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasPrefix(host, "127.")
}

func parseFloatEnv(key string, def float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return v
}

func parseIntEnv(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

// HTTPTimeout is the default per-call timeout for outbound Alpaca / feedback
// HTTP. Kept short so a slow Alpaca API cannot back-pressure the webhook
// handler indefinitely.
const HTTPTimeout = 10 * time.Second
