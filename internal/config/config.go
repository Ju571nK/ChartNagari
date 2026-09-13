// Package config loads web-managed YAML configuration files.
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

	"gopkg.in/yaml.v3"
)

// Config is the top-level application configuration.
type Config struct {
	// StartupSettings is the YAML input actually read during this process startup.
	// It is not proof that an optional integration is connected or enabled.
	StartupSettings map[string]string

	Env        string
	ServerHost string // bind address; default "127.0.0.1"
	ServerPort string
	APIToken   string // optional bearer token for mutating API endpoints
	LogLevel   string
	DBPath     string

	Binance      BinanceConfig
	Yahoo        YahooConfig
	Tiingo       TiingoConfig
	AlphaVantage AlphaVantageConfig
	Finnhub      FinnhubConfig
	FMP          FMPConfig
	Telegram     TelegramConfig
	Discord      DiscordConfig
	Alert        AlertConfig
	Anthropic    AnthropicConfig
	OpenAI       OpenAIConfig
	Groq         GroqConfig
	Gemini       GeminiConfig
	Ollama       OllamaConfig
	LLMProvider  string // "anthropic" | "openai" | "groq" | "gemini" | "ollama"
	Language     string // "en" | "ko" | "ja" (default "en")

	Rules       RulesConfig
	Watchlist   WatchlistConfig
	DailyReport DailyReportConfig
}

