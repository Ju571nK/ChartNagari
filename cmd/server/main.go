package main

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// 구조화 로깅 초기화 (콘솔 출력은 pretty print, 프로덕션은 JSON)
	if os.Getenv("ENV") == "production" {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	}

	log.Info().
		Str("version", "0.1.0").
		Str("env", getEnv("ENV", "development")).
		Msg("Chart Analyzer 서버 시작")

	// TODO: Phase 1-1 이후 수집기, 룰 엔진, 알림 시스템 초기화
	// collector.Start()
	// engine.Start()
	// notifier.Start()

	log.Info().Msg("서버 종료")
}

// getEnv returns the value of an environment variable or a fallback default.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
