package api

import (
	"encoding/json"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebSettingsYAMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	s := New(dir, "")
	s.WithSettingsFile(path)
	s.WithAPIToken("admin")
	for _, token := range []string{"", "wrong", "admin"} {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/check", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		want := http.StatusUnauthorized
		if token == "admin" {
			want = http.StatusNoContent
		}
		if w.Code != want {
			t.Fatalf("auth check: got %d, want %d", w.Code, want)
		}
	}
	call := func(method, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/settings/config", strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := call(http.MethodPut, `{"DB_PATH":"private.db"}`, ""); w.Code != 401 {
		t.Fatalf("unauthorized save: %d", w.Code)
	}
	body := `{"DB_PATH":"private.db","SERVER_HOST":"127.0.0.1","OLLAMA_TIMEOUT_SEC":"75","ALPACA_API_KEY":"paper-secret","CHARTNAGARI_TOKEN":"bridge-secret"}`
	if w := call(http.MethodPut, body, "admin"); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w := call(http.MethodGet, "", "")
	if strings.Contains(w.Body.String(), "paper-secret") || strings.Contains(w.Body.String(), "bridge-secret") {
		t.Fatal("GET exposed secrets")
	}
	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["ALPACA_API_KEY"] != envSentinel || m["DB_PATH"] != "private.db" {
		t.Fatal("missing settings")
	}
	if w := call(http.MethodPut, `{"ALPACA_API_KEY":"__configured__","CHARTNAGARI_TOKEN":""}`, "admin"); w.Code != 200 {
		t.Fatal(w.Body)
	}
	stored, err := appconfig.LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ToMap()["ALPACA_API_KEY"] != "paper-secret" || stored.ToMap()["CHARTNAGARI_TOKEN"] != "" || stored.Version != 1 {
		t.Fatal("secret preservation/deletion failed")
	}
	before, _ := os.ReadFile(path)
	if w := call(http.MethodPut, `{"YAHOO_POLL_INTERVAL":"0"}`, "admin"); w.Code != 400 {
		t.Fatal("invalid interval accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid update changed file")
	}
	if err := os.WriteFile(path, []byte("server: [bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := call(http.MethodPut, `{"DB_PATH":"x"}`, "admin"); w.Code != 500 {
		t.Fatal("malformed YAML overwritten")
	}
}