// DailyReportConfig mirrors config/report.yaml structure.
type DailyReportConfig struct {
	Enabled       bool    `yaml:"enabled"`
	Time          string  `yaml:"time"`     // "HH:MM"
	Timezone      string  `yaml:"timezone"` // "Asia/Seoul"
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

type FinnhubConfig struct {
	APIKey             string
	AlertWindowMinutes int // minutes before event to send pre-alert (default 30)
}

type FMPConfig struct {
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
	ScoreThreshold  float64 `yaml:"score_threshold" json:"score_threshold"`
	CooldownHours   int     `yaml:"cooldown_hours" json:"cooldown_hours"`
	MTFConsensusMin int     `yaml:"mtf_consensus_min" json:"mtf_consensus_min"`
	CryptoTPMult    float64 `yaml:"crypto_tp_mult" json:"crypto_tp_mult"`
	CryptoSLMult    float64 `yaml:"crypto_sl_mult" json:"crypto_sl_mult"`
	StockTPMult     float64 `yaml:"stock_tp_mult" json:"stock_tp_mult"`
	StockSLMult     float64 `yaml:"stock_sl_mult" json:"stock_sl_mult"`
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

type OllamaConfig struct {
	Host    string
	Model   string
	Timeout time.Duration
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
		Crypto  []SymbolEntry `yaml:"crypto"`
		Stocks  []SymbolEntry `yaml:"stocks"`
		Indices []SymbolEntry `yaml:"indices"`
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
	Version int               `yaml:"version"`
	Clients map[string]string `yaml:"clients,omitempty"`
	Server  struct {
		Env            string `yaml:"env"`
		Host           string `yaml:"host"`
		Port           string `yaml:"port"`
		LogLevel       string `yaml:"log_level"`
		APIToken       string `yaml:"api_token"`
		RemoteAccess   string `yaml:"remote_access,omitempty"`
		AllowedOrigins string `yaml:"allowed_origins,omitempty"`
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
	Ollama struct {
		Host       string `yaml:"host"`
		Model      string `yaml:"model"`
		TimeoutSec int    `yaml:"timeout_sec"`
	} `yaml:"ollama"`
	AlphaVantage struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"alphavantage"`
	Finnhub struct {
		APIKey             string `yaml:"api_key"`
		AlertWindowMinutes int    `yaml:"alert_window_minutes"`
	} `yaml:"finnhub"`
	Fmp struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"fmp"`
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
	m := map[string]string{
		"DB_PATH":                s.Database.Path,
		"ENV":                    s.Server.Env,
		"SERVER_HOST":            s.Server.Host,
		"SERVER_PORT":            s.Server.Port,
		"LOG_LEVEL":              s.Server.LogLevel,
		"API_TOKEN":              s.Server.APIToken,
		"REMOTE_ACCESS":          s.Server.RemoteAccess,
		"REMOTE_ALLOWED_ORIGINS": s.Server.AllowedOrigins,
		"BINANCE_API_KEY":        s.Binance.APIKey,
		"BINANCE_SECRET_KEY":     s.Binance.SecretKey,
		"TIINGO_API_KEY":         s.Tiingo.APIKey,
		"TIINGO_POLL_INTERVAL":   itoa(s.Tiingo.PollInterval),
		"YAHOO_POLL_INTERVAL":    itoa(s.Yahoo.PollInterval),
		"TELEGRAM_BOT_TOKEN":     s.Telegram.BotToken,
		"TELEGRAM_CHAT_ID":       s.Telegram.ChatID,
		"DISCORD_WEBHOOK_URL":    s.Discord.WebhookURL,
		"ALERT_COOLDOWN_HOURS":   itoa(s.Alert.CooldownHours),
		"LLM_PROVIDER":           s.LLM.Provider,
		"LLM_LANGUAGE":           s.LLM.Language,
		"AI_MIN_SCORE":           ftoa(s.LLM.MinScore),
		"ANTHROPIC_API_KEY":      s.Anthropic.APIKey,
		"OPENAI_API_KEY":         s.OpenAI.APIKey,
		"GROQ_API_KEY":           s.Groq.APIKey,
		"GEMINI_API_KEY":         s.Gemini.APIKey,
		"OLLAMA_HOST":            s.Ollama.Host,
		"OLLAMA_MODEL":           s.Ollama.Model,
		"OLLAMA_TIMEOUT_SEC":     itoa(s.Ollama.TimeoutSec),
		"ALPHAVANTAGE_API_KEY":   s.AlphaVantage.APIKey,
		"FINNHUB_API_KEY":        s.Finnhub.APIKey,
		"CALENDAR_ALERT_WINDOW":  itoa(s.Finnhub.AlertWindowMinutes),
		"FMP_API_KEY":            s.Fmp.APIKey,
	}
	for key, fallback := range ClientDefaults {
		m[key] = fallback
		if value, ok := s.Clients[key]; ok {
			m[key] = value
		}
	}
	return m
}

// ApplyMap applies a flat env-key map onto the SettingsYAML struct.
func (s *SettingsYAML) ApplyMap(m map[string]string) {
	if s.Clients == nil {
		s.Clients = make(map[string]string)
	}
	for key := range ClientDefaults {
		if value, ok := m[key]; ok {
			s.Clients[key] = value
		}
	}
	set := func(dst *string, key string) {
		if v, ok := m[key]; ok {
			*dst = v
		}
	}
	setInt := func(dst *int, key string) {
		if v, ok := m[key]; ok {
			if v == "" {
				*dst = 0
				return
			}
			if i, err := strconv.Atoi(v); err == nil {
				*dst = i
			}
		}
	}
	setFloat := func(dst *float64, key string) {
		if v, ok := m[key]; ok {
			if v == "" {
				*dst = 0
				return
			}
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				*dst = f
			}
		}
	}

	set(&s.Server.Env, "ENV")
	set(&s.Database.Path, "DB_PATH")
	set(&s.Server.Host, "SERVER_HOST")
	set(&s.Server.Port, "SERVER_PORT")
	set(&s.Server.LogLevel, "LOG_LEVEL")
	set(&s.Server.APIToken, "API_TOKEN")
	set(&s.Server.RemoteAccess, "REMOTE_ACCESS")
	set(&s.Server.AllowedOrigins, "REMOTE_ALLOWED_ORIGINS")
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
	set(&s.Ollama.Host, "OLLAMA_HOST")
	set(&s.Ollama.Model, "OLLAMA_MODEL")
	setInt(&s.Ollama.TimeoutSec, "OLLAMA_TIMEOUT_SEC")
	set(&s.AlphaVantage.APIKey, "ALPHAVANTAGE_API_KEY")
	set(&s.Finnhub.APIKey, "FINNHUB_API_KEY")
	setInt(&s.Finnhub.AlertWindowMinutes, "CALENDAR_ALERT_WINDOW")
	set(&s.Fmp.APIKey, "FMP_API_KEY")
}

// LoadSettings reads settings.yaml over application defaults.
func LoadSettings(path string) (*SettingsYAML, error) {
	var s SettingsYAML
	s.ApplyMap(map[string]string{
		"ENV": "development", "SERVER_HOST": "127.0.0.1", "SERVER_PORT": "8080", "LOG_LEVEL": "debug",
		"DB_PATH": "./data/chart_analyzer.db", "TIINGO_POLL_INTERVAL": "900", "YAHOO_POLL_INTERVAL": "60",
		"ALERT_COOLDOWN_HOURS": "4", "AI_MIN_SCORE": "12", "LLM_LANGUAGE": "en",
		"OLLAMA_HOST": "http://localhost:11434", "OLLAMA_MODEL": "gemma4:4b", "OLLAMA_TIMEOUT_SEC": "120",
		"CALENDAR_ALERT_WINDOW": "30",
	})
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
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(s); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Load migrates legacy environment once, then reads YAML only.
// configDir is typically "config/" relative to the binary location.
func Load(envFile, configDir string) (*Config, error) {
	// Primary: load config/settings.yaml
	s, err := MigrateSettings(envFile, configDir+"/settings.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load settings.yaml: %w", err)
	}

	ollamaTimeout := getEnvOrDuration("OLLAMA_TIMEOUT_SEC", s.Ollama.TimeoutSec, 120, time.Second)
	if ollamaTimeout <= 0 {
		ollamaTimeout = 120 * time.Second
	}

	cfg := &Config{
		StartupSettings: s.ToMap(),

		Env:        getEnvOr("ENV", s.Server.Env, "development"),
		ServerHost: getEnvOr("SERVER_HOST", s.Server.Host, "127.0.0.1"),
		ServerPort: getEnvOr("SERVER_PORT", s.Server.Port, "8080"),
		APIToken:   getEnvOr("API_TOKEN", s.Server.APIToken, ""),
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
		Finnhub: FinnhubConfig{
			APIKey:             getEnvOr("FINNHUB_API_KEY", s.Finnhub.APIKey, ""),
			AlertWindowMinutes: getEnvOrInt("CALENDAR_ALERT_WINDOW", s.Finnhub.AlertWindowMinutes, 30),
		},
		FMP: FMPConfig{
			APIKey: getEnvOr("FMP_API_KEY", s.Fmp.APIKey, ""),
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
		Ollama: OllamaConfig{
			Host:    getEnvOr("OLLAMA_HOST", s.Ollama.Host, "http://localhost:11434"),
			Model:   getEnvOr("OLLAMA_MODEL", s.Ollama.Model, "gemma4:4b"),
			Timeout: ollamaTimeout,
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

	// Validate the economic-calendar alert window: clamp to [5, 1440] minutes.
	// Out-of-range values are pinned to the nearest bound so a typo can't disable
	// alerts (too small) or schedule them days ahead (too large).
	if cfg.Finnhub.AlertWindowMinutes < 5 {
		cfg.Finnhub.AlertWindowMinutes = 5
	} else if cfg.Finnhub.AlertWindowMinutes > 1440 {
		cfg.Finnhub.AlertWindowMinutes = 1440
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

// EnabledIndexSymbols returns only the enabled index symbols (e.g. ^VIX).
func (c *Config) EnabledIndexSymbols() []string {
	var out []string
	for _, s := range c.Watchlist.Symbols.Indices {
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

// Legacy helper names retained internally; resolution is YAML > fallback only.
func getEnvOr(key, yamlVal, fallback string) string {
	if yamlVal != "" {
		return yamlVal
	}
	return fallback
}

func getEnvOrInt(key string, yamlVal, fallback int) int {
	if yamlVal != 0 {
		return yamlVal
	}
	return fallback
}

func getEnvOrFloat(key string, yamlVal, fallback float64) float64 {
	if yamlVal != 0 {
		return yamlVal
	}
	return fallback
}

func getEnvOrDuration(key string, yamlValSecs, fallbackSecs int, unit time.Duration) time.Duration {
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
