package rakutenfx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
	"github.com/Ju571nK/Chatter/pkg/brokerplugin/conformance"
)

const (
	testToken  = "bridge-token-0123456789abcdef0123456789"
	testSecret = "plugin-secret-0123456789abcdef0123456789"
	validSnap  = `{"login":"12345","server":"RakutenSecurities-Demo","trade_mode":"demo","currency":"JPY","free_margin":"1000000.00",
"orders":[{"ticket":"1001","symbol":"USDJPY","status":"filled","units":"10000"},{"ticket":"1002","symbol":"EURJPY","status":"submitted","units":"1000"}]}`
)

func push(t *testing.T, b *Bridge, method, path, token, body string) int {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set(TokenHeader, token)
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w.Code
}

func newBridge(t *testing.T) *Bridge {
	t.Helper()
	b, err := New("rakuten-fx-demo", testToken, "")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSnapshotIngest(t *testing.T) {
	tests := []struct {
		name, method, path, token, body string
		want                            int
	}{
		{"valid demo", "POST", SnapshotPath, testToken, validSnap, 204},
		{"wrong token", "POST", SnapshotPath, "nope", validSnap, 401},
		{"wrong route", "POST", "/bridge/v1/other", testToken, validSnap, 404},
		{"get", "GET", SnapshotPath, testToken, "", 404},
		{"live account", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"demo"`, `"live"`, 1), 403},
		{"live server", "POST", SnapshotPath, testToken, strings.Replace(validSnap, "RakutenSecurities-Demo", "RakutenSecurities-Live", 1), 403},
		{"unknown field", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"login"`, `"extra":1,"login"`, 1), 400},
		{"zero units", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"10000"`, `"0"`, 1), 400},
		{"unknown status", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"submitted"`, `"open"`, 1), 400},
		{"bad ticket", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"1001"`, `"../x"`, 1), 400},
		{"duplicate ticket", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"1002"`, `"1001"`, 1), 400},
		{"bad symbol", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"USDJPY"`, `"USD JPY"`, 1), 400},
		{"bad margin", "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"1000000.00"`, `"1e6"`, 1), 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := push(t, newBridge(t), tt.method, tt.path, tt.token, tt.body); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLiveSnapshotDropsDemoData(t *testing.T) {
	b := newBridge(t)
	push(t, b, "POST", SnapshotPath, testToken, validSnap)
	push(t, b, "POST", SnapshotPath, testToken, strings.Replace(validSnap, `"demo"`, `"live"`, 1))
	if _, err := b.Accounts(context.Background()); err == nil {
		t.Fatal("accounts still served after a live snapshot")
	}
}

// Exercises the bridge through the real v1 server: signed reads work, writes stay closed.
func TestReadOnlyProtocol(t *testing.T) {
	b := newBridge(t)
	j, err := plugin.OpenJournal(filepath.Join(t.TempDir(), "journal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	api, err := plugin.NewServer(plugin.ServerConfig{PluginID: "rakuten-fx-demo", Secret: testSecret}, b, j)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c, err := plugin.NewClient(srv.URL, "rakuten-fx-demo", testSecret)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if code, _ := c.Do(ctx, "GET", "/v1/accounts", nil, nil); code != http.StatusBadGateway {
		t.Fatalf("accounts before first snapshot = %d, want 502", code)
	}
	if code := push(t, b, "POST", SnapshotPath, testToken, validSnap); code != 204 {
		t.Fatalf("snapshot = %d", code)
	}
	report, err := conformance.Inspect(ctx, c)
	if err != nil || report.Accounts != 1 {
		t.Fatalf("inspect = %+v, %v", report, err)
	}
	var o plugin.Order
	if _, err = c.Do(ctx, "GET", "/v1/orders/mt4-1001?account_id=mt4-12345", nil, &o); err != nil || o.Status != "filled" || o.FilledQuantity != "10000" {
		t.Fatalf("order = %+v, %v", o, err)
	}
	req := plugin.OrderRequest{ClientOrderID: "x1", AccountID: "mt4-12345", Mode: "paper", Symbol: "USDJPY", AssetClass: "stock", Side: "buy", Type: "market", TimeInForce: "day", Quantity: "1000"}
	if code, _ := c.Do(ctx, "POST", "/v1/orders", req, nil); code != http.StatusLocked {
		t.Fatalf("submit = %d, want 423", code)
	}

	b.now = func() time.Time { return time.Now().Add(2 * maxSnapshotAge) }
	if code, _ := c.Do(ctx, "GET", "/v1/accounts", nil, nil); code != http.StatusBadGateway {
		t.Fatalf("stale accounts = %d, want 502", code)
	}
}
