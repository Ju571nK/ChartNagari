package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsMigrationOnceAndSecretDeletion(t *testing.T) {
	dir := ollamaMinimalConfigDir(t)
	envPath := filepath.Join(dir, "legacy.env")
	if err := os.WriteFile(envPath, []byte("TIINGO_API_KEY=legacy-secret\nSERVER_PORT=8088\nALPACA_API_KEY=paper-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TIINGO_API_KEY", "process-secret")
	first, err := Load(envPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Tiingo.APIKey != "process-secret" || first.ServerPort != "8088" {
		t.Fatal("legacy precedence not preserved")
	}
	path := filepath.Join(dir, "settings.yaml")
	s, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Clients["ALPACA_API_KEY"] != "paper-key" {
		t.Fatal("sidecar key not migrated")
	}
	s.ApplyMap(map[string]string{"TIINGO_API_KEY": "", "SERVER_PORT": "8090", "DB_PATH": "new.db"})
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	second, err := Load(envPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	if second.Tiingo.APIKey != "" || second.ServerPort != "8090" || second.DBPath != "new.db" {
		t.Fatal("YAML did not override stale environment")
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0600 {
		t.Fatalf("secret file permissions: %v", stat.Mode())
	}
}

func TestSettingsRejectInvalidUpdates(t *testing.T) {
	for _, m := range []map[string]string{
		{"SERVER_PORT": "-1"}, {"YAHOO_POLL_INTERVAL": "0"}, {"OLLAMA_TIMEOUT_SEC": "NaN"},
		{"TIINGO_POLL_INTERVAL": "1.5"}, {"ALPACA_NOTIONAL_PER_TRADE": "Inf"}, {"UNKNOWN": "x"},
	} {
		if ValidateSettingsUpdates(m) == nil {
			t.Errorf("accepted %v", m)
		}
	}
}

func TestMalformedMigrationDoesNotOverwriteSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	before := "server: [invalid"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateSettings("", path); err == nil {
		t.Fatal("accepted broken YAML")
	}
	after, _ := os.ReadFile(path)
	if string(after) != before {
		t.Fatal("overwrote broken file")
	}
}
