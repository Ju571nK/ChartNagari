// Package config loads environment variables and YAML configuration files.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
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

	Binance      BinanceConfig
	Yahoo        YahooConfig
	Tiingo       TiingoConfig
	AlphaVantage AlphaVantageConfig
	Telegram     TelegramConfig
	Discord      DiscordConfig
	Alert        AlertConfig
	Anthropic    AnthropicConfig
	OpenAI       OpenAIConfig
	Groq         GroqConfig
	Gemini       GeminiConfig
	LLMProvider  string // "anthropic" | "openai" | "groq" | "gemini"
	Language     string // "en" | "ko" | "ja" (default "en")

	Rules       RulesConfig
	Watchlist   WatchlistConfig
	DailyReport DailyReportConfig
}

// DailyReportConfig mirrors config/report.yaml structure.
type DailyReportConfig struct {
	Enabled       bool    `yaml:"enabled"`
	Time          string  `yaml:"time"`          // "HH:MM"
	Timezone      string  `yaml:"timezone"`      // "Asia/Seoul"
	AIMinScore    float64 `yaml:"ai_min_score"`
	OnlyIfSignals bool    `yaml:"only_if_signals"`
	Compact       bool    `yaml:"compact"`
}

type BinanceConfig struct {
	APIKey    string
	SecretKey string
}

type YahooConfig struct {
	PollInterval time.Duration
}

type TiingoConfig struct {
	APIKey       string
	PollInterval time.Duration
}

type AlphaVantageConfig struct {
	APIKey string
}

type TelegramConfig struct {
	BotToken string
	ChatID   string
}

type DiscordConfig struct {
	WebhookURL string
}

type AlertConfig struct {
	ScoreThreshold  float64 `yaml:"score_threshold"`
	CooldownHours   int     `yaml:"cooldown_hours"`
	MTFConsensusMin int     `yaml:"mtf_consensus_min"`
	CryptoTPMult    float64 `yaml:"crypto_tp_mult"`
	CryptoSLMult    float64 `yaml:"crypto_sl_mult"`
	StockTPMult     float64 `yaml:"stock_tp_mult"`
	StockSLMult     float64 `yaml:"stock_sl_mult"`
}

// AlertConfigHolder is a mutex-protected holder for live-updated AlertConfig.
type AlertConfigHolder struct {
	mu  sync.RWMutex
	cfg AlertConfig
}

func NewAlertConfigHolder(cfg AlertConfig) *AlertConfigHolder {
	return &AlertConfigHolder{cfg: cfg}
}

func (h *AlertConfigHolder) Get() AlertConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

func (h *AlertConfigHolder) Set(cfg AlertConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = cfg
}

type AnthropicConfig struct {
	APIKey   string
	MinScore float64 // minimum total signal score to trigger AI interpretation
}

type OpenAIConfig struct {
	APIKey string
}

type GroqConfig struct {
	APIKey string
}

type GeminiConfig struct {
	APIKey string
}

