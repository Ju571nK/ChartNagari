package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

func TestSettingsStartupStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	saved, _ := appconfig.LoadSettings(path)
	saved.ApplyMap(map[string]string{"FMP_API_KEY": "original-secret"})
	if err := appconfig.SaveSettings(path, saved); err != nil {
		t.Fatal(err)
	}
	s := New(dir, "")
	s.WithSettingsFile(path)
	get := func() map[string]string {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/settings/status", nil))
		if w.Code != 200 || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("unsafe status: %d", w.Code)
		}
		var out struct {
			Fields map[string]string `json:"fields"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Fields
	}
	if get()["FMP_API_KEY"] != "unknown" {
		t.Fatal("must not infer startup from disk")
	}
	startup := saved.ToMap()
	s.WithStartupSettings(startup)
	startup["FMP_API_KEY"] = "caller-changed-secret"
	if get()["FMP_API_KEY"] != "startup_match" {
		t.Fatal("snapshot must be copied")
	}
	saved.ApplyMap(map[string]string{"FMP_API_KEY": "replacement-secret"})
	if err := appconfig.SaveSettings(path, saved); err != nil {
		t.Fatal(err)
	}
	if get()["FMP_API_KEY"] != "pending_restart" {
		t.Fatal("replacement must require restart")
	}
	if get()["ALPACA_API_KEY"] != "external" {
		t.Fatal("external process cannot be verified")
	}
	saved.ApplyMap(map[string]string{"FMP_API_KEY": ""})
	if err := appconfig.SaveSettings(path, saved); err != nil {
		t.Fatal(err)
	}
	if get()["FMP_API_KEY"] != "pending_restart" {
		t.Fatal("clear must require restart")
	}
	s.WithStartupSettings(saved.ToMap()) // simulate next process startup
	if get()["FMP_API_KEY"] != "startup_match" {
		t.Fatal("restart comparison failed")
	}
}

func TestCalendarStatusIndependentOfEventStore(t *testing.T) {
	s := New(t.TempDir(), "")
	check := func(want string) {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/calendar/status", nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	check("unknown")
	s.WithCalendarDiagnostics(func() any { return map[string]string{"state": "disabled"} })
	check("disabled")
}
