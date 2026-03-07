package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ── test fixtures ─────────────────────────────────────────────────────────────

const testWatchlist = `symbols:
  crypto:
    - symbol: BTCUSDT
      exchange: binance
      enabled: true
    - symbol: ETHUSDT
      exchange: binance
      enabled: false
  stocks:
    - symbol: AAPL
      exchange: nasdaq
      enabled: false
timeframes: [1H, 4H, 1D, 1W]
`

const testRules = `rules:
  - name: rsi_overbought_oversold
    enabled: true
    methodology: general_ta
    params:
      strength: 3.0
  - name: ema_cross
    enabled: false
    methodology: general_ta
    params:
      strength: 2.5
scoring:
  mtf_bonus: 3.0
  thresholds:
    weak: 5.0
    medium: 8.0
    strong: 12.0
timeframe_weights:
  "1W": 2.0
  "1D": 1.5
  "4H": 1.2
  "1H": 1.0
`

// setupTest creates a temporary config directory with minimal YAML fixtures.
func setupTest(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "watchlist.yaml"), []byte(testWatchlist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte(testRules), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(dir, "") // no static serving
}

// do executes an HTTP request against the server handler and returns the recorder.
func do(t *testing.T, srv *Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf.Write(data)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

// ── tests ─────────────────────────────────────────────────────────────────────

// Test 1: GET /api/status returns 200 with a non-empty phase string.
func TestGetStatus_OK(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var s StatusItem
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, w.Body)
	}
	if s.Phase == "" {
		t.Error("phase must not be empty")
	}
}

// Test 2: GET /api/status counts total symbols correctly.
func TestGetStatus_SymbolCount(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/status", nil)
	var s StatusItem
	json.Unmarshal(w.Body.Bytes(), &s) //nolint:errcheck
	if s.Symbols != 3 {                // BTCUSDT + ETHUSDT + AAPL
		t.Errorf("want 3 symbols, got %d", s.Symbols)
	}
	if s.Rules != 2 {
		t.Errorf("want 2 rules, got %d", s.Rules)
	}
}

// Test 3: GET /api/symbols returns all symbols (crypto + stocks).
func TestGetSymbols_ReturnsAll(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/symbols", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var items []SymbolItem
	json.Unmarshal(w.Body.Bytes(), &items) //nolint:errcheck
	if len(items) != 3 {
		t.Fatalf("want 3 symbols, got %d", len(items))
	}
}

// Test 4: GET /api/symbols includes crypto symbols with correct type.
func TestGetSymbols_IncludesCrypto(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/symbols", nil)
	var items []SymbolItem
	json.Unmarshal(w.Body.Bytes(), &items) //nolint:errcheck

	found := false
	for _, s := range items {
		if s.Symbol == "BTCUSDT" && s.Type == "crypto" && s.Enabled {
			found = true
		}
	}
	if !found {
		t.Error("BTCUSDT (enabled crypto) not found in response")
	}
}

// Test 5: GET /api/symbols includes stock symbols.
func TestGetSymbols_IncludesStocks(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/symbols", nil)
	var items []SymbolItem
	json.Unmarshal(w.Body.Bytes(), &items) //nolint:errcheck

	found := false
	for _, s := range items {
		if s.Symbol == "AAPL" && s.Type == "stock" {
			found = true
		}
	}
	if !found {
		t.Error("AAPL (stock) not found in response")
	}
}

// Test 6: PUT /api/symbols/{symbol} with enabled=true returns 204 and persists the change.
func TestUpdateSymbol_Enable(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "PUT", "/api/symbols/ETHUSDT", map[string]bool{"enabled": true})
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d\nbody: %s", w.Code, w.Body)
	}

	// Verify persistence via a subsequent GET.
	w2 := do(t, srv, "GET", "/api/symbols", nil)
	var items []SymbolItem
	json.Unmarshal(w2.Body.Bytes(), &items) //nolint:errcheck
	for _, s := range items {
		if s.Symbol == "ETHUSDT" && !s.Enabled {
			t.Error("ETHUSDT should be enabled after PUT")
		}
	}
}

// Test 7: PUT /api/symbols/{symbol} with enabled=false disables the symbol.
func TestUpdateSymbol_Disable(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "PUT", "/api/symbols/BTCUSDT", map[string]bool{"enabled": false})
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d\nbody: %s", w.Code, w.Body)
	}
}

// Test 8: PUT /api/symbols/{unknown} returns 404.
func TestUpdateSymbol_NotFound(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "PUT", "/api/symbols/UNKNOWN", map[string]bool{"enabled": true})
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// Test 9: PUT /api/symbols with malformed JSON returns 400.
func TestUpdateSymbol_BadJSON(t *testing.T) {
	srv := setupTest(t)
	req := httptest.NewRequest("PUT", "/api/symbols/BTCUSDT", bytes.NewBufferString("bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// Test 10: GET /api/rules returns all rules with their enabled flags.
func TestGetRules_ReturnsAll(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/rules", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var items []RuleItem
	json.Unmarshal(w.Body.Bytes(), &items) //nolint:errcheck
	if len(items) != 2 {
		t.Fatalf("want 2 rules, got %d", len(items))
	}
}

// Test 11: GET /api/rules preserves the enabled flag for each rule.
func TestGetRules_EnabledFlag(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/rules", nil)
	var items []RuleItem
	json.Unmarshal(w.Body.Bytes(), &items) //nolint:errcheck

	for _, r := range items {
		switch r.Name {
		case "rsi_overbought_oversold":
			if !r.Enabled {
				t.Error("rsi_overbought_oversold should be enabled")
			}
		case "ema_cross":
			if r.Enabled {
				t.Error("ema_cross should be disabled")
			}
		}
	}
}

// Test 12: PUT /api/rules/{name} enables a rule and persists the change.
func TestUpdateRule_Enable(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "PUT", "/api/rules/ema_cross", map[string]bool{"enabled": true})
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d\nbody: %s", w.Code, w.Body)
	}

	w2 := do(t, srv, "GET", "/api/rules", nil)
	var items []RuleItem
	json.Unmarshal(w2.Body.Bytes(), &items) //nolint:errcheck
	for _, r := range items {
		if r.Name == "ema_cross" && !r.Enabled {
			t.Error("ema_cross should be enabled after PUT")
		}
	}
}

// Test 13: PUT /api/rules/{name} disables a rule.
func TestUpdateRule_Disable(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "PUT", "/api/rules/rsi_overbought_oversold", map[string]bool{"enabled": false})
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d\nbody: %s", w.Code, w.Body)
	}
}

// Test 14: PUT /api/rules/{unknown} returns 404.
func TestUpdateRule_NotFound(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "PUT", "/api/rules/no_such_rule", map[string]bool{"enabled": true})
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// Test 15: CORS header is present on every response.
func TestCORSHeaders_Present(t *testing.T) {
	srv := setupTest(t)
	w := do(t, srv, "GET", "/api/status", nil)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin: want \"*\", got %q", got)
	}
}

// Test 16: OPTIONS preflight returns 204 with no body.
func TestOptions_Preflight(t *testing.T) {
	srv := setupTest(t)
	req := httptest.NewRequest("OPTIONS", "/api/symbols", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 for OPTIONS, got %d", w.Code)
	}
}
