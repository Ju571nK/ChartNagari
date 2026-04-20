package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// minimalConfigDir writes the minimum set of config files that Load() requires
// and returns the temp directory path.
func minimalConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	must("settings.yaml", "") // empty — defaults should fill in
	must("rules.yaml", "rules: []\nscoring:\n  thresholds: {}\ntimeframe_weights: {}\n")
	must("watchlist.yaml", "symbols:\n  crypto: []\n  stocks: []\n  indices: []\ntimeframes: []\n")
	return dir
}

func TestLoad_OllamaDefaults(t *testing.T) {
	os.Unsetenv("OLLAMA_HOST")
	os.Unsetenv("OLLAMA_MODEL")
	os.Unsetenv("OLLAMA_TIMEOUT_SEC")

	cfg, err := Load("", minimalConfigDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Ollama.Host != "http://localhost:11434" {
		t.Fatalf("default host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "gemma4:4b" {
		t.Fatalf("default model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout != 120*time.Second {
		t.Fatalf("default timeout: %v", cfg.Ollama.Timeout)
	}
}

func TestLoad_OllamaEnvOverrides(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://ollama:11434")
	t.Setenv("OLLAMA_MODEL", "gemma4:12b")
	t.Setenv("OLLAMA_TIMEOUT_SEC", "60")

	cfg, err := Load("", minimalConfigDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Ollama.Host != "http://ollama:11434" {
		t.Fatalf("host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "gemma4:12b" {
		t.Fatalf("model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout != 60*time.Second {
		t.Fatalf("timeout: %v", cfg.Ollama.Timeout)
	}
}

func TestLoad_OllamaYAMLOverrides(t *testing.T) {
	os.Unsetenv("OLLAMA_HOST")
	os.Unsetenv("OLLAMA_MODEL")
	os.Unsetenv("OLLAMA_TIMEOUT_SEC")

	dir := t.TempDir()
	settings := `ollama:
  host: http://yaml:11434
  model: gemma4:27b
  timeout_sec: 300
`
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte(settings), 0644); err != nil {
		t.Fatalf("write settings.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte("rules: []\nscoring:\n  thresholds: {}\ntimeframe_weights: {}\n"), 0644); err != nil {
		t.Fatalf("write rules.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "watchlist.yaml"), []byte("symbols:\n  crypto: []\n  stocks: []\n  indices: []\ntimeframes: []\n"), 0644); err != nil {
		t.Fatalf("write watchlist.yaml: %v", err)
	}

	cfg, err := Load("", dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Ollama.Host != "http://yaml:11434" {
		t.Fatalf("yaml host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "gemma4:27b" {
		t.Fatalf("yaml model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout != 300*time.Second {
		t.Fatalf("yaml timeout: %v", cfg.Ollama.Timeout)
	}
}

func TestSettingsYAML_OllamaMapRoundtrip(t *testing.T) {
	in := SettingsYAML{}
	in.Ollama.Host = "http://localhost:11434"
	in.Ollama.Model = "gemma4:4b"
	in.Ollama.TimeoutSec = 90

	m := in.ToMap()
	if m["OLLAMA_HOST"] != "http://localhost:11434" {
		t.Fatalf("ToMap OLLAMA_HOST: %q", m["OLLAMA_HOST"])
	}
	if m["OLLAMA_MODEL"] != "gemma4:4b" {
		t.Fatalf("ToMap OLLAMA_MODEL: %q", m["OLLAMA_MODEL"])
	}
	if m["OLLAMA_TIMEOUT_SEC"] != "90" {
		t.Fatalf("ToMap OLLAMA_TIMEOUT_SEC: %q", m["OLLAMA_TIMEOUT_SEC"])
	}

	var out SettingsYAML
	out.ApplyMap(m)
	if out.Ollama.Host != in.Ollama.Host || out.Ollama.Model != in.Ollama.Model || out.Ollama.TimeoutSec != in.Ollama.TimeoutSec {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", out.Ollama, in.Ollama)
	}
}
