package config

import (
	"strings"
	"testing"
)

func TestRemoteSettingsValidation(t *testing.T) {
	for _, values := range []map[string]string{
		{"REMOTE_ACCESS": "true"}, {"REMOTE_ACCESS": "yes"},
		{"REMOTE_ALLOWED_ORIGINS": "*"}, {"REMOTE_ALLOWED_ORIGINS": "http://internet.example"},
		{"REMOTE_ALLOWED_ORIGINS": "https://app.example/path"}, {"REMOTE_ALLOWED_ORIGINS": "https://user:secret@app.example"},
	} {
		if ValidateRemoteSettings(values) == nil {
			t.Fatal("unsafe remote settings accepted")
		}
	}
	values := map[string]string{"REMOTE_ACCESS": "true", "API_TOKEN": strings.Repeat("x", 32), "REMOTE_ALLOWED_ORIGINS": "https://app.example, http://localhost:5173"}
	if err := ValidateRemoteSettings(values); err != nil {
		t.Fatal(err)
	}
	var yaml SettingsYAML
	yaml.ApplyMap(values)
	for k, v := range values {
		if yaml.ToMap()[k] != v {
			t.Fatal("remote YAML roundtrip failed", k)
		}
	}
}
