package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ju571nK/Chatter/internal/storage"
)

func TestSymbolMessageTemplateAPI(t *testing.T) {
	s := setupTest(t)
	db, err := storage.New(filepath.Join(t.TempDir(), "templates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.WithMessageTemplateStore(storage.NewSymbolMessageTemplateStore(db))
	s.WithOverrideStore(storage.NewSymbolOverrideStore(db))
	s.apiToken = "secret"
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		return w
	}
	path := "/api/symbol-message-templates/EURUSD"
	if got := call("GET", path, "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("GET auth: %d", got.Code)
	}
	if got := call("PUT", path, `{"long":"x","short":""}`, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("PUT auth: %d", got.Code)
	}
	if got := call("POST", path+"/preview", `{"direction":"LONG","template":"x"}`, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("preview auth: %d", got.Code)
	}
	if got := call("GET", path, "", "secret"); got.Code != 200 || !strings.Contains(got.Body.String(), `"long":""`) {
		t.Fatalf("empty GET: %d %s", got.Code, got.Body.String())
	}
	for _, invalid := range []string{
		`{"long":"{unknown}","short":""}`,
		`{"long":"{symbol","short":""}`,
		`{"long":"x"}`,
		`{"long":"x","short":"","extra":1}`,
	} {
		if got := call("PUT", path, invalid, "secret"); got.Code != 400 || !strings.Contains(got.Body.String(), "error") {
			t.Fatalf("validation %s: %d %s", invalid, got.Code, got.Body.String())
		}
	}
	if got := call("PUT", path, `{"long":"매수 {symbol}","short":"Sell {종목}"}`, "secret"); got.Code != 200 {
		t.Fatalf("PUT: %d %s", got.Code, got.Body.String())
	}
	if got := call("GET", "/api/symbol-message-templates/USDJPY", "", "secret"); got.Code != 200 || strings.Contains(got.Body.String(), "매수") {
		t.Fatalf("symbol leak: %s", got.Body.String())
	}
	if got := call("PUT", "/api/symbol-overrides/EURUSD", `{"score_threshold":14}`, "secret"); got.Code != 200 {
		t.Fatalf("legacy override PUT: %d %s", got.Code, got.Body.String())
	}
	if got := call("DELETE", "/api/symbol-overrides/EURUSD", "", "secret"); got.Code != 200 {
		t.Fatalf("legacy override DELETE: %d %s", got.Code, got.Body.String())
	}
	if got := call("GET", path, "", "secret"); !strings.Contains(got.Body.String(), "매수") {
		t.Fatalf("legacy API erased template: %s", got.Body.String())
	}
	preview := call("POST", path+"/preview", `{"direction":"LONG","template":"<b>{symbol}&"}`, "secret")
	if preview.Code != 200 {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
	}
	var value struct {
		HTML     string `json:"html"`
		Included bool   `json:"custom_included"`
		Sample   string `json:"sample"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if !value.Included || !strings.Contains(value.HTML, "&lt;b&gt;EURUSD&amp;") || !strings.Contains(value.Sample, "EURUSD · 1H") {
		t.Fatalf("preview: %+v", value)
	}
	if got := call("PUT", path, `{"long":"","short":""}`, "secret"); got.Code != 200 {
		t.Fatalf("reset: %s", got.Body.String())
	}
	if got := call("GET", path, "", "secret"); strings.Contains(got.Body.String(), "매수") {
		t.Fatalf("reset failed: %s", got.Body.String())
	}
}

func TestSymbolMessageTemplatePreviewUsesForexSample(t *testing.T) {
	s := setupTest(t)
	if err := os.WriteFile(filepath.Join(s.configDir, "watchlist.yaml"), []byte("symbols:\n  forex:\n    - symbol: USDJPY\n      exchange: FX\n      enabled: true\n    - symbol: JPYUSD\n      exchange: FX\n      enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := storage.New(filepath.Join(t.TempDir(), "preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.WithMessageTemplateStore(storage.NewSymbolMessageTemplateStore(db))
	req := httptest.NewRequest("POST", "/api/symbol-message-templates/USDJPY/preview", bytes.NewBufferString(`{"direction":"LONG","template":"{entry} {tp} {sl}"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var value struct {
		HTML   string `json:"html"`
		Sample string `json:"sample"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(value.Sample, "150.000") || !strings.Contains(value.HTML, "pips") {
		t.Fatalf("FX preview: %+v", value)
	}
	for _, tt := range []struct{ symbol, entry, tp, sl string }{
		{"USDJPY", "150.000", "149.000", "151.000"},
		{"JPYUSD", "0.00670", "0.00660", "0.00680"},
	} {
		req := httptest.NewRequest("POST", "/api/symbol-message-templates/"+tt.symbol+"/preview", bytes.NewBufferString(`{"direction":"SHORT","template":"{entry} {tp} {sl}"}`))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("SHORT preview %s: %d %s", tt.symbol, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(value.Sample, "entry "+tt.entry) || !strings.Contains(value.Sample, "TP "+tt.tp) || !strings.Contains(value.Sample, "SL "+tt.sl) {
			t.Fatalf("SHORT sample %s: %+v", tt.symbol, value)
		}
	}
}
