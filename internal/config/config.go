// Package config loads environment variables and YAML configuration files.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Config is the top-level application configuration.
type Config struct {
	Env        string
	ServerPort string
	LogLevel   string
	DBPath     string

	Binance  BinanceConfig
	Yahoo    YahooConfig
	Telegram TelegramConfig
	Discord  DiscordConfig
	Alert    AlertConfig

	Rules     RulesConfig
	Watchlist WatchlistConfig
}

type BinanceConfig struct {
	APIKey    string
	SecretKey string
}

type YahooConfig struct {
	PollInterval time.Duration
}

type TelegramConfig struct {
	BotToken string
	ChatID   string
}

type DiscordConfig struct {
	WebhookURL string
}

type AlertConfig struct {
	CooldownHours int
}

// RulesConfig mirrors config/rules.yaml structure.
type RulesConfig struct {
	Rules             []RuleEntry           `yaml:"rules"`
	Scoring           ScoringConfig         `yaml:"scoring"`
	TimeframeWeights  map[string]float64    `yaml:"timeframe_weights"`
}

type RuleEntry struct {
	Name        string                 `yaml:"name"`
	Enabled     bool                   `yaml:"enabled"`
	Methodology string                 `yaml:"methodology"`
	Params      map[string]interface{} `yaml:"params"`
}

type ScoringConfig struct {
	MTFBonus   float64            `yaml:"mtf_bonus"`
	Thresholds map[string]float64 `yaml:"thresholds"`
}

// WatchlistConfig mirrors config/watchlist.yaml structure.
type WatchlistConfig struct {
	Symbols struct {
		Crypto []SymbolEntry `yaml:"crypto"`
		Stocks []SymbolEntry `yaml:"stocks"`
	} `yaml:"symbols"`
	Timeframes []string `yaml:"timeframes"`
}

type SymbolEntry struct {
	Symbol   string `yaml:"symbol"`
	Exchange string `yaml:"exchange"`
	Enabled  bool   `yaml:"enabled"`
}

// Load reads .env and YAML config files from configDir.
// configDir is typically "config/" relative to the binary location.
func Load(envFile, configDir string) (*Config, error) {
	// .env 파일 로드 (없어도 무시 — 환경변수로 대체 가능)
	_ = godotenv.Load(envFile)

	cfg := &Config{
		Env:        getEnv("ENV", "development"),
		ServerPort: getEnv("SERVER_PORT", "8080"),
		LogLevel:   getEnv("LOG_LEVEL", "debug"),
		DBPath:     getEnv("DB_PATH", "./data/chart_analyzer.db"),
		Binance: BinanceConfig{
			APIKey:    getEnv("BINANCE_API_KEY", ""),
			SecretKey: getEnv("BINANCE_SECRET_KEY", ""),
		},
		Yahoo: YahooConfig{
			PollInterval: parseDuration(getEnv("YAHOO_POLL_INTERVAL", "60"), time.Second),
		},
		Telegram: TelegramConfig{
			BotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
			ChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
		},
		Discord: DiscordConfig{
			WebhookURL: getEnv("DISCORD_WEBHOOK_URL", ""),
		},
		Alert: AlertConfig{
			CooldownHours: parseInt(getEnv("ALERT_COOLDOWN_HOURS", "4")),
		},
	}

	// rules.yaml 로드
	if err := loadYAML(configDir+"/rules.yaml", &cfg.Rules); err != nil {
		return nil, fmt.Errorf("rules.yaml 로드 실패: %w", err)
	}

	// watchlist.yaml 로드
	if err := loadYAML(configDir+"/watchlist.yaml", &cfg.Watchlist); err != nil {
		return nil, fmt.Errorf("watchlist.yaml 로드 실패: %w", err)
	}

	return cfg, nil
}

// EnabledCryptoSymbols returns only the enabled crypto symbols.
func (c *Config) EnabledCryptoSymbols() []string {
	var out []string
	for _, s := range c.Watchlist.Symbols.Crypto {
		if s.Enabled {
			out = append(out, s.Symbol)
		}
	}
	return out
}

// EnabledStockSymbols returns only the enabled stock symbols.
func (c *Config) EnabledStockSymbols() []string {
	var out []string
	for _, s := range c.Watchlist.Symbols.Stocks {
		if s.Enabled {
			out = append(out, s.Symbol)
		}
	}
	return out
}

func loadYAML(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return yaml.NewDecoder(f).Decode(v)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseDuration(s string, unit time.Duration) time.Duration {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 60 * unit
	}
	return time.Duration(v) * unit
}
