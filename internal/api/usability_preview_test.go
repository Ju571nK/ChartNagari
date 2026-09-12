package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in isolated browser fixture: no user DB, secrets, collectors,
// notifications or execution plugins. Never runs in normal tests/CI.
func TestUsabilityPreview(t *testing.T) {
	if os.Getenv("CHARTTER_USABILITY_PREVIEW") != "1" {
		t.Skip("manual browser fixture")
	}
	s, db := preparationServer(t)
	seedPreparationBars(t, db, "BTCUSDT", 202)
	seedPreparationBars(t, db, "AAPL", 132)
	webDist, err := filepath.Abs("../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	s.static = New(s.configDir, webDist).static
	handler := s.Handler()
	preview := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/symbols/validate" {
			jsonOK(w, map[string]any{"found": true, "type": "stock", "exchange": "nasdaq", "name": "TEST instrument"})
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer preview.Close()
	fmt.Println("ISOLATED_USABILITY_PREVIEW", preview.URL)
	<-time.After(10 * time.Minute)
}
