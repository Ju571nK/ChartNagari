package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/analyst"
	"github.com/Ju571nK/Chatter/internal/api"
	"github.com/Ju571nK/Chatter/internal/calendar"
	"github.com/Ju571nK/Chatter/internal/history"
	"github.com/Ju571nK/Chatter/internal/llm"
	"github.com/Ju571nK/Chatter/internal/backtest"
	"github.com/Ju571nK/Chatter/internal/collector"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/execution"
	"github.com/Ju571nK/Chatter/internal/interpreter"
	"github.com/Ju571nK/Chatter/internal/ollama"
	general_ta "github.com/Ju571nK/Chatter/internal/methodology/general_ta"
	"github.com/Ju571nK/Chatter/internal/methodology/ict"
	"github.com/Ju571nK/Chatter/internal/methodology/smc"
	"github.com/Ju571nK/Chatter/internal/methodology/wyckoff"
	candlestick "github.com/Ju571nK/Chatter/internal/methodology/candlestick"
	"github.com/Ju571nK/Chatter/internal/hub"
	"github.com/Ju571nK/Chatter/internal/notifier"
	"github.com/Ju571nK/Chatter/internal/paper"
	"github.com/Ju571nK/Chatter/internal/pricealert"
	"github.com/Ju571nK/Chatter/internal/pipeline"
	"github.com/Ju571nK/Chatter/internal/report"
	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/internal/storage"
)

