package alpaca

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/execution"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// feedbackRecorder captures every feedback POST the server emits so tests can
// assert status + synchronize without sleeps.
type feedbackRecorder struct {
	mu       sync.Mutex
	received []models.OrderFeedback
	done     chan struct{}
}

func newFeedbackRecorder() *feedbackRecorder {
	return &feedbackRecorder{done: make(chan struct{}, 4)}
}

func (r *feedbackRecorder) handler(secret, pluginID string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		pid := req.Header.Get(execution.PluginIDHeader)
		ts, _ := execution.ParseTimestamp(req.Header.Get(execution.TimestampHeader))
		sig := req.Header.Get(execution.SignatureHeader)
		if !execution.Verify(secret, pid, ts, req.Method, req.URL.Path, body, sig) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		if pid != pluginID {
			http.Error(w, "bad plugin id", http.StatusUnauthorized)
			return
		}
		var fb models.OrderFeedback
		_ = json.Unmarshal(body, &fb)
		r.mu.Lock()
		r.received = append(r.received, fb)
		r.mu.Unlock()
		select {
		case r.done <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func (r *feedbackRecorder) wait(t *testing.T) models.OrderFeedback {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for feedback")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.received[len(r.received)-1]
}

type serverFixture struct {
	srv           *Server
	feedback      *feedbackRecorder
	feedbackSrv   *httptest.Server
	store         *IdempotencyStore
	cfg           Config
	submitCalls   atomic.Int64
	submitHandler func(context.Context, OrderRequest) (*OrderResponse, error)
}

func newServerFixture(t *testing.T) *serverFixture {
	t.Helper()
	fb := newFeedbackRecorder()
	const (
		pluginID = "alpaca-paper"
		secret   = "shh"
	)
	fbSrv := httptest.NewServer(fb.handler(secret, pluginID))
	t.Cleanup(fbSrv.Close)

	store, err := OpenIdempotencyStore(filepath.Join(t.TempDir(), "idem.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	sender, err := NewFeedbackSender(fbSrv.URL+"/api/execution/feedback", pluginID, secret)
	if err != nil {
		t.Fatalf("sender: %v", err)
	}

	cfg := Config{
		AlpacaAPIURL:     "http://127.0.0.1:1", // unused; we override submitFn
		AlpacaAPIKey:     "k",
		AlpacaAPISecret:  "s",
		FeedbackURL:      fbSrv.URL + "/api/execution/feedback",
		PluginSecret:     secret,
		PluginID:         pluginID,
		ListenAddr:       ":0",
		NotionalPerTrade: 1000,
		DBPath:           "",
		TimestampSkewSec: 300,
	}

	fx := &serverFixture{feedback: fb, feedbackSrv: fbSrv, store: store, cfg: cfg}

	alpacaClient := NewAlpacaClient(cfg.AlpacaAPIURL, cfg.AlpacaAPIKey, cfg.AlpacaAPISecret)
	srv := NewServer(cfg, alpacaClient, store, sender, zerolog.Nop())
	srv.WithSubmitFn(func(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
		fx.submitCalls.Add(1)
		if fx.submitHandler != nil {
			return fx.submitHandler(ctx, req)
		}
		return &OrderResponse{ID: "ord-1", Status: "accepted", Symbol: req.Symbol, Qty: req.Qty, Side: req.Side, Type: req.Type}, nil
	})
	fx.srv = srv
	return fx
}

func signedRequest(t *testing.T, cfg Config, method, path string, body []byte, mutate func(r *http.Request)) *http.Request {
	t.Helper()
	ts := time.Now().Unix()
	sig := execution.Sign(cfg.PluginSecret, cfg.PluginID, ts, method, path, body)
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(execution.PluginIDHeader, cfg.PluginID)
	r.Header.Set(execution.TimestampHeader, strconv.FormatInt(ts, 10))
	r.Header.Set(execution.SignatureHeader, sig)
	if mutate != nil {
		mutate(r)
	}
	return r
}

func validSignalBody(t *testing.T, sigID string) []byte {
	t.Helper()
	sig := models.TradeSignal{
		ID: sigID, Version: "1.0", Timestamp: time.Now().UTC(),
		Symbol: "AAPL", Direction: "LONG", Timeframe: "1D", Rule: "ob", EntryPrice: 200,
		Score: 0.9, AssetClass: "stock", Exchange: "nasdaq",
	}
	b, err := json.Marshal(sig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestServer_Webhook_HappyPath(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	body := validSignalBody(t, "sig-happy")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if fx.submitCalls.Load() != 1 {
		t.Errorf("submitCalls = %d want 1", fx.submitCalls.Load())
	}
	got := fx.feedback.wait(t)
	if got.Status != models.OrderStatusSubmitted || got.OrderID != "ord-1" || got.SignalID != "sig-happy" {
		t.Errorf("unexpected feedback: %+v", got)
	}
}

func TestServer_Webhook_FeedbackIncludesSymbol(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	body := validSignalBody(t, "sig-symbol")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	got := fx.feedback.wait(t)
	if got.Symbol != "AAPL" {
		t.Errorf("feedback Symbol = %q, want AAPL", got.Symbol)
	}
}

func TestServer_Webhook_DuplicateSignal(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	body := validSignalBody(t, "sig-dup")

	// First call: 202.
	r1 := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr1 := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr1, r1)
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first call: %d", rr1.Code)
	}
	_ = fx.feedback.wait(t) // drain

	// Second call with same signal_id: 409.
	r2 := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr2 := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr2, r2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("dup call: %d body=%s", rr2.Code, rr2.Body.String())
	}
	if fx.submitCalls.Load() != 1 {
		t.Errorf("submitCalls after dup = %d want 1", fx.submitCalls.Load())
	}
}

func TestServer_Webhook_HMACFail(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	body := validSignalBody(t, "sig-hmac")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, func(r *http.Request) {
		r.Header.Set(execution.SignatureHeader, "deadbeef")
	})
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
	if fx.submitCalls.Load() != 0 {
		t.Errorf("submit called despite HMAC fail")
	}
}

func TestServer_Webhook_ClockSkew(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	body := validSignalBody(t, "sig-skew")
	// Far past timestamp.
	ts := time.Now().Add(-1 * time.Hour).Unix()
	sig := execution.Sign(fx.cfg.PluginSecret, fx.cfg.PluginID, ts, http.MethodPost, "/webhook", body)
	r := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	r.Header.Set(execution.PluginIDHeader, fx.cfg.PluginID)
	r.Header.Set(execution.TimestampHeader, strconv.FormatInt(ts, 10))
	r.Header.Set(execution.SignatureHeader, sig)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestServer_Webhook_UnknownPlugin(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	body := validSignalBody(t, "sig-plg")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, func(r *http.Request) {
		r.Header.Set(execution.PluginIDHeader, "nope")
	})
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestServer_Webhook_WrongMethod(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	r := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, r)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestServer_Webhook_BadJSON(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	body := []byte("{not json")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestServer_Webhook_MapperReject_Crypto(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	sig := models.TradeSignal{
		ID: "sig-crypto", Version: "1.0", Timestamp: time.Now().UTC(),
		Symbol: "BTCUSDT", Direction: "LONG", Timeframe: "1H", Rule: "ob", EntryPrice: 50000,
		AssetClass: "crypto", Exchange: "binance",
	}
	body, _ := json.Marshal(sig)
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	got := fx.feedback.wait(t)
	if got.Status != models.OrderStatusRejected {
		t.Errorf("feedback status = %q want REJECTED", got.Status)
	}
	// After a mapping reject, a retry with a NEW signal_id of a valid signal
	// must still work (release freed the slot for the failed signal_id).
	if fx.submitCalls.Load() != 0 {
		t.Errorf("submit called despite mapping reject")
	}
}

func TestServer_Webhook_AlpacaClientError(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	fx.submitHandler = func(context.Context, OrderRequest) (*OrderResponse, error) {
		return nil, &AlpacaError{StatusCode: 422, Message: "asset not shortable", Code: 42210000}
	}
	body := validSignalBody(t, "sig-422")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	got := fx.feedback.wait(t)
	if got.Status != models.OrderStatusRejected {
		t.Errorf("feedback status = %q want REJECTED", got.Status)
	}
}

func TestServer_Webhook_AlpacaServerError(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	fx.submitHandler = func(context.Context, OrderRequest) (*OrderResponse, error) {
		return nil, &AlpacaError{StatusCode: 503, Message: "service unavailable"}
	}
	body := validSignalBody(t, "sig-503")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	got := fx.feedback.wait(t)
	if got.Status != models.OrderStatusRejected {
		t.Errorf("feedback status = %q want REJECTED", got.Status)
	}
}

func TestServer_Webhook_AlpacaTransportError(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	fx.submitHandler = func(context.Context, OrderRequest) (*OrderResponse, error) {
		return nil, errors.New("dial tcp: connection refused")
	}
	body := validSignalBody(t, "sig-transport")
	req := signedRequest(t, fx.cfg, http.MethodPost, "/webhook", body, nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	got := fx.feedback.wait(t)
	if got.Status != models.OrderStatusError {
		t.Errorf("feedback status = %q want ERROR", got.Status)
	}
}

func TestServer_Webhook_MissingHeaders(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	r := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("{}")))
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestServer_Healthz(t *testing.T) {
	t.Parallel()
	fx := newServerFixture(t)
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	fx.srv.Routes().ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}
