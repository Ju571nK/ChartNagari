package alpaca

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
	"github.com/Ju571nK/Chatter/pkg/brokerplugin/conformance"
	"github.com/rs/zerolog"
)

func TestAlpacaProtocolIsReadOnlyAndMapped(t *testing.T) {
	writes := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("APCA-API-KEY-ID") != "mock-key" || r.Header.Get("APCA-API-SECRET-KEY") != "mock-secret" {
			t.Error("missing credentials")
		}
		if r.Method != "GET" {
			writes++
			http.Error(w, "must not write", 500)
			return
		}
		switch r.URL.Path {
		case "/v2/account":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "account-1", "currency": "USD", "buying_power": "1234.50"})
		case "/v2/orders":
			if r.URL.Query().Get("status") != "all" || r.URL.Query().Get("limit") != "100" {
				t.Error("missing snapshot bounds")
			}
			_ = json.NewEncoder(w).Encode([]OrderResponse{{ID: "broker-1", ClientOrderID: "client-1", Status: "partially_filled", Qty: "2", FilledQty: "1", Symbol: "AAPL"}})
		case "/v2/orders:by_client_order_id":
			if r.URL.Query().Get("client_order_id") != "client-1" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(OrderResponse{ID: "broker-1", ClientOrderID: "client-1", Status: "filled", Qty: "2", FilledQty: "2", Symbol: "AAPL"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	cfg := Config{AlpacaAPIURL: upstream.URL, AlpacaAPIKey: "mock-key", AlpacaAPISecret: "mock-secret", FeedbackURL: "http://127.0.0.1:1/api/execution/feedback", PluginSecret: "mock-shared", PluginID: "alpaca-paper", ListenAddr: "127.0.0.1:0", NotionalPerTrade: 1000, DBPath: filepath.Join(t.TempDir(), "legacy.db")}
	r, err := NewRunner(cfg, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer r.store.Close()
	defer r.protocolJournal.Close()
	s := httptest.NewServer(r.server.Handler)
	defer s.Close()
	c, err := plugin.NewClient(s.URL, cfg.PluginID, cfg.PluginSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conformance.Inspect(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	var page plugin.OrderPage
	if _, err := c.Do(context.Background(), "GET", "/v1/orders?account_id=account-1", nil, &page); err != nil || page.Complete || len(page.Orders) != 1 || page.Orders[0].Status != "partial_fill" {
		t.Fatalf("snapshot: %+v %v", page, err)
	}
	var o plugin.Order
	if _, err := c.Do(context.Background(), "GET", "/v1/orders/client-1?account_id=account-1", nil, &o); err != nil || o.Status != "filled" {
		t.Fatalf("lookup: %+v %v", o, err)
	}
	for _, path := range []string{"/v1/orders", "/v1/orders/client-1/cancel"} {
		if status, _ := c.Do(context.Background(), "POST", path, map[string]string{}, nil); status != 423 {
			t.Fatal("v1 write enabled")
		}
	}
	if status, _ := c.Do(context.Background(), "GET", "/v1/orders?account_id=foreign", nil, nil); status != 404 {
		t.Fatal("foreign account accepted")
	}
	if writes != 0 {
		t.Fatal("v1 bypassed legacy execution pipeline")
	}
}

func TestAlpacaUnknownStatesAreNotReportedFilled(t *testing.T) {
	for _, status := range []string{"pending_replace", "replaced", "suspended", "done_for_day", "future-state"} {
		if o := protocolOrder(OrderResponse{Status: status}, "a"); o.Status != "unknown" {
			t.Fatal(status)
		}
	}
}

func TestAlpacaDoesNotForwardCredentialsOnRedirect(t *testing.T) {
	calls := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer other.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 307) }))
	defer s.Close()
	c := NewAlpacaClient(s.URL, "mock-key", "mock-secret")
	var output any
	if err := c.readJSON(context.Background(), "/v2/account", &output); err == nil || calls != 0 {
		t.Fatal("followed read redirect")
	}
	if _, err := c.SubmitOrder(context.Background(), OrderRequest{Symbol: "AAPL", Qty: "1", Side: "buy"}); err == nil || calls != 0 {
		t.Fatal("followed order redirect")
	}
}
