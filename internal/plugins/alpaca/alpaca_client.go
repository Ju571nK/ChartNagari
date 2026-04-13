package alpaca

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AlpacaClient is the minimal REST client we need for Phase 3.
//
// Endpoints used:
//   - POST /v2/orders          — submit a market order
//
// We do NOT use the official Alpaca Go SDK: a tiny hand-rolled client keeps the
// binary's dependency surface small and the behavior fully auditable (critical
// for anything that touches real money, even in paper mode).
type AlpacaClient struct {
	baseURL   string
	apiKey    string
	apiSecret string
	http      *http.Client
}

// NewAlpacaClient builds a client against the paper API URL and credentials.
func NewAlpacaClient(baseURL, apiKey, apiSecret string) *AlpacaClient {
	return &AlpacaClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		apiSecret: apiSecret,
		http:      &http.Client{Timeout: HTTPTimeout},
	}
}

// OrderRequest is the subset of Alpaca's /v2/orders payload we populate.
// Reference: https://docs.alpaca.markets/reference/postorder
type OrderRequest struct {
	Symbol        string `json:"symbol"`
	Qty           string `json:"qty"`                      // decimal string
	Side          string `json:"side"`                     // "buy" | "sell"
	Type          string `json:"type"`                     // "market"
	TimeInForce   string `json:"time_in_force"`            // "day"
	ClientOrderID string `json:"client_order_id,omitempty"` // we set = signal_id for observability
}

// OrderResponse is the subset we care about from Alpaca's response.
type OrderResponse struct {
	ID            string `json:"id"`
	ClientOrderID string `json:"client_order_id"`
	Status        string `json:"status"` // accepted | pending_new | new | filled | rejected | canceled
	Symbol        string `json:"symbol"`
	Qty           string `json:"qty"`
	Side          string `json:"side"`
	Type          string `json:"type"`
}

// AlpacaError is returned when Alpaca responds non-2xx. StatusCode lets callers
// map back to the webhook response code (4xx → 422, 5xx → 502).
type AlpacaError struct {
	StatusCode int
	Code       int    `json:"code"`
	Message    string `json:"message"`
}

// Error satisfies the error interface.
func (e *AlpacaError) Error() string {
	return fmt.Sprintf("alpaca: http %d: %s (code=%d)", e.StatusCode, e.Message, e.Code)
}

// IsClientError reports true when Alpaca returned a 4xx.
func (e *AlpacaError) IsClientError() bool {
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// IsServerError reports true when Alpaca returned a 5xx.
func (e *AlpacaError) IsServerError() bool {
	return e.StatusCode >= 500
}

// SubmitOrder POSTs /v2/orders and returns the parsed response. Authentication
// uses Alpaca's APCA-API-KEY-ID / APCA-API-SECRET-KEY header pair.
func (c *AlpacaClient) SubmitOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	if req.Symbol == "" || req.Qty == "" || req.Side == "" {
		return nil, errors.New("alpaca: symbol/qty/side required")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal order: %w", err)
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v2/orders", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json")
	hreq.Header.Set("APCA-API-KEY-ID", c.apiKey)
	hreq.Header.Set("APCA-API-SECRET-KEY", c.apiSecret)

	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("alpaca do: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read alpaca response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		ae := &AlpacaError{StatusCode: resp.StatusCode}
		// Alpaca error shape: {"code": 40310000, "message": "..."}.
		// We attempt to parse and fall back to the raw body.
		if jerr := json.Unmarshal(respBody, ae); jerr != nil || ae.Message == "" {
			ae.Message = strings.TrimSpace(string(respBody))
		}
		return nil, ae
	}

	var out OrderResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}
	return &out, nil
}

