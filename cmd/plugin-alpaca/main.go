// Command plugin-alpaca is a ChartNagari execution-plugin adapter that turns
// HMAC-signed TradeSignal webhooks into Alpaca PAPER trading orders and
// reports lifecycle events back to ChartNagari's /api/execution/feedback.
//
// Hard constraint: this binary refuses to start unless ALPACA_API_URL points
// at the Alpaca paper endpoint. Live trading is explicitly out of scope.
//
// Required env vars:
//
//	ALPACA_API_KEY              Alpaca paper key id
//	ALPACA_API_SECRET           Alpaca paper secret
//	CHARTNAGARI_FEEDBACK_URL    e.g. http://127.0.0.1:8080/api/execution/feedback
//	CHARTNAGARI_PLUGIN_SECRET   shared HMAC secret (must match execution.yaml plugin.secret)
//
// Optional env vars (with defaults):
//
//	ALPACA_API_URL              https://paper-api.alpaca.markets
//	CHARTNAGARI_PLUGIN_ID       alpaca-paper
//	LISTEN_ADDR                 :9100
//	ALPACA_NOTIONAL_PER_TRADE   1000 (USD)
//	ALPACA_DB_PATH              ./plugin-alpaca.db
//	ALPACA_TIMESTAMP_SKEW_SEC   300
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/plugins/alpaca"
)

func main() {
	// Best-effort .env load — silent if absent.
	_ = godotenv.Load()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "2006-01-02T15:04:05Z07:00"}).
		With().Str("component", "plugin-alpaca").Logger()

	cfg, err := alpaca.LoadConfigFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("plugin-alpaca: invalid config")
	}

	runner, err := alpaca.NewRunner(cfg, log.Logger)
	if err != nil {
		log.Fatal().Err(err).Msg("plugin-alpaca: init failed")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("plugin-alpaca: server error")
	}
	log.Info().Msg("plugin-alpaca: shutdown complete")
}
