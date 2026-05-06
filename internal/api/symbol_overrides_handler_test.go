package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
)

// newTestServer builds a minimal Server wired with a real DB + override store.
// apiToken=="" disables auth.
func newTestServer(t *testing.T, apiToken string) (*Server, *storage.SymbolOverrideStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := storage.NewSymbolOverrideStore(db)
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "p1",
		Profiles: map[string]appconfig.Profile{
			"p1": {ScoreThreshold: 10, CooldownHours: 6, AlertLimitPerDay: 3,
				Timeframes: []string{"1D"}, AllowedRules: []string{"rsi_overbought_oversold"}},
		},
	})

	s := &Server{
		apiToken:       apiToken,
		profileHolder:  holder,
		overrideStore:  store,
		validRuleNames: map[string]struct{}{"rsi_overbought_oversold": {}, "ict_order_block": {}},
	}
	return s, store
}

func TestGetSymbolOverride_Empty(t *testing.T) {
	// GET with no override stored must return the merged effective shape
	// ({value, source}) so the React editor can read it without a PUT first.
	s, _ := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/symbol-overrides/TSLA", nil)
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.getSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["symbol"] != "TSLA" {
		t.Errorf("symbol = %v, want TSLA", got["symbol"])
	}
	// No override stored: all fields must come from profile with source=="profile".
	scoreEntry, ok := got["score_threshold"].(map[string]any)
	if !ok {
		t.Fatalf("score_threshold not a {value,source} object: %v", got["score_threshold"])
	}
	if scoreEntry["source"] != "profile" {
		t.Errorf("score_threshold.source = %v, want profile", scoreEntry["source"])
	}
	if scoreEntry["value"].(float64) != 10.0 {
		t.Errorf("score_threshold.value = %v, want 10.0 (from profile)", scoreEntry["value"])
	}
}

func TestPutSymbolOverride_ValidThenGet(t *testing.T) {
	s, store := newTestServer(t, "")
	body := []byte(`{
		"score_threshold": 14.0,
		"cooldown_hours": null,
		"alert_limit_per_day": null,
		"timeframes": ["1D","1W"],
		"allowed_rules": null
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	got, _ := store.Get("TSLA")
	if got == nil || got.ScoreThreshold == nil || *got.ScoreThreshold != 14.0 {
		t.Errorf("DB row mismatch: %#v", got)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	scoreEntry, ok := resp["score_threshold"].(map[string]any)
	if !ok {
		t.Fatalf("score_threshold entry missing or wrong shape: %v", resp["score_threshold"])
	}
	if scoreEntry["source"] != "override" {
		t.Errorf("score_threshold.source = %v, want override", scoreEntry["source"])
	}
	if scoreEntry["value"].(float64) != 14.0 {
		t.Errorf("score_threshold.value = %v, want 14.0", scoreEntry["value"])
	}
	cooldownEntry := resp["cooldown_hours"].(map[string]any)
	if cooldownEntry["source"] != "profile" {
		t.Errorf("cooldown_hours.source = %v, want profile", cooldownEntry["source"])
	}
}

func TestPutSymbolOverride_AllNullDeletesRow(t *testing.T) {
	s, store := newTestServer(t, "")
	score := 14.0
	_ = store.Put(storage.SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score})

	body := []byte(`{
		"score_threshold": null, "cooldown_hours": null, "alert_limit_per_day": null,
		"timeframes": null, "allowed_rules": null
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	got, _ := store.Get("TSLA")
	if got != nil {
		t.Errorf("row not deleted on all-null PUT: %#v", got)
	}
}

func TestPutSymbolOverride_ScoreOutOfRange(t *testing.T) {
	s, _ := newTestServer(t, "")
	body := []byte(`{"score_threshold": 999.0}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Errorf("missing error message in response")
	}
}

func TestPutSymbolOverride_UnknownTimeframe(t *testing.T) {
	s, _ := newTestServer(t, "")
	body := []byte(`{"timeframes": ["1H","30m"]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPutSymbolOverride_UnknownRule(t *testing.T) {
	s, _ := newTestServer(t, "")
	body := []byte(`{"allowed_rules": ["foo_bar"]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPutSymbolOverride_AuthRequired(t *testing.T) {
	s, _ := newTestServer(t, "secret-token")
	body := []byte(`{"score_threshold": 14.0}`)

	// No header → 401
	req := httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.putSymbolOverride(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", w.Code)
	}

	// Valid header → 200
	req = httptest.NewRequest(http.MethodPut, "/api/symbol-overrides/TSLA", bytes.NewReader(body))
	req.SetPathValue("symbol", "TSLA")
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	s.putSymbolOverride(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("with token: status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteSymbolOverride(t *testing.T) {
	s, store := newTestServer(t, "")
	score := 14.0
	_ = store.Put(storage.SymbolOverride{Symbol: "TSLA", ScoreThreshold: &score})

	req := httptest.NewRequest(http.MethodDelete, "/api/symbol-overrides/TSLA", nil)
	req.SetPathValue("symbol", "TSLA")
	w := httptest.NewRecorder()
	s.deleteSymbolOverride(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	if got, _ := store.Get("TSLA"); got != nil {
		t.Errorf("row still present after delete")
	}

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	scoreEntry := resp["score_threshold"].(map[string]any)
	if scoreEntry["source"] != "profile" {
		t.Errorf("after delete: source = %v, want profile", scoreEntry["source"])
	}
}
