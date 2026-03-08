package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/api"
	"github.com/Ju571nK/Chatter/internal/collector"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/interpreter"
	general_ta "github.com/Ju571nK/Chatter/internal/methodology/general_ta"
	"github.com/Ju571nK/Chatter/internal/methodology/ict"
	"github.com/Ju571nK/Chatter/internal/methodology/smc"
	"github.com/Ju571nK/Chatter/internal/methodology/wyckoff"
	"github.com/Ju571nK/Chatter/internal/notifier"
	"github.com/Ju571nK/Chatter/internal/pipeline"
	"github.com/Ju571nK/Chatter/internal/storage"
)

func main() {
	// ── 설정 로드 ────────────────────────────────────────────────────
	cfg, err := appconfig.Load(".env", "config")
	if err != nil {
		log.Fatal().Err(err).Msg("설정 로드 실패")
	}

	// ── 로거 초기화 ──────────────────────────────────────────────────
	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	zerolog.SetGlobalLevel(level)
	if cfg.Env == "production" {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	}

	log.Info().
		Str("version", "0.8.0").
		Str("env", cfg.Env).
		Msg("Chart Analyzer 서버 시작")

	// ── SQLite 초기화 ────────────────────────────────────────────────
	db, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", cfg.DBPath).Msg("DB 초기화 실패")
	}
	defer db.Close()
	log.Info().Str("path", cfg.DBPath).Msg("SQLite 연결 완료")

	// ── 컨텍스트 (SIGINT/SIGTERM 감지) ───────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── 수집기 시작 (goroutine) ──────────────────────────────────────
	timeframes := cfg.Watchlist.Timeframes

	cryptoSymbols := cfg.EnabledCryptoSymbols()
	if len(cryptoSymbols) > 0 {
		binance := collector.NewBinanceCollector(db, cryptoSymbols, timeframes)
		go binance.Start(ctx)
		log.Info().Strs("symbols", cryptoSymbols).Msg("Binance 수집기 실행 중")
	} else {
		log.Warn().Msg("활성화된 코인 종목 없음 — watchlist.yaml 확인")
	}

	stockSymbols := cfg.EnabledStockSymbols()
	if len(stockSymbols) > 0 {
		yahoo := collector.NewYahooCollector(db, stockSymbols, timeframes, cfg.Yahoo.PollInterval)
		go yahoo.Start(ctx)
		log.Info().Strs("symbols", stockSymbols).Msg("Yahoo Finance 수집기 실행 중")
	} else {
		log.Info().Msg("활성화된 주식 종목 없음 — watchlist.yaml에서 enabled: true 설정 필요")
	}

	// ── 룰 엔진 구성 ─────────────────────────────────────────────────
	eng := engine.New(toEngineConfig(cfg.Rules))
	// General TA
	eng.Register(&general_ta.RSIOverboughtOversoldRule{})
	eng.Register(&general_ta.RSIDivergenceRule{})
	eng.Register(&general_ta.EMACrossRule{})
	eng.Register(&general_ta.SupportResistanceBreakoutRule{})
	eng.Register(&general_ta.FibonacciConfluenceRule{})
	eng.Register(&general_ta.VolumeSpikeRule{})
	// ICT
	eng.Register(&ict.ICTOrderBlockRule{})
	eng.Register(&ict.ICTFairValueGapRule{})
	eng.Register(&ict.ICTLiquiditySweepRule{})
	eng.Register(&ict.ICTBreakerBlockRule{})
	eng.Register(ict.NewICTKillZoneRule())
	// Wyckoff
	eng.Register(&wyckoff.WyckoffAccumulationRule{})
	eng.Register(&wyckoff.WyckoffDistributionRule{})
	eng.Register(&wyckoff.WyckoffSpringRule{})
	eng.Register(&wyckoff.WyckoffUpthrustRule{})
	eng.Register(&wyckoff.WyckoffVolumeAnomalyRule{})
	// SMC
	eng.Register(&smc.SMCBOSRule{})
	eng.Register(&smc.SMCChoCHRule{})

	// ── 알림 시스템 ───────────────────────────────────────────────────
	notifCfg := notifier.Config{
		ScoreThreshold: cfg.Rules.Scoring.Thresholds["strong"],
		CooldownDur:    time.Duration(cfg.Alert.CooldownHours) * time.Hour,
	}
	if notifCfg.ScoreThreshold == 0 {
		notifCfg.ScoreThreshold = 12.0
	}
	if notifCfg.CooldownDur == 0 {
		notifCfg.CooldownDur = 4 * time.Hour
	}
	notif := notifier.New(notifCfg, log.Logger)

	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
		notif.Register(notifier.NewTelegramSender(cfg.Telegram.BotToken, cfg.Telegram.ChatID))
		log.Info().Msg("Telegram 알림 활성화")
	}
	if cfg.Discord.WebhookURL != "" {
		notif.Register(notifier.NewDiscordSender(cfg.Discord.WebhookURL))
		log.Info().Msg("Discord 알림 활성화")
	}

	// ── AI 해석 레이어 ────────────────────────────────────────────────
	interp := interpreter.New(cfg.Anthropic.APIKey, cfg.Anthropic.MinScore)
	if cfg.Anthropic.APIKey != "" {
		log.Info().Float64("min_score", cfg.Anthropic.MinScore).Msg("Claude AI 해석 활성화")
	} else {
		log.Info().Msg("Claude AI 해석 비활성화 (ANTHROPIC_API_KEY 미설정)")
	}

	// ── 분석 파이프라인 시작 ──────────────────────────────────────────
	allSymbols := append(cryptoSymbols, stockSymbols...)
	if len(allSymbols) > 0 {
		pipe := pipeline.New(
			pipeline.DefaultConfig(),
			db,
			eng,
			interp,
			notif,
			allSymbols,
			timeframes,
			log.Logger,
		)
		go pipe.Run(ctx)
		log.Info().
			Strs("symbols", allSymbols).
			Dur("interval", pipeline.DefaultConfig().Interval).
			Msg("분석 파이프라인 실행 중")
	}

	// ── HTTP API + 설정 UI 서버 ───────────────────────────────────────
	apiSrv := api.New("config", "web/dist")
	httpAddr := ":" + cfg.ServerPort
	go func() {
		log.Info().Str("addr", httpAddr).Msg("HTTP API 서버 시작")
		if err := http.ListenAndServe(httpAddr, apiSrv.Handler()); err != nil {
			log.Error().Err(err).Msg("HTTP API 서버 종료")
		}
	}()

	// ── Graceful shutdown 대기 ────────────────────────────────────────
	<-ctx.Done()
	log.Info().Msg("종료 신호 수신 — 수집기 정리 중...")
	time.Sleep(500 * time.Millisecond)
	log.Info().Msg("Chart Analyzer 종료 완료")
}

// toEngineConfig converts the app-level RulesConfig (list format from YAML)
// to the engine's map-based RuleConfig.
// Rule weight is read from params["strength"]; defaults to 1.0 if absent.
func toEngineConfig(rc appconfig.RulesConfig) engine.RuleConfig {
	rules := make(map[string]engine.RuleEntry, len(rc.Rules))
	for _, r := range rc.Rules {
		weight := 1.0
		if s, ok := r.Params["strength"].(float64); ok {
			weight = s
		}
		rules[r.Name] = engine.RuleEntry{
			Enabled:   r.Enabled,
			Timeframe: "ALL",
			Weight:    weight,
		}
	}
	return engine.RuleConfig{Rules: rules}
}