// RulesConfig mirrors config/rules.yaml structure.
type RulesConfig struct {
	Rules            []RuleEntry        `yaml:"rules"`
	Scoring          ScoringConfig      `yaml:"scoring"`
	TimeframeWeights map[string]float64 `yaml:"timeframe_weights"`
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

// SettingsYAML is the structure of config/settings.yaml.
// It stores all secrets and runtime settings that were previously in .env.
type SettingsYAML struct {
	Server struct {
		Env      string `yaml:"env"`
		Port     string `yaml:"port"`
		LogLevel string `yaml:"log_level"`
	} `yaml:"server"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	Binance struct {
		APIKey    string `yaml:"api_key"`
		SecretKey string `yaml:"secret_key"`
	} `yaml:"binance"`
	Tiingo struct {
		APIKey       string `yaml:"api_key"`
		PollInterval int    `yaml:"poll_interval"`
	} `yaml:"tiingo"`
	Yahoo struct {
		PollInterval int `yaml:"poll_interval"`
	} `yaml:"yahoo"`
	Telegram struct {
		BotToken string `yaml:"bot_token"`
		ChatID   string `yaml:"chat_id"`
	} `yaml:"telegram"`
	Discord struct {
		WebhookURL string `yaml:"webhook_url"`
	} `yaml:"discord"`
	Alert struct {
		CooldownHours int `yaml:"cooldown_hours"`
	} `yaml:"alert"`
	LLM struct {
		Provider string  `yaml:"provider"`
		Language string  `yaml:"language"`
		MinScore float64 `yaml:"min_score"`
	} `yaml:"llm"`
	Anthropic struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"anthropic"`
	OpenAI struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"openai"`
	Groq struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"groq"`
	Gemini struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"gemini"`
	AlphaVantage struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"alphavantage"`
}

// ToMap converts SettingsYAML to the flat env-key map used by the API.
func (s *SettingsYAML) ToMap() map[string]string {
	itoa := func(i int) string {
		if i == 0 {
			return ""
		}
		return strconv.Itoa(i)
	}
	ftoa := func(f float64) string {
		if f == 0 {
			return ""
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return map[string]string{
		"ENV":                  s.Server.Env,
		"SERVER_PORT":          s.Server.Port,
		"LOG_LEVEL":            s.Server.LogLevel,
		"BINANCE_API_KEY":      s.Binance.APIKey,
		"BINANCE_SECRET_KEY":   s.Binance.SecretKey,
		"TIINGO_API_KEY":       s.Tiingo.APIKey,
		"TIINGO_POLL_INTERVAL": itoa(s.Tiingo.PollInterval),
		"YAHOO_POLL_INTERVAL":  itoa(s.Yahoo.PollInterval),
		"TELEGRAM_BOT_TOKEN":   s.Telegram.BotToken,
		"TELEGRAM_CHAT_ID":     s.Telegram.ChatID,
		"DISCORD_WEBHOOK_URL":  s.Discord.WebhookURL,
		"ALERT_COOLDOWN_HOURS": itoa(s.Alert.CooldownHours),
		"LLM_PROVIDER":         s.LLM.Provider,
		"LLM_LANGUAGE":         s.LLM.Language,
		"AI_MIN_SCORE":         ftoa(s.LLM.MinScore),
		"ANTHROPIC_API_KEY":    s.Anthropic.APIKey,
		"OPENAI_API_KEY":       s.OpenAI.APIKey,
		"GROQ_API_KEY":         s.Groq.APIKey,
		"GEMINI_API_KEY":       s.Gemini.APIKey,
		"ALPHAVANTAGE_API_KEY": s.AlphaVantage.APIKey,
	}
}

// ApplyMap applies a flat env-key map onto the SettingsYAML struct.
func (s *SettingsYAML) ApplyMap(m map[string]string) {
	set := func(dst *string, key string) {
		if v, ok := m[key]; ok {
			*dst = v
		}
	}
	setInt := func(dst *int, key string) {
		if v, ok := m[key]; ok {
			if i, err := strconv.Atoi(v); err == nil {
				*dst = i
			}
		}
	}
	setFloat := func(dst *float64, key string) {
		if v, ok := m[key]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				*dst = f
			}
		}
	}

	set(&s.Server.Env, "ENV")
	set(&s.Server.Port, "SERVER_PORT")
	set(&s.Server.LogLevel, "LOG_LEVEL")
	set(&s.Binance.APIKey, "BINANCE_API_KEY")
	set(&s.Binance.SecretKey, "BINANCE_SECRET_KEY")
	set(&s.Tiingo.APIKey, "TIINGO_API_KEY")
	setInt(&s.Tiingo.PollInterval, "TIINGO_POLL_INTERVAL")
	setInt(&s.Yahoo.PollInterval, "YAHOO_POLL_INTERVAL")
	set(&s.Telegram.BotToken, "TELEGRAM_BOT_TOKEN")
	set(&s.Telegram.ChatID, "TELEGRAM_CHAT_ID")
	set(&s.Discord.WebhookURL, "DISCORD_WEBHOOK_URL")
	setInt(&s.Alert.CooldownHours, "ALERT_COOLDOWN_HOURS")
	set(&s.LLM.Provider, "LLM_PROVIDER")
	set(&s.LLM.Language, "LLM_LANGUAGE")
	setFloat(&s.LLM.MinScore, "AI_MIN_SCORE")
	set(&s.Anthropic.APIKey, "ANTHROPIC_API_KEY")
	set(&s.OpenAI.APIKey, "OPENAI_API_KEY")
	set(&s.Groq.APIKey, "GROQ_API_KEY")
	set(&s.Gemini.APIKey, "GEMINI_API_KEY")
	set(&s.AlphaVantage.APIKey, "ALPHAVANTAGE_API_KEY")
}

// LoadSettings reads settings.yaml. Returns an empty struct (no error) if the file is absent or empty.
func LoadSettings(path string) (*SettingsYAML, error) {
	var s SettingsYAML
	if err := loadYAML(path, &s); err != nil {
		if os.IsNotExist(err) || errors.Is(err, io.EOF) {
			return &s, nil
		}
		return nil, err
	}
	return &s, nil
}

// SaveSettings writes SettingsYAML to the given path, creating parent directories if needed.
func SaveSettings(path string, s *SettingsYAML) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	return enc.Encode(s)
}

