package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/api"
	"github.com/Ju571nK/Chatter/internal/backtest"
	"github.com/Ju571nK/Chatter/internal/collector"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/interpreter"
	general_ta "github.com/Ju571nK/Chatter/internal/methodology/general_ta"
	"github.com/Ju571nK/Chatter/internal/methodology/ict"
	"github.com/Ju571nK/Chatter/internal/methodology/smc"
	"github.com/Ju571nK/Chatter/internal/methodology/wyckoff"
	"github.com/Ju571nK/Chatter/internal/notifier"
	"github.com/Ju571nK/Chatter/internal/paper"
	"github.com/Ju571nK/Chatter/internal/pipeline"
	"github.com/Ju571nK/Chatter/internal/rule"
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
		if cfg.Tiingo.APIKey != "" {
			// Tiingo가 설정된 경우 Yahoo 대신 사용
			tiingo := collector.NewTiingoCollector(cfg.Tiingo.APIKey, db, stockSymbols, timeframes, cfg.Tiingo.PollInterval)
			go tiingo.Start(ctx)
			log.Info().Strs("symbols", stockSymbols).Msg("Tiingo 수집기 실행 중")
		} else {
			// Tiingo API Key 미설정 시 Yahoo Fallback
			yahoo := collector.NewYahooCollector(db, stockSymbols, timeframes, cfg.Yahoo.PollInterval)
			go yahoo.Start(ctx)
			log.Info().Strs("symbols", stockSymbols).Msg("Yahoo Finance 수집기 실행 중 (Tiingo 미설정 — fallback)")
		}
	} else {
		log.Info().Msg("활성화된 주식 종목 없음 — watchlist.yaml에서 enabled: true 설정 필요")
	}

	// ── 룰 엔진 구성 ─────────────────────────────────────────────────
	// Collect all rules into a slice so they can be shared with the backtest engine.
	allRules := []rule.AnalysisRule{
		// General TA
		&general_ta.RSIOverboughtOversoldRule{},
		&general_ta.RSIDivergenceRule{},
		&general_ta.EMACrossRule{},
		&general_ta.SupportResistanceBreakoutRule{},
		&general_ta.FibonacciConfluenceRule{},
		&general_ta.VolumeSpikeRule{},
		// ICT
		&ict.ICTOrderBlockRule{},
		&ict.ICTFairValueGapRule{},
		&ict.ICTLiquiditySweepRule{},
		&ict.ICTBreakerBlockRule{},
		ict.NewICTKillZoneRule(),
		// Wyckoff
		&wyckoff.WyckoffAccumulationRule{},
		&wyckoff.WyckoffDistributionRule{},
		&wyckoff.WyckoffSpringRule{},
		&wyckoff.WyckoffUpthrustRule{},
		&wyckoff.WyckoffVolumeAnomalyRule{},
		// SMC
		&smc.SMCBOSRule{},
		&smc.SMCChoCHRule{},
	}

	eng := engine.New(toEngineConfig(cfg.Rules))
	for _, r := range allRules {
		eng.Register(r)
	}

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

	// ── 페이퍼 트레이딩 엔진 ──────────────────────────────────────────────
	paperTrader := paper.New(db, log.Logger)

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
		pipe.SetSignalSaver(db)
		pipe.SetPaperTrader(paperTrader)
		go pipe.Run(ctx)
		log.Info().
			Strs("symbols", allSymbols).
			Dur("interval", pipeline.DefaultConfig().Interval).
			Msg("분석 파이프라인 실행 중")
	}

	// ── 백테스팅 엔진 구성 ────────────────────────────────────────────
	btEngine := backtest.New(allRules, toEngineConfig(cfg.Rules), backtest.DefaultConfig())
	btRunner := backtest.NewRunner(db, btEngine)

	// ── HTTP API + 설정 UI 서버 ───────────────────────────────────────
	apiSrv := api.New("config", "web/dist")
	apiSrv.WithChartStore(db)
	apiSrv.WithBacktestRunner(btRunner)
	apiSrv.WithPaperStore(db)

	// 활성 데이터 소스 목록 전달 (상태탭 표시용)
	activeSources := []string{"Binance (BTC/ETH)"}
	if cfg.Tiingo.APIKey != "" {
		activeSources = append(activeSources, "Tiingo ("+strings.Join(stockSymbols, "/")+")")
	} else if len(stockSymbols) > 0 {
		activeSources = append(activeSources, "Yahoo Finance ("+strings.Join(stockSymbols, "/")+")")
	}
	apiSrv.WithDataSources(activeSources)
	httpAddr := ":" + cfg.ServerPort
	go func() {
		log.Info().Str("addr", httpAddr).Msg("HTTP API 서버 시작")
		if err := http.ListenAndServe(httpAddr, apiSrv.Handler()); err != nil {
			log.Error().Err(err).Msg("HTTP API 서버 종료")
		}
	}()

	// ── 서버 시작 알림 ────────────────────────────────────────────────
	dataSource := "Yahoo Finance (fallback)"
	if cfg.Tiingo.APIKey != "" {
		dataSource = "Tiingo"
	}
	startupMsg := fmt.Sprintf(
		"🚀 <b>Chart Analyzer 시작</b>\n\n"+
			"📊 종목: <code>%s</code>\n"+
			"📋 활성 룰: %d개\n"+
			"🔌 데이터: %s\n"+
			"🌐 API: :%s\n"+
			"⏰ %s KST",
		strings.Join(allSymbols, ", "),
		len(allRules),
		dataSource,
		cfg.ServerPort,
		time.Now().In(time.FixedZone("KST", 9*3600)).Format("2006-01-02 15:04:05"),
	)
	go notif.Announce(context.Background(), startupMsg)

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
