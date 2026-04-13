package alpaca

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlpacaClient_SubmitOrder_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/orders" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("APCA-API-KEY-ID") != "key" || r.Header.Get("APCA-API-SECRET-KEY") != "secret" {
			t.Errorf("missing auth headers: %v", r.Header)
		}
		var in OrderRequest
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &in); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if in.Symbol != "AAPL" || in.Qty != "5" || in.Side != "buy" {
			t.Errorf("unexpected order payload: %+v", in)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(OrderResponse{
			ID: "ord-xyz", ClientOrderID: in.ClientOrderID, Status: "accepted",
			Symbol: "AAPL", Qty: "5", Side: "buy", Type: "market",
		})
	}))
	defer srv.Close()

	client := NewAlpacaClient(srv.URL, "key", "secret")
	resp, err := client.SubmitOrder(context.Background(), OrderRequest{
		Symbol: "AAPL", Qty: "5", Side: "buy", Type: "market", TimeInForce: "day", ClientOrderID: "c-1",
	})
	if err != nil {
		t.Fatalf("SubmitOrder: %v", err)
	}
	if resp.ID != "ord-xyz" || resp.Status != "accepted" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestAlpacaClient_SubmitOrder_4xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"code":42210000,"message":"asset not shortable"}`))
	}))
	defer srv.Close()

	client := NewAlpacaClient(srv.URL, "k", "s")
	_, err := client.SubmitOrder(context.Background(), OrderRequest{Symbol: "X", Qty: "1", Side: "sell"})
	var ae *AlpacaError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AlpacaError, got %v", err)
	}
	if !ae.IsClientError() || ae.IsServerError() {
		t.Errorf("classification wrong for 422: %+v", ae)
	}
	if !strings.Contains(ae.Message, "shortable") {
		t.Errorf("message lost: %q", ae.Message)
	}
	if ae.Code != 42210000 {
		t.Errorf("code = %d", ae.Code)
	}
}

func TestAlpacaClient_SubmitOrder_5xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	client := NewAlpacaClient(srv.URL, "k", "s")
	_, err := client.SubmitOrder(context.Background(), OrderRequest{Symbol: "X", Qty: "1", Side: "buy"})
	var ae *AlpacaError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AlpacaError, got %v", err)
	}
	if !ae.IsServerError() {
		t.Errorf("expected server-error classification, got %+v", ae)
	}
	if !strings.Contains(ae.Message, "upstream") {
		t.Errorf("fallback message lost: %q", ae.Message)
	}
}

func TestAlpacaClient_SubmitOrder_ValidatesInput(t *testing.T) {
	t.Parallel()
	client := NewAlpacaClient("http://127.0.0.1:1", "k", "s")
	if _, err := client.SubmitOrder(context.Background(), OrderRequest{}); err == nil {
		t.Fatal("expected validation error")
	}
}
