package alpaca

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/execution"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// Server is the /webhook HTTP handler: it authenticates inbound TradeSignal
// POSTs, reserves idempotency, submits to Alpaca, persists the resulting
// order id, and fires an async OrderFeedback POST back to ChartNagari.
type Server struct {
	cfg     Config
	alpaca  *AlpacaClient
	store   *IdempotencyStore
	feedbck *FeedbackSender
	log     zerolog.Logger

	// nowFn is injected for deterministic HMAC tests. Defaults to time.Now.
	nowFn func() time.Time

	// submitFn is a test seam that lets unit tests replace AlpacaClient.SubmitOrder
	// without standing up a full httptest.Server per case. In production nil →
	// falls through to alpaca.SubmitOrder.
	submitFn func(ctx context.Context, req OrderRequest) (*OrderResponse, error)

	// feedbackWG tracks in-flight async feedback goroutines so the Runner can
	// wait for them to drain during graceful shutdown. Without this the process
	// could exit while a feedback POST was mid-flight, silently dropping the
	// status update that ChartNagari relies on to close the in-flight window.
	feedbackWG sync.WaitGroup
}

// WaitFeedback blocks until all in-flight async feedback POSTs have finished.
// Callers pass a bounded ctx so a stuck feedback send cannot block shutdown
// forever — the goroutines themselves carry their own HTTPTimeout.
func (s *Server) WaitFeedback(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.feedbackWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// NewServer builds a Server from resolved dependencies. Callers construct the
// AlpacaClient, IdempotencyStore, and FeedbackSender and hand them in — this
// keeps Server unit-testable without touching disk or network.
func NewServer(cfg Config, alpaca *AlpacaClient, store *IdempotencyStore, fb *FeedbackSender, log zerolog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		alpaca:  alpaca,
		store:   store,
		feedbck: fb,
		log:     log,
		nowFn:   time.Now,
	}
}

// WithSubmitFn overrides the Alpaca submission path (tests). When set it is
// called instead of s.alpaca.SubmitOrder.
func (s *Server) WithSubmitFn(fn func(context.Context, OrderRequest) (*OrderResponse, error)) *Server {
	s.submitFn = fn
	return s
}