// Load reads settings.yaml (primary), falls back to .env (legacy), then OS env vars.
// configDir is typically "config/" relative to the binary location.
func Load(envFile, configDir string) (*Config, error) {
	// Primary: load config/settings.yaml
	s, err := LoadSettings(configDir + "/settings.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load settings.yaml: %w", err)
	}

	// Legacy fallback: load .env (sets env vars only if not already set by OS)
	_ = godotenv.Load(envFile)

	cfg := &Config{
		Env:        getEnvOr("ENV", s.Server.Env, "development"),
		ServerPort: getEnvOr("SERVER_PORT", s.Server.Port, "8080"),
		LogLevel:   getEnvOr("LOG_LEVEL", s.Server.LogLevel, "debug"),
		DBPath:     getEnvOr("DB_PATH", s.Database.Path, "./data/chart_analyzer.db"),
		Binance: BinanceConfig{
			APIKey:    getEnvOr("BINANCE_API_KEY", s.Binance.APIKey, ""),
			SecretKey: getEnvOr("BINANCE_SECRET_KEY", s.Binance.SecretKey, ""),
		},
		Yahoo: YahooConfig{
			PollInterval: getEnvOrDuration("YAHOO_POLL_INTERVAL", s.Yahoo.PollInterval, 60, time.Second),
		},
		Tiingo: TiingoConfig{
			APIKey:       getEnvOr("TIINGO_API_KEY", s.Tiingo.APIKey, ""),
			PollInterval: getEnvOrDuration("TIINGO_POLL_INTERVAL", s.Tiingo.PollInterval, 900, time.Second),
		},
		AlphaVantage: AlphaVantageConfig{
			APIKey: getEnvOr("ALPHAVANTAGE_API_KEY", s.AlphaVantage.APIKey, ""),
		},
		Telegram: TelegramConfig{
			BotToken: getEnvOr("TELEGRAM_BOT_TOKEN", s.Telegram.BotToken, ""),
			ChatID:   getEnvOr("TELEGRAM_CHAT_ID", s.Telegram.ChatID, ""),
		},
		Discord: DiscordConfig{
			WebhookURL: getEnvOr("DISCORD_WEBHOOK_URL", s.Discord.WebhookURL, ""),
		},
		Alert: AlertConfig{
			ScoreThreshold:  12.0,
			CooldownHours:   getEnvOrInt("ALERT_COOLDOWN_HOURS", s.Alert.CooldownHours, 4),
			MTFConsensusMin: 2,
			CryptoTPMult:    1.5,
			CryptoSLMult:    0.75,
			StockTPMult:     2.0,
			StockSLMult:     1.0,
		},
		Anthropic: AnthropicConfig{
			APIKey:   getEnvOr("ANTHROPIC_API_KEY", s.Anthropic.APIKey, ""),
			MinScore: getEnvOrFloat("AI_MIN_SCORE", s.LLM.MinScore, 12.0),
		},
		OpenAI: OpenAIConfig{
			APIKey: getEnvOr("OPENAI_API_KEY", s.OpenAI.APIKey, ""),
		},
		Groq: GroqConfig{
			APIKey: getEnvOr("GROQ_API_KEY", s.Groq.APIKey, ""),
		},
		Gemini: GeminiConfig{
			APIKey: getEnvOr("GEMINI_API_KEY", s.Gemini.APIKey, ""),
		},
		LLMProvider: getEnvOr("LLM_PROVIDER", s.LLM.Provider, ""),
		Language:    getEnvOr("LLM_LANGUAGE", s.LLM.Language, "en"),
	}

	// Load rules.yaml
	if err := loadYAML(configDir+"/rules.yaml", &cfg.Rules); err != nil {
		return nil, fmt.Errorf("failed to load rules.yaml: %w", err)
	}

	// Load watchlist.yaml
	if err := loadYAML(configDir+"/watchlist.yaml", &cfg.Watchlist); err != nil {
		return nil, fmt.Errorf("failed to load watchlist.yaml: %w", err)
	}

	// Load report.yaml — use defaults if absent
	cfg.DailyReport = DailyReportConfig{
		Enabled:    true,
		Time:       "09:00",
		Timezone:   "Asia/Seoul",
		AIMinScore: 8.0,
	}
	if err := loadYAML(configDir+"/report.yaml", &cfg.DailyReport); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load report.yaml: %w", err)
	}

	// Load alert.yaml — keep defaults if absent
	alertCfgPath := configDir + "/alert.yaml"
	if _, err := os.Stat(alertCfgPath); err == nil {
		if err := loadYAML(alertCfgPath, &cfg.Alert); err != nil {
			return nil, fmt.Errorf("failed to load alert.yaml: %w", err)
		}
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

// getEnvOr returns: OS env var > yamlVal > fallback.
func getEnvOr(key, yamlVal, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if yamlVal != "" {
		return yamlVal
	}
	return fallback
}

func getEnvOrInt(key string, yamlVal, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	if yamlVal != 0 {
		return yamlVal
	}
	return fallback
}

func getEnvOrFloat(key string, yamlVal, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	if yamlVal != 0 {
		return yamlVal
	}
	return fallback
}

func getEnvOrDuration(key string, yamlValSecs, fallbackSecs int, unit time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return time.Duration(i) * unit
		}
	}
	if yamlValSecs != 0 {
		return time.Duration(yamlValSecs) * unit
	}
	return time.Duration(fallbackSecs) * unit
}

// Kept for internal use in Load; superseded by getEnvOr for new code.
func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
