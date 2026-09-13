package brokerplugin_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
	"github.com/Ju571nK/Chatter/pkg/brokerplugin/conformance"
	"github.com/Ju571nK/Chatter/pkg/brokerplugin/simulator"
)

const secret = "test-only-secret-never-a-real-credential"

func setup(t *testing.T, enabled bool) (*plugin.Client, *simulator.Broker, *plugin.Server) {
	t.Helper()
	dir := t.TempDir()
	b, err := simulator.Open("test-plugin", filepath.Join(dir, "sim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	j, err := plugin.OpenJournal(filepath.Join(dir, "journal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	h, err := plugin.NewServer(plugin.ServerConfig{PluginID: "test-plugin", Secret: secret, OrdersEnabled: enabled}, b, j)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c, err := plugin.NewClient(s.URL, "test-plugin", secret)
	if err != nil {
		t.Fatal(err)
	}
	return c, b, h
}
func request(id, symbol string) plugin.OrderRequest {
	return plugin.OrderRequest{ClientOrderID: id, AccountID: "sim-account", Mode: "paper", Symbol: symbol, AssetClass: "stock", Side: "buy", Type: "market", TimeInForce: "day", Quantity: "2"}
}
func post(t *testing.T, c *plugin.Client, req plugin.OrderRequest) plugin.Order {
	t.Helper()
	var o plugin.Order
	status, err := c.Do(context.Background(), "POST", "/v1/orders", req, &o)
	if err != nil || status != 202 {
		t.Fatalf("post: %d %v", status, err)
	}
	return o
}

func TestSimulatorConformance(t *testing.T) {
	c, b, _ := setup(t, true)
	report, err := conformance.Inspect(context.Background(), c)
	if err != nil || !report.ReadOnly || report.Accounts != 1 || b.SubmissionCount() != 0 {
		t.Fatalf("read-only inspection: %+v %v", report, err)
	}
	for _, tc := range []struct{ symbol, status string }{{"SIM-FILL", "filled"}, {"SIM-PARTIAL", "partial_fill"}, {"SIM-PENDING", "submitted"}, {"SIM-REJECT", "rejected"}, {"SIM-TIMEOUT", "unknown"}} {
		t.Run(tc.symbol, func(t *testing.T) {
			r := request(tc.symbol, tc.symbol)
			first := post(t, c, r)
			if first.Status != tc.status {
				t.Fatalf("%+v", first)
			}
			before := b.SubmissionCount()
			again := post(t, c, r)
			if again != first || b.SubmissionCount() != before {
				t.Fatal("replayed submission")
			}
			var current plugin.Order
			_, err := c.Do(context.Background(), "GET", "/v1/orders/"+r.ClientOrderID+"?account_id=sim-account", nil, &current)
			if err != nil {
				t.Fatal(err)
			}
			if tc.symbol == "SIM-TIMEOUT" && current.Status != "filled" {
				t.Fatal("lost response was not reconciled")
			}
			if tc.symbol == "SIM-PARTIAL" && current.FilledQuantity != "1.0" {
				t.Fatal("partial quantity lost")
			}
			r.Quantity = "4"
			if status, _ := c.Do(context.Background(), "POST", "/v1/orders", r, nil); status != 409 {
				t.Fatalf("changed payload got %d", status)
			}
		})
	}
}

func TestConcurrentDuplicateIsNeverResubmitted(t *testing.T) {
	c, b, _ := setup(t, true)
	r := request("parallel", "SIM-FILL")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var o plugin.Order
			if _, err := c.Do(context.Background(), "POST", "/v1/orders", r, &o); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if b.SubmissionCount() != 1 {
		t.Fatalf("submitted %d times", b.SubmissionCount())
	}
}

func TestRestartPreservesUnknownAndBrokerReconciliation(t *testing.T) {
	dir := t.TempDir()
	state, journal := filepath.Join(dir, "sim.db"), filepath.Join(dir, "journal.db")
	start := func() (*plugin.Client, *simulator.Broker, func()) {
		b, err := simulator.Open("test-plugin", state)
		if err != nil {
			t.Fatal(err)
		}
		j, err := plugin.OpenJournal(journal)
		if err != nil {
			t.Fatal(err)
		}
		h, err := plugin.NewServer(plugin.ServerConfig{PluginID: "test-plugin", Secret: secret, OrdersEnabled: true}, b, j)
		if err != nil {
			t.Fatal(err)
		}
		s := httptest.NewServer(h)
		c, err := plugin.NewClient(s.URL, "test-plugin", secret)
		if err != nil {
			t.Fatal(err)
		}
		return c, b, func() { s.Close(); _ = j.Close(); _ = b.Close() }
	}
	c, _, closeFirst := start()
	r := request("restart", "SIM-TIMEOUT")
	if post(t, c, r).Status != "unknown" {
		t.Fatal("expected unknown")
	}
	closeFirst()
	c, b, closeSecond := start()
	defer closeSecond()
	if post(t, c, r).Status != "unknown" || b.SubmissionCount() != 0 {
		t.Fatal("restart retried ambiguous order")
	}
	var o plugin.Order
	if _, err := c.Do(context.Background(), "GET", "/v1/orders/restart?account_id=sim-account", nil, &o); err != nil || o.Status != "filled" {
		t.Fatalf("reconcile: %+v %v", o, err)
	}
	info, err := os.Stat(journal)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("journal is not private")
	}
}

func TestDisabledAndInvalidOrdersDoNotReachBroker(t *testing.T) {
	c, b, _ := setup(t, false)
	if status, _ := c.Do(context.Background(), "POST", "/v1/orders", request("disabled", "SIM-FILL"), nil); status != 423 {
		t.Fatal(status)
	}
	if b.SubmissionCount() != 0 {
		t.Fatal("disabled orders reached broker")
	}
	c, b, _ = setup(t, true)
	for name, change := range map[string]func(*plugin.OrderRequest){
		"live": func(r *plugin.OrderRequest) { r.Mode = "live" }, "fraction": func(r *plugin.OrderRequest) { r.Quantity = "0.5" }, "zero": func(r *plugin.OrderRequest) { r.Quantity = "0" }, "exponent": func(r *plugin.OrderRequest) { r.Quantity = "1e2" }, "negative": func(r *plugin.OrderRequest) { r.Quantity = "-2" }, "limit": func(r *plugin.OrderRequest) { r.Type = "limit" }, "account": func(r *plugin.OrderRequest) { r.AccountID = "another-account" }, "id": func(r *plugin.OrderRequest) { r.ClientOrderID = "../escape" }, "crypto": func(r *plugin.OrderRequest) { r.AssetClass = "crypto" },
	} {
		t.Run(name, func(t *testing.T) {
			r := request(name, "SIM-FILL")
			change(&r)
			status, _ := c.Do(context.Background(), "POST", "/v1/orders", r, nil)
			if status != 422 && status != 404 {
				t.Fatal(status)
			}
		})
	}
	if status, _ := c.Do(context.Background(), "POST", "/v1/orders", map[string]any{"unexpected": "field"}, nil); status != 400 {
		t.Fatal(status)
	}
	if b.SubmissionCount() != 0 {
		t.Fatal("invalid order reached broker")
	}
}

func TestCancellationPreservesPartialFillAndIdempotency(t *testing.T) {
	c, _, _ := setup(t, true)
	post(t, c, request("partial", "SIM-PARTIAL"))
	req := plugin.CancelRequest{RequestID: "cancel-1", AccountID: "sim-account"}
	var o plugin.Order
	if _, err := c.Do(context.Background(), "POST", "/v1/orders/partial/cancel", req, &o); err != nil || o.Status != "cancelled" || o.FilledQuantity != "1.0" {
		t.Fatalf("cancel: %+v %v", o, err)
	}
	var again plugin.Order
	if _, err := c.Do(context.Background(), "POST", "/v1/orders/partial/cancel", req, &again); err != nil || again != o {
		t.Fatal("duplicate cancel changed result")
	}
	if status, _ := c.Do(context.Background(), "POST", "/v1/orders/different/cancel", req, nil); status != 409 {
		t.Fatal("cancel id reused for another order")
	}
	post(t, c, request("filled", "SIM-FILL"))
	req.RequestID = "cancel-filled"
	if _, err := c.Do(context.Background(), "POST", "/v1/orders/filled/cancel", req, &o); err != nil || o.Status != "unknown" {
		t.Fatal("cancel rejection incorrectly changed order status")
	}
}

func signedRequest(method, target string, body []byte, ts int64) *http.Request {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	r.Header.Set(plugin.PluginIDHeader, "test-plugin")
	r.Header.Set(plugin.TimestampHeader, strconv.FormatInt(ts, 10))
	r.Header.Set(plugin.SignatureHeader, plugin.Sign(secret, "test-plugin", ts, method, r.URL.RequestURI(), body))
	return r
}
func TestAuthenticationAndRequestBounds(t *testing.T) {
	_, b, h := setup(t, true)
	for name, makeRequest := range map[string]func() *http.Request{
		"no auth": func() *http.Request { return httptest.NewRequest("GET", "/v1/manifest", nil) },
		"expired": func() *http.Request {
			return signedRequest("GET", "/v1/manifest", nil, time.Now().Add(-10*time.Minute).Unix())
		},
		"timestamp overflow": func() *http.Request { return signedRequest("GET", "/v1/manifest", nil, -9223372036854775808) },
		"query tamper": func() *http.Request {
			r := signedRequest("GET", "/v1/orders?account_id=sim-account", nil, time.Now().Unix())
			r.URL.RawQuery = "account_id=other"
			return r
		},
		"plugin tamper": func() *http.Request {
			r := signedRequest("GET", "/v1/manifest", nil, time.Now().Unix())
			r.Header.Set(plugin.PluginIDHeader, "other")
			return r
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, makeRequest())
			if w.Code != 401 {
				t.Fatal(w.Code)
			}
		})
	}
	for _, tc := range []struct {
		body string
		code int
	}{{strings.Repeat("x", (1<<20)+1), 413}, {`{} {}`, 400}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest("POST", "/v1/orders", []byte(tc.body), time.Now().Unix()))
		if w.Code != tc.code {
			t.Fatalf("got %d want %d", w.Code, tc.code)
		}
	}
	if b.SubmissionCount() != 0 {
		t.Fatal("unauthenticated request submitted")
	}
}

func TestClientRejectsUnsafeOriginsAndRedirects(t *testing.T) {
	for _, origin := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com/path", "https://example.com?q=1", "file:///tmp/x"} {
		if _, err := plugin.NewClient(origin, "test-plugin", secret); err == nil {
			t.Fatal(origin)
		}
	}
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer s.Close()
	c, _ := plugin.NewClient(s.URL, "test-plugin", secret)
	if status, err := c.Do(context.Background(), "GET", "/v1/manifest", nil, nil); err == nil || status != 307 || targetCalls != 0 {
		t.Fatalf("redirect: %d %v calls=%d", status, err, targetCalls)
	}
}

func TestManifestDoesNotExposeSecrets(t *testing.T) {
	c, _, _ := setup(t, false)
	var m plugin.Manifest
	if _, err := c.Do(context.Background(), "GET", "/v1/manifest", nil, &m); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%+v", m), secret) || m.OrdersEnabled {
		t.Fatal("unsafe manifest")
	}
}