func main() {
	// ── Load config ────────────────────────────────────────────────────
	cfg, err := appconfig.Load(".env", "config")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
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
		Msg("Chart Nagari server starting")

	// ── SQLite 초기화 ────────────────────────────────────────────────
	db, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", cfg.DBPath).Msg("failed to initialize DB")
	}
	defer db.Close()
	log.Info().Str("path", cfg.DBPath).Msg("SQLite connected")

	// ── 컨텍스트 (SIGINT/SIGTERM 감지) ───────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── 수집기 시작 (goroutine) ──────────────────────────────────────
	timeframes := cfg.Watchlist.Timeframes

	cryptoSymbols := cfg.EnabledCryptoSymbols()
	if len(cryptoSymbols) > 0 {
		binance := collector.NewBinanceCollector(db, cryptoSymbols, timeframes)
		go binance.Start(ctx)
		log.Info().Strs("symbols", cryptoSymbols).Msg("Binance collector started")
	} else {
		log.Warn().Msg("no enabled crypto symbols — check watchlist.yaml")
	}

	stockSymbols := cfg.EnabledStockSymbols()
	if len(stockSymbols) > 0 {
		if cfg.Tiingo.APIKey != "" {
			// Use Tiingo if configured instead of Yahoo
			tiingo := collector.NewTiingoCollector(cfg.Tiingo.APIKey, db, stockSymbols, timeframes, cfg.Tiingo.PollInterval)
			tiingo.SetStateFile(filepath.Join(filepath.Dir(cfg.DBPath), "tiingo_state.json"))
			go tiingo.Start(ctx)
			log.Info().Strs("symbols", stockSymbols).Msg("Tiingo collector started")
		} else {
			// Yahoo fallback when Tiingo API key not set
			yahoo := collector.NewYahooCollector(db, stockSymbols, timeframes, cfg.Yahoo.PollInterval)
			go yahoo.Start(ctx)
			log.Info().Strs("symbols", stockSymbols).Msg("Yahoo Finance collector started (Tiingo not configured — fallback)")
		}
	} else {
		log.Info().Msg("no enabled stock symbols — set enabled: true in watchlist.yaml")
	}

	// ── Index (VIX) data collector — 1D only, not included in pipeline ──
	indexSymbols := cfg.EnabledIndexSymbols()
	if len(indexSymbols) > 0 {
		indexTFs := []string{"1D"} // indices use daily timeframe only
		if cfg.Tiingo.APIKey != "" {
			tiingoIdx := collector.NewTiingoCollector(cfg.Tiingo.APIKey, db, indexSymbols, indexTFs, cfg.Tiingo.PollInterval)
			go tiingoIdx.Start(ctx)
			log.Info().Strs("symbols", indexSymbols).Msg("Index collector started (Tiingo, 1D only)")
		} else {
			yahooIdx := collector.NewYahooCollector(db, indexSymbols, indexTFs, cfg.Yahoo.PollInterval)
			go yahooIdx.Start(ctx)
			log.Info().Strs("symbols", indexSymbols).Msg("Index collector started (Yahoo, 1D only)")
		}
	}

	// ── AlphaVantage 20년 일봉 수집기 (1회 실행) ─────────────────────
	if cfg.AlphaVantage.APIKey != "" {
		avCollector := collector.NewAlphaVantageCollector(cfg.AlphaVantage.APIKey, db, []string{"SPY"})
		go avCollector.Start(ctx)
		log.Info().Msg("AlphaVantage collector started (SPY 20-year daily)")
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
		&general_ta.VSAEffortCandleRule{},
		// ICT
		&ict.ICTOrderBlockRule{},
		&ict.ICTFairValueGapRule{},
		&ict.ICTLiquiditySweepRule{},
		&ict.ICTBreakerBlockRule{},
		ict.NewICTKillZoneRule(),
		&ict.ICTOTERule{},
		&ict.ICTAMDSessionRule{},
		// Wyckoff
		&wyckoff.WyckoffAccumulationRule{},
		&wyckoff.WyckoffDistributionRule{},
		&wyckoff.WyckoffSpringRule{},
		&wyckoff.WyckoffUpthrustRule{},
		&wyckoff.WyckoffVolumeAnomalyRule{},
		// SMC
		&smc.SMCBOSRule{},
		&smc.SMCChoCHRule{},
		// Candlestick
		&candlestick.DojiRule{},
		&candlestick.HammerRule{},
		&candlestick.HangingManRule{},
		&candlestick.ShootingStarRule{},
		&candlestick.InvertedHammerRule{},
		&candlestick.MarubozuRule{},
		&candlestick.BullishEngulfingRule{},
		&candlestick.BearishEngulfingRule{},
		&candlestick.BullishHaramiRule{},
		&candlestick.BearishHaramiRule{},
		&candlestick.MorningStarRule{},
		&candlestick.EveningStarRule{},
		&candlestick.ThreeWhiteSoldiersRule{},
		&candlestick.ThreeBlackCrowsRule{},
	}

	eng := engine.New(toEngineConfig(cfg.Rules))
	for _, r := range allRules {
		eng.Register(r)
	}

	// ── WebSocket Hub ─────────────────────────────────────────────────
	wsHub := hub.New(log.Logger)
	go wsHub.Run()

	// ── AlertConfig 홀더 ──────────────────────────────────────────────
	alertHolder := appconfig.NewAlertConfigHolder(cfg.Alert)

	// ── Signal Tuning 홀더 ────────────────────────────────────────────
	tuningCfg, err := appconfig.LoadSignalTuning("config/signal_tuning.yaml")
	if err != nil {
		log.Warn().Err(err).Msg("failed to load signal_tuning.yaml — using defaults")
		tuningCfg = appconfig.DefaultSignalTuning()
	}
	tuningHolder := appconfig.NewSignalTuningHolder(tuningCfg)
	log.Info().
		Int("htf_penalty", tuningCfg.HTFFilter.CounterTrendPenaltyPct).
		Int("low_vol_pctl", tuningCfg.VolatilityRegime.LowVolPercentile).
		Int("high_vol_pctl", tuningCfg.VolatilityRegime.HighVolPercentile).
		Msg("signal tuning config loaded")

	// ── Symbol Profiles 홀더 ──────────────────────────────────────────
	profilesCfg, err := appconfig.LoadSymbolProfiles("config/symbol_profiles.yaml")
	if err != nil {
		log.Warn().Err(err).Msg("failed to load symbol_profiles.yaml — using defaults")
		profilesCfg = appconfig.SymbolProfilesConfig{}
	}
	profileHolder := appconfig.NewSymbolProfilesHolder(profilesCfg)
	log.Info().
		Int("profiles", len(profilesCfg.Profiles)).
		Int("overrides", len(profilesCfg.SymbolOverrides)).
		Msg("symbol profiles loaded")

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
	notif.SetAlertConfigHolder(alertHolder)

	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
		notif.Register(notifier.NewTelegramSender(cfg.Telegram.BotToken, cfg.Telegram.ChatID))
		log.Info().Msg("Telegram notifications enabled")
	}
	if cfg.Discord.WebhookURL != "" {
		notif.Register(notifier.NewDiscordSender(cfg.Discord.WebhookURL))
		log.Info().Msg("Discord notifications enabled")
	}

	// ── AI 해석 레이어 ────────────────────────────────────────────────
	interp := interpreter.New(cfg.Anthropic.APIKey, cfg.Anthropic.MinScore, cfg.Language)
	if cfg.Anthropic.APIKey != "" {
		log.Info().Float64("min_score", cfg.Anthropic.MinScore).Msg("Claude AI interpretation enabled")
	} else {
		log.Info().Msg("Claude AI interpretation disabled (ANTHROPIC_API_KEY not set)")
	}

	// ── 페이퍼 트레이딩 엔진 ──────────────────────────────────────────────
	paperTrader := paper.New(db, log.Logger)

	// ── Trade Execution Dispatcher (Phase 2) ─────────────────────────────
	// Load execution.yaml (missing file → disabled config with no plugins).
	// Wire: ExecutionHolder → DedupStore → Dispatcher → FeedbackIdempotency.
	// The dispatcher is always constructed; individual dispatches are gated by
	// ExecutionConfig.Enabled + KillSwitch so an empty config is a no-op.
	execPath := "config/execution.yaml"
	execCfg, err := appconfig.LoadExecutionConfig(execPath)
	if err != nil {
		log.Warn().Err(err).Str("path", execPath).Msg("failed to load execution config — using disabled default")
		execCfg = appconfig.ExecutionConfig{}
	}
	execHolder := appconfig.NewExecutionHolder(execPath, execCfg)
	dedupStore := execution.NewDedupStore(db.Conn(), execCfg.DedupWindow())
	dispatcher := execution.New(execHolder, dedupStore, execution.Options{Logger: log.Logger})
	feedbackIdem := execution.NewFeedbackIdempotency(db.Conn())
	execState := execution.NewStateStore(db.Conn())
	dedupCleaner := execution.NewDedupCleaner(dedupStore, log.Logger)
	go dedupCleaner.Run(ctx)
	log.Info().
		Bool("enabled", execCfg.Enabled).
		Bool("kill_switch", execCfg.KillSwitch).
		Int("plugins", len(execCfg.Plugins)).
		Msg("execution dispatcher wired")

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
		priceWatcher := pricealert.New(db, notif, log.Logger)
		pipe.SetPriceAlertWatcher(priceWatcher)
		pipe.SetBroadcaster(wsHub)
		pipe.SetAlertConfigHolder(alertHolder)
		pipe.SetSymbolProfiles(profileHolder)
		pipe.SetSignalTuningHolder(tuningHolder)
		pipe.SetForwardReturnStore(db, db)
		pipe.SetCryptoSymbols(cryptoSymbols)
		pipe.SetExecutionDispatcher(dispatcher)
		go pipe.Run(ctx)
		log.Info().
			Strs("symbols", allSymbols).
			Dur("interval", pipeline.DefaultConfig().Interval).
			Msg("analysis pipeline started")
	}

	// ── 경제 캘린더 ───────────────────────────────────────────────────
	if cfg.Finnhub.APIKey != "" || cfg.FMP.APIKey != "" {
		calProvider := "finnhub"
		if cfg.FMP.APIKey != "" {
			calProvider = "fmp"
		}
		calFetcher := calendar.New(cfg.Finnhub.APIKey, cfg.FMP.APIKey, db, log.Logger)
		go calFetcher.Run(ctx)
		alertWindow := time.Duration(cfg.Finnhub.AlertWindowMinutes) * time.Minute
		calWatcher := calendar.NewWatcher(db, notif, alertWindow, log.Logger)
		go calWatcher.Run(ctx)
		log.Info().Str("provider", calProvider).Msg("economic calendar enabled")
	} else {
		log.Info().Msg("economic calendar disabled (set FMP_API_KEY or FINNHUB_API_KEY)")
	}

	// ── 백테스팅 엔진 구성 ────────────────────────────────────────────
	btEngine := backtest.New(allRules, toEngineConfig(cfg.Rules), backtest.DefaultConfig())
	btRunner := backtest.NewRunner(db, btEngine)

	// ── 일일 리포트 스케줄러 ─────────────────────────────────────────────────────────────────
	reporter := report.NewDailyReporter(db, notif, stockSymbols, log.Logger)
	sched := report.NewScheduler(reporter, cfg.DailyReport, log.Logger)
	go sched.Start(ctx)
	log.Info().
		Bool("enabled", cfg.DailyReport.Enabled).
		Str("time", cfg.DailyReport.Time).
		Msg("daily report scheduler registered")

	// ── HTTP API + 설정 UI 서버 ───────────────────────────────────────
	apiSrv := api.New("config", "web/dist")
	apiSrv.WithSettingsFile("config/settings.yaml")
	apiSrv.WithDBPath(cfg.DBPath)
	apiSrv.WithChartStore(db)
	apiSrv.WithBacktestRunner(btRunner)
	apiSrv.WithPaperStore(db)
	apiSrv.WithReportScheduler(sched)
	apiSrv.WithAlertConfigHolder(alertHolder)
	apiSrv.WithAnnouncer(notif)
	apiSrv.WithPriceAlertStore(db)
	apiSrv.WithHub(wsHub)
	apiSrv.WithCalendarStore(db)
	apiSrv.WithDemoEngine(eng)
	apiSrv.WithSymbolProfiles(profileHolder)
	apiSrv.WithSignalTuningHolder(tuningHolder)
	apiSrv.WithExecutionHolder(execHolder, execPath)
	apiSrv.WithExecutionDispatcher(dispatcher)
	apiSrv.WithExecutionFeedback(feedbackIdem)
	apiSrv.WithExecutionDB(db.Conn())
	apiSrv.WithExecutionState(execState)

	// Ollama detector (opt-in local LLM status endpoint).
	ollamaDet := ollama.NewDetector(cfg.Ollama.Host, cfg.Ollama.Model, ollama.DefaultRuntime())
	apiSrv.WithOllamaDetector(ollamaDet)

	// ── Multi-analyst AI 분석 엔진 ────────────────────────────────────
	var llmProvider llm.Provider
	selectedProvider := cfg.LLMProvider
	if selectedProvider == "" {
		// Auto-select: key priority anthropic → openai → groq → gemini
		switch {
		case cfg.Anthropic.APIKey != "":
			selectedProvider = "anthropic"
		case cfg.OpenAI.APIKey != "":
			selectedProvider = "openai"
		case cfg.Groq.APIKey != "":
			selectedProvider = "groq"
		case cfg.Gemini.APIKey != "":
			selectedProvider = "gemini"
		}
	}
	switch selectedProvider {
	case "anthropic":
		if cfg.Anthropic.APIKey != "" {
			llmProvider = llm.NewAnthropicProvider(cfg.Anthropic.APIKey)
			log.Info().Msg("Multi-analyst AI: using Anthropic Claude Opus 4.6")
		}
	case "openai":
		if cfg.OpenAI.APIKey != "" {
			llmProvider = llm.NewOpenAIProvider(cfg.OpenAI.APIKey)
			log.Info().Msg("Multi-analyst AI: using OpenAI GPT-4o")
		}
	case "groq":
		if cfg.Groq.APIKey != "" {
			llmProvider = llm.NewGroqProvider(cfg.Groq.APIKey)
			log.Info().Msg("Multi-analyst AI: using Groq Llama 3.3 70B")
		}
	case "gemini":
		if cfg.Gemini.APIKey != "" {
			llmProvider = llm.NewGeminiProvider(cfg.Gemini.APIKey)
			log.Info().Msg("Multi-analyst AI: using Google Gemini 1.5 Flash")
		}
	case "ollama":
		if cfg.Ollama.Host != "" && cfg.Ollama.Model != "" {
			llmProvider = llm.NewOllamaProvider(cfg.Ollama.Host, cfg.Ollama.Model, cfg.Ollama.Timeout)
			log.Info().
				Str("host", cfg.Ollama.Host).
				Str("model", cfg.Ollama.Model).
				Msg("Multi-analyst AI: using Ollama (local)")
		}
	}
	var director *analyst.Director
	if llmProvider != nil {
		director = analyst.NewDirector(llmProvider)
		apiSrv.WithFullStore(db)
		apiSrv.WithAnalystDirector(director)
		log.Info().Str("provider", selectedProvider).Msg("Multi-analyst AI analysis enabled")
	} else {
		log.Info().Msg("Multi-analyst AI disabled (no API key — set ANTHROPIC/OPENAI/GROQ/GEMINI_API_KEY or LLM_PROVIDER=ollama)")
	}

	// ── Telegram 봇 명령어 수신 (/analysis SYMBOL) ────────────────────
	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" && director != nil {
		analysisHandler := func(botCtx context.Context, symbol string) (string, error) {
			bars, err := db.GetOHLCVAll(symbol, "1D")
			if err != nil || len(bars) == 0 {
				return "", fmt.Errorf("no data found: %s", symbol)
			}
			sum := history.New().Summarize(symbol, bars)
			input := analyst.AnalystInput{Symbol: symbol, HistorySummary: sum, Language: cfg.Language}
			if strings.ToUpper(symbol) != "SPY" {
				if spyBars, err := db.GetOHLCVAll("SPY", "1D"); err == nil && len(spyBars) > 0 {
					input.MacroContext = history.New().Summarize("SPY", spyBars)
				}
			}
			res := director.Analyze(botCtx, input)
			finalEmoji := map[string]string{"BULL": "🟢", "BEAR": "🔴", "SIDEWAYS": "🟡"}[res.Final]
			return fmt.Sprintf(
				"📊 <b>%s Analysis</b>\n\n%s <b>%s</b> | %s\n\n📈 %.1f%% / 📉 %.1f%% / ➡️ %.1f%%\n\n<i>%s</i>",
				res.Symbol, finalEmoji, res.Final, res.Confidence,
				res.BullPct, res.BearPct, res.SidewaysPct,
				res.AggregatorReason,
			), nil
		}
		tgBot := notifier.NewTelegramBot(cfg.Telegram.BotToken, cfg.Telegram.ChatID, analysisHandler)
		go tgBot.Start(ctx)
		log.Info().Msg("Telegram bot command listener enabled (/analysis SYMBOL)")
	}

	// 활성 데이터 소스 목록 전달 (상태탭 표시용)
	activeSources := []string{"Binance (BTC/ETH)"}
	if cfg.Tiingo.APIKey != "" {
		activeSources = append(activeSources, "Tiingo ("+strings.Join(stockSymbols, "/")+")")
	} else if len(stockSymbols) > 0 {
		activeSources = append(activeSources, "Yahoo Finance ("+strings.Join(stockSymbols, "/")+")")
	}
	apiSrv.WithDataSources(activeSources)
	if cfg.APIToken != "" {
		apiSrv.WithAPIToken(cfg.APIToken)
	}
	httpAddr := cfg.ServerHost + ":" + cfg.ServerPort
	go func() {
		log.Info().Str("addr", httpAddr).Msg("HTTP API server starting")
		if err := http.ListenAndServe(httpAddr, apiSrv.Handler()); err != nil {
			log.Error().Err(err).Msg("HTTP API server stopped")
		}
	}()

	// ── 서버 시작 알림 ────────────────────────────────────────────────
	dataSource := "Yahoo Finance (fallback)"
	if cfg.Tiingo.APIKey != "" {
		dataSource = "Tiingo"
	}
	startupMsg := fmt.Sprintf(
		"🚀 <b>Chart Nagari Started</b>\n\n"+
			"📊 Symbols: <code>%s</code>\n"+
			"📋 Active rules: %d\n"+
			"🔌 Data: %s\n"+
			"🌐 API: :%s\n"+
			"⏰ %s UTC",
		strings.Join(allSymbols, ", "),
		len(allRules),
		dataSource,
		cfg.ServerPort,
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)
	go notif.Announce(context.Background(), startupMsg)

	// ── Graceful shutdown 대기 ────────────────────────────────────────
	<-ctx.Done()
	log.Info().Msg("shutdown signal received — cleaning up...")
	time.Sleep(500 * time.Millisecond)
	log.Info().Msg("Chart Nagari stopped")
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
