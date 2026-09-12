package alpaca

import (
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"path/filepath"
	"testing"
)

func TestAdapterReadsWebManagedYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	s := &appconfig.SettingsYAML{Version: 1}
	s.ApplyMap(map[string]string{"ALPACA_API_KEY": "yaml-key", "ALPACA_API_SECRET": "yaml-secret", "CHARTNAGARI_PLUGIN_SECRET": "yaml-hmac", "CHARTNAGARI_FEEDBACK_URL": "http://localhost:8080/api/execution/feedback", "ALPACA_NOTIONAL_PER_TRADE": "123"})
	if err := appconfig.SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPACA_API_KEY", "stale-key")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AlpacaAPIKey != "yaml-key" || cfg.NotionalPerTrade != 123 {
		t.Fatal("adapter ignored YAML")
	}
	s.ApplyMap(map[string]string{"ALPACA_API_KEY": ""})
	if err := appconfig.SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("deleted key resurrected from environment")
	}
}
