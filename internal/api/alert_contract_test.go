package api

import (
	"encoding/json"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"gopkg.in/yaml.v3"
	"os"
	"testing"
)

func TestAlertBrowserContractRoundTrip(t *testing.T) {
	s := setupTest(t)
	s.WithAlertConfigHolder(appconfig.NewAlertConfigHolder(appconfig.AlertConfig{ScoreThreshold: 12, CooldownHours: 4, MTFConsensusMin: 2}))
	get := do(t, s, "GET", "/api/alert/config", nil)
	var data map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["score_threshold"] != float64(12) {
		t.Fatalf("browser fields missing: %v", data)
	}
	data["score_threshold"] = float64(15)
	data["cooldown_hours"] = float64(6)
	data["crypto_tp_mult"] = float64(2)
	data["crypto_sl_mult"] = float64(1)
	data["stock_tp_mult"] = float64(3)
	data["stock_sl_mult"] = float64(1.5)
	if put := do(t, s, "PUT", "/api/alert/config", data); put.Code != 204 {
		t.Fatalf("save failed: %s", put.Body.String())
	}
	get = do(t, s, "GET", "/api/alert/config", nil)
	if err := json.Unmarshal(get.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["score_threshold"] != float64(15) || data["cooldown_hours"] != float64(6) || data["stock_tp_mult"] != float64(3) {
		t.Fatalf("save did not stick: %v", data)
	}
	bytes, err := os.ReadFile(s.alertCfgFile())
	if err != nil {
		t.Fatal(err)
	}
	var persisted appconfig.AlertConfig
	if err := yaml.Unmarshal(bytes, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != s.alertHolder.Get() {
		t.Fatal("disk and runtime differ")
	}
}
