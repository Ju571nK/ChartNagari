package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/collector"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
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
		Str("version", "0.2.0").
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

	// Phase 1-2: Binance WebSocket 수집기
	cryptoSymbols := cfg.EnabledCryptoSymbols()
	if len(cryptoSymbols) > 0 {
		binance := collector.NewBinanceCollector(db, cryptoSymbols, timeframes)
		go binance.Start(ctx)
		log.Info().Strs("symbols", cryptoSymbols).Msg("Binance 수집기 실행 중")
	} else {
		log.Warn().Msg("활성화된 코인 종목 없음 — watchlist.yaml 확인")
	}

	// Phase 1-3: Yahoo Finance 수집기
	stockSymbols := cfg.EnabledStockSymbols()
	if len(stockSymbols) > 0 {
		yahoo := collector.NewYahooCollector(db, stockSymbols, timeframes, cfg.Yahoo.PollInterval)
		go yahoo.Start(ctx)
		log.Info().Strs("symbols", stockSymbols).Msg("Yahoo Finance 수집기 실행 중")
	} else {
		log.Info().Msg("활성화된 주식 종목 없음 — watchlist.yaml에서 enabled: true 설정 필요")
	}

	// TODO Phase 1-4~: 인디케이터 엔진, 룰 엔진, 알림 시스템

	// ── Graceful shutdown 대기 ────────────────────────────────────────
	<-ctx.Done()
	log.Info().Msg("종료 신호 수신 — 수집기 정리 중...")
	time.Sleep(500 * time.Millisecond)
	log.Info().Msg("Chart Analyzer 종료 완료")
}
