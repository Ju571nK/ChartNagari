package alpaca

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// Runner wires a Config into a listening HTTP server. Callers typically use
// Run(ctx, cfg, logger) from main(); tests construct the lower-level
// components directly and drive Server via httptest.
type Runner struct {
	cfg    Config
	server *http.Server
	store  *IdempotencyStore
	log    zerolog.Logger
}

// NewRunner constructs the adapter's runtime graph but does NOT start it.
// The caller uses Start(ctx) / Shutdown(ctx).
func NewRunner(cfg Config, log zerolog.Logger) (*Runner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	store, err := OpenIdempotencyStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	alpaca := NewAlpacaClient(cfg.AlpacaAPIURL, cfg.AlpacaAPIKey, cfg.AlpacaAPISecret)
	fb, err := NewFeedbackSender(cfg.FeedbackURL, cfg.PluginID, cfg.PluginSecret)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	srv := NewServer(cfg, alpaca, store, fb, log)
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &Runner{cfg: cfg, server: httpServer, store: store, log: log}, nil
}

// Start blocks on ListenAndServe. Returns nil when ctx is cancelled and the
// server has shut down gracefully; otherwise returns the listen error.
func (r *Runner) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		r.log.Info().Str("addr", r.cfg.ListenAddr).Str("alpaca_url", r.cfg.AlpacaAPIURL).
			Str("plugin_id", r.cfg.PluginID).Msg("alpaca: listening")
		err := r.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.server.Shutdown(shutdownCtx)
		_ = r.store.Close()
		return nil
	case err := <-errCh:
		_ = r.store.Close()
		return err
	}
}
