package brokerplugin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

type ServerConfig struct {
	PluginID string
	Secret   string
	// Default false. This is an independent API gate, NOT ChartNagari's global
	// execution kill switch. A production host must integrate its risk controls.
	OrdersEnabled bool
}

type Server struct {
	cfg      ServerConfig
	broker   Broker
	journal  *Journal
	manifest Manifest
	mux      *http.ServeMux
}

func NewServer(cfg ServerConfig, broker Broker, journal *Journal) (*Server, error) {
	if broker == nil || journal == nil || cfg.Secret == "" {
		return nil, errors.New("broker, journal and shared secret required")
	}
	m := broker.Manifest()
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if m.ID != cfg.PluginID {
		return nil, errors.New("manifest ID must match configured plugin ID")
	}
	if m.Capabilities.Cancel {
		if _, ok := broker.(Canceller); !ok {
			return nil, errors.New("cancel capability requires Canceller")
		}
	}
	m.OrdersEnabled = cfg.OrdersEnabled
	s := &Server{cfg: cfg, broker: broker, journal: journal, manifest: m, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /v1/manifest", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, m) })
	s.mux.HandleFunc("GET /v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		a, err := broker.Accounts(r.Context())
		if err != nil {
			brokerError(w, err)
			return
		}
		for _, account := range a {
			if account.Validate() != nil {
				fail(w, 502, "invalid_broker_response")
				return
			}
		}
		if a == nil {
			a = []Account{}
		}
		respond(w, 200, a)
	})
	s.mux.HandleFunc("GET /v1/orders", s.orders)
	s.mux.HandleFunc("GET /v1/orders/{id}", s.order)
	s.mux.HandleFunc("POST /v1/orders", s.submit)
	s.mux.HandleFunc("POST /v1/orders/{id}/cancel", s.cancel)
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not_found") })
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "GET" && r.Method != "POST" {
		fail(w, 405, "method_not_allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		fail(w, 413, "body_too_large")
		return
	}
	ts, err := strconv.ParseInt(r.Header.Get(TimestampHeader), 10, 64)
	now := time.Now().Unix()
	if err != nil || ts < now-300 || ts > now+300 || r.Header.Get(PluginIDHeader) != s.cfg.PluginID || !hmac.Equal([]byte(r.Header.Get(SignatureHeader)), []byte(Sign(s.cfg.Secret, s.cfg.PluginID, ts, r.Method, r.URL.RequestURI(), body))) {
		fail(w, 401, "authentication_failed")
		return
	}
	if r.Method == "GET" && len(body) > 0 {
		fail(w, 400, "get_body_not_allowed")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	s.mux.ServeHTTP(w, r.WithContext(ctx))
}

func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		fail(w, 400, "invalid_request")
		return false
	}
	return true
}

func (s *Server) account(ctx context.Context, id string) error {
	if !idPattern.MatchString(id) {
		return ErrNotFound
	}
	a, err := s.broker.Accounts(ctx)
	if err != nil {
		return err
	}
	for _, v := range a {
		if v.ID == id && v.Validate() == nil {
			return nil
		}
	}
	return ErrNotFound
}

func (s *Server) orders(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("account_id")
	if err := s.account(r.Context(), id); err != nil {
		brokerError(w, err)
		return
	}
	page, err := s.broker.Orders(r.Context(), id)
	if err != nil {
		brokerError(w, err)
		return
	}
	for _, o := range page.Orders {
		if validateOrder(o, id, o.ClientOrderID) != nil {
			fail(w, 502, "invalid_broker_response")
			return
		}
	}
	if page.Orders == nil {
		page.Orders = []Order{}
	}
	respond(w, 200, page)
}

func (s *Server) order(w http.ResponseWriter, r *http.Request) {
	account, id := r.URL.Query().Get("account_id"), r.PathValue("id")
	if !idPattern.MatchString(id) {
		fail(w, 400, "invalid_order_id")
		return
	}
	if err := s.account(r.Context(), account); err != nil {
		brokerError(w, err)
		return
	}
	o, err := s.broker.Order(r.Context(), account, id)
	if err != nil {
		brokerError(w, err)
		return
	}
	if validateOrder(o, account, id) != nil {
		fail(w, 502, "invalid_broker_response")
		return
	}
	respond(w, 200, o)
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.OrdersEnabled {
		fail(w, 423, "orders_disabled")
		return
	}
	if !s.manifest.Capabilities.Submit {
		fail(w, 501, "unsupported")
		return
	}
	var req OrderRequest
	if !decode(w, r, &req) {
		return
	}
	if err := req.Validate(s.manifest.Capabilities); err != nil {
		fail(w, 422, "invalid_or_unsupported_order")
		return
	}
	if err := s.account(r.Context(), req.AccountID); err != nil {
		brokerError(w, err)
		return
	}
	s.write(w, r, "submit/"+req.AccountID+"/"+req.ClientOrderID, req, req.AccountID, req.ClientOrderID, func() (Order, error) { return s.broker.Submit(r.Context(), req) })
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.OrdersEnabled {
		fail(w, 423, "orders_disabled")
		return
	}
	broker, ok := s.broker.(Canceller)
	if !ok || !s.manifest.Capabilities.Cancel {
		fail(w, 501, "unsupported")
		return
	}
	var req CancelRequest
	if !decode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if !idPattern.MatchString(id) || !idPattern.MatchString(req.RequestID) {
		fail(w, 422, "invalid_request_id")
		return
	}
	if err := s.account(r.Context(), req.AccountID); err != nil {
		brokerError(w, err)
		return
	}
	// Include target order in fingerprint so a reused cancellation ID cannot
	// accidentally apply to a different order.
	payload := struct {
		CancelRequest
		ClientOrderID string `json:"client_order_id"`
	}{req, id}
	s.write(w, r, "cancel/"+req.AccountID+"/"+req.RequestID, payload, req.AccountID, id, func() (Order, error) { return broker.Cancel(r.Context(), req.AccountID, id) })
}

func (s *Server) write(w http.ResponseWriter, r *http.Request, key string, payload any, account, id string, invoke func() (Order, error)) {
	raw, _ := json.Marshal(payload)
	hash := sha256.Sum256(raw)
	fresh, previous, conflict, err := s.journal.reserve(r.Context(), key, hex.EncodeToString(hash[:]))
	if err != nil {
		fail(w, 503, "journal_unavailable")
		return
	}
	if conflict {
		fail(w, 409, "idempotency_conflict")
		return
	}
	unknown := Order{ClientOrderID: id, AccountID: account, Status: "unknown"}
	if !fresh {
		if previous == nil {
			previous = &unknown
		}
		respond(w, 202, previous)
		return
	}
	o, err := invoke()
	if err != nil || validateOrder(o, account, id) != nil {
		o = unknown
		// A cancel rejection does not mean the ORIGINAL ORDER was rejected.
		// Leave ambiguous/failed cancellations unknown and query the order.
		if errors.Is(err, ErrRejected) && len(key) >= 7 && key[:7] == "submit/" {
			o.Status = "rejected"
		}
	}
	// Persist even when the caller disconnected; an interrupted response must
	// never cause another order. Reservation alone also blocks re-submission.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = s.journal.finish(ctx, key, o); err != nil {
		respond(w, 202, unknown)
		return
	}
	respond(w, 202, o)
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code string) {
	respond(w, status, map[string]string{"error": code})
}
func brokerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		fail(w, 404, "not_found")
	case errors.Is(err, ErrUnsupported):
		fail(w, 501, "unsupported")
	default:
		fail(w, 502, "broker_unavailable")
	}
}