// Routes returns an *http.ServeMux with the adapter endpoints mounted. Having
// a dedicated method keeps main() thin and makes httptest wiring trivial.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// handleWebhook is the heart of the adapter. Error-response taxonomy:
//
//	400 bad JSON / missing fields
//	401 HMAC fail / clock skew / unknown plugin
//	405 wrong method
//	409 duplicate signal_id (idempotency)
//	422 mapping error or Alpaca 4xx (client-fault)
//	502 Alpaca 5xx (upstream fault)
//	500 internal (db write failed, etc.)
//
// The 202 return value means "we accepted the signal and are acting on it" —
// terminal status is reported asynchronously via the feedback callback.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the raw body once — HMAC is verified over exact bytes, and we also
	// reuse the same bytes to parse JSON below so there's zero re-marshal drift.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// --- Auth: HMAC verification using the SAME canonical format as dispatcher.
	pluginID := r.Header.Get(execution.PluginIDHeader)
	sigHex := r.Header.Get(execution.SignatureHeader)
	tsRaw := r.Header.Get(execution.TimestampHeader)
	if pluginID == "" || sigHex == "" || tsRaw == "" {
		http.Error(w, "missing auth headers", http.StatusUnauthorized)
		return
	}
	if pluginID != s.cfg.PluginID {
		http.Error(w, "unknown plugin_id", http.StatusUnauthorized)
		return
	}
	ts, err := execution.ParseTimestamp(tsRaw)
	if err != nil {
		http.Error(w, "bad timestamp", http.StatusUnauthorized)
		return
	}
	if !execution.WithinSkew(ts, s.cfg.TimestampSkewSec, s.nowFn()) {
		http.Error(w, "timestamp outside skew window", http.StatusUnauthorized)
		return
	}
	if !execution.Verify(s.cfg.PluginSecret, pluginID, ts, r.Method, r.URL.Path, body, sigHex) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	// --- Parse the TradeSignal envelope.
	var sig models.TradeSignal
	if err := json.Unmarshal(body, &sig); err != nil {
		http.Error(w, "invalid trade signal json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if sig.ID == "" {
		http.Error(w, "signal id required", http.StatusBadRequest)
		return
	}

	// --- Idempotency reservation. If we've already seen this signal_id, return
	// 409 immediately — the dispatcher treats 4xx as client-fault and will not
	// retry, so double submissions are impossible.
	ctx := r.Context()
	if err := s.store.Reserve(ctx, sig.ID, s.nowFn()); err != nil {
		if errors.Is(err, ErrDuplicate) {
			s.log.Info().Str("signal_id", sig.ID).Msg("alpaca: duplicate signal_id, skip")
			http.Error(w, "duplicate signal_id", http.StatusConflict)
			return
		}
		s.log.Error().Err(err).Str("signal_id", sig.ID).Msg("alpaca: idempotency reserve failed")
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}

	// --- Map TradeSignal → Alpaca order.
	orderReq, err := MapTradeSignalToOrder(sig, s.cfg.NotionalPerTrade)
	if err != nil {
		// Release the idempotency slot because we never actually submitted — a
		// manual re-send with the same signal_id after a config fix should work.
		_ = s.store.Release(ctx, sig.ID)
		s.log.Warn().Err(err).Str("signal_id", sig.ID).Str("symbol", sig.Symbol).Msg("alpaca: mapping rejected")
		s.sendFeedbackAsync(sig.ID, "", sig.Symbol, models.OrderStatusRejected, err.Error())
		http.Error(w, "mapping rejected: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// --- Submit to Alpaca. We respond 202 BEFORE the Alpaca call only if we
	// deferred it to a goroutine — but to give the dispatcher a meaningful
	// response code we submit synchronously within the webhook timeout. This
	// is still well under the dispatcher's 10s HTTP timeout because Alpaca
	// paper typically returns in <500ms.
	submitCtx, cancel := context.WithTimeout(ctx, HTTPTimeout)
	defer cancel()
	submit := s.submitFn
	if submit == nil {
		submit = s.alpaca.SubmitOrder
	}
	resp, err := submit(submitCtx, orderReq)
	if err != nil {
		var ae *AlpacaError
		if errors.As(err, &ae) {
			s.log.Warn().Int("status", ae.StatusCode).Str("signal_id", sig.ID).
				Str("symbol", sig.Symbol).Msg("alpaca: order rejected")
			s.sendFeedbackAsync(sig.ID, "", sig.Symbol, models.OrderStatusRejected, ae.Error())
			if ae.IsServerError() {
				http.Error(w, "alpaca upstream error", http.StatusBadGateway)
				return
			}
			// Don't echo raw upstream text back to the dispatcher — Alpaca error
			// bodies can contain account-scoped detail (position sizes, equity).
			// Full detail is already in the Warn log above and in the async
			// feedback payload, both of which stay inside our trust boundary.
			http.Error(w, "alpaca rejected order", http.StatusUnprocessableEntity)
			return
		}
		s.log.Error().Err(err).Str("signal_id", sig.ID).Msg("alpaca: submit failed")
		s.sendFeedbackAsync(sig.ID, "", sig.Symbol, models.OrderStatusError, err.Error())
		http.Error(w, "submit failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// --- Persist the order id on the idempotency row for future diagnostics.
	if err := s.store.MarkSubmitted(ctx, sig.ID, resp.ID, resp.Status); err != nil {
		// Non-fatal: the order is already live on Alpaca. Log and continue.
		s.log.Error().Err(err).Str("plugin_name", s.cfg.PluginID).Str("signal_id", sig.ID).
			Str("order_id", resp.ID).Msg("alpaca: mark submitted failed (order still live upstream)")
	}

	s.log.Info().Str("plugin_name", s.cfg.PluginID).Str("signal_id", sig.ID).
		Str("order_id", resp.ID).Str("symbol", resp.Symbol).Str("side", resp.Side).
		Str("qty", resp.Qty).Str("status", resp.Status).Msg("alpaca: order submitted")

	// --- Feedback to ChartNagari. Non-terminal "SUBMITTED" acknowledges the
	// handoff; a separate FILLED/REJECTED/CANCELED event would normally come
	// from a long-poll or streaming connection — that's Phase 4 scope.
	s.sendFeedbackAsync(sig.ID, resp.ID, sig.Symbol, models.OrderStatusSubmitted, "accepted by alpaca")

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "accepted",
		"order_id": resp.ID,
	})
}

// sendFeedbackAsync fires a feedback POST in a goroutine with its own timeout
// so webhook response is never blocked on ChartNagari. Failures are logged.
func (s *Server) sendFeedbackAsync(signalID, orderID, symbol, status, message string) {
	if s.feedbck == nil {
		return
	}
	fb := models.OrderFeedback{
		SignalID:   signalID,
		OrderID:    orderID,
		Symbol:     symbol,
		Status:     status,
		Message:    message,
		PluginName: s.cfg.PluginID,
		Timestamp:  s.nowFn().UTC(),
	}
	s.feedbackWG.Add(1)
	go func() {
		defer s.feedbackWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), HTTPTimeout)
		defer cancel()
		if err := s.feedbck.Send(ctx, fb); err != nil {
			s.log.Warn().Err(err).Str("plugin_name", s.cfg.PluginID).
				Str("signal_id", signalID).Str("status", status).
				Msg("alpaca: feedback send failed")
		}
	}()
}
