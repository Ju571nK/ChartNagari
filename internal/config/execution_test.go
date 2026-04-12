package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExecutionConfig_Missing(t *testing.T) {
	cfg, err := LoadExecutionConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should return zero-value, got err: %v", err)
	}
	if cfg.Enabled || len(cfg.Plugins) != 0 {
		t.Errorf("zero-value expected, got %+v", cfg)
	}
}

func TestLoadExecutionConfig_Valid(t *testing.T) {
	yaml := `
enabled: true
kill_switch: false
max_dispatched: 3
dedup_window_sec: 300
timestamp_skew_sec: 300
allowed_origins:
  - "https://plugin.example.com"
symbol_map:
  btcusdt: binance:BTCUSDT
plugins:
  - id: alpaca
    url: https://hooks.example.com/alpaca
    secret: topsecret
    enabled: true
    symbols: ["aapl", "msft"]
    min_score: 12
    direction_filter: LONG
`
	path := writeTempYAML(t, yaml)
	cfg, err := LoadExecutionConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.Enabled || cfg.MaxDispatched != 3 || cfg.TimestampSkewSec != 300 {
		t.Errorf("top-level fields not parsed: %+v", cfg)
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0].ID != "alpaca" {
		t.Fatalf("plugin not parsed: %+v", cfg.Plugins)
	}
	p := cfg.Plugins[0]
	// symbols normalized to uppercase by Validate()
	if p.Symbols[0] != "AAPL" || p.Symbols[1] != "MSFT" {
		t.Errorf("plugin symbols not uppercased: %v", p.Symbols)
	}
	// symbol_map keys normalized
	if _, ok := cfg.SymbolMap["BTCUSDT"]; !ok {
		t.Errorf("symbol_map key not uppercased: %v", cfg.SymbolMap)
	}
}

func TestValidate_RequiresURL(t *testing.T) {
	cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "p1", URL: ""}}}
	if err := cfg.Validate(); err == nil {
		t.Error("empty url should fail validation")
	}
}

func TestValidate_URLScheme(t *testing.T) {
	cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "p1", URL: "ftp://x"}}}
	if err := cfg.Validate(); err == nil {
		t.Error("non-http(s) url should fail validation")
	}
}

func TestValidate_RequiresID(t *testing.T) {
	cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "", URL: "https://x"}}}
	if err := cfg.Validate(); err == nil {
		t.Error("empty id should fail validation")
	}
}

func TestValidate_DuplicateID(t *testing.T) {
	cfg := ExecutionConfig{Plugins: []PluginConfig{
		{ID: "dup", URL: "https://a"},
		{ID: "dup", URL: "https://b"},
	}}
	if err := cfg.Validate(); err == nil {
		t.Error("duplicate plugin id should fail validation")
	}
}

func TestValidate_NegativeMinScore(t *testing.T) {
	cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "p1", URL: "https://x", MinScore: -1}}}
	if err := cfg.Validate(); err == nil {
		t.Error("negative min_score should fail validation")
	}
}

func TestValidate_DirectionFilter(t *testing.T) {
	good := []string{"", "LONG", "SHORT"}
	for _, d := range good {
		cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "p1", URL: "https://x", DirectionFilter: d}}}
		if err := cfg.Validate(); err != nil {
			t.Errorf("direction_filter=%q should pass, got %v", d, err)
		}
	}
	bad := []string{"long", "both", "BUY", "NEUTRAL"}
	for _, d := range bad {
		cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "p1", URL: "https://x", DirectionFilter: d}}}
		if err := cfg.Validate(); err == nil {
			t.Errorf("direction_filter=%q should fail validation", d)
		}
	}
}

func TestValidate_NegativeCounters(t *testing.T) {
	cases := []ExecutionConfig{
		{MaxDispatched: -1},
		{DedupWindowSec: -1},
		{TimestampSkewSec: -1},
	}
	for i, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Errorf("case %d: negative counter should fail validation", i)
		}
	}
}

func TestValidate_SymbolMapKeyUppercased(t *testing.T) {
	cfg := ExecutionConfig{SymbolMap: map[string]string{"btcusdt": "binance:BTCUSDT"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, ok := cfg.SymbolMap["BTCUSDT"]; !ok {
		t.Errorf("key not uppercased in place: %v", cfg.SymbolMap)
	}
	if _, ok := cfg.SymbolMap["btcusdt"]; ok {
		t.Errorf("lowercase key still present: %v", cfg.SymbolMap)
	}
}

func TestValidate_SymbolMapEmptyKey(t *testing.T) {
	cfg := ExecutionConfig{SymbolMap: map[string]string{"  ": "binance:X"}}
	if err := cfg.Validate(); err == nil {
		t.Error("empty/whitespace key should fail validation")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")

	orig := ExecutionConfig{
		Enabled:          true,
		MaxDispatched:    5,
		DedupWindowSec:   120,
		TimestampSkewSec: 200,
		SymbolMap:        map[string]string{"btcusdt": "binance:BTCUSDT"},
		Plugins: []PluginConfig{
			{ID: "p1", URL: "https://a", Secret: "s1", Enabled: true, Symbols: []string{"aapl"}, MinScore: 10, DirectionFilter: "LONG"},
		},
	}
	if err := SaveExecutionConfig(path, orig); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadExecutionConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.MaxDispatched != 5 || loaded.DedupWindowSec != 120 || loaded.TimestampSkewSec != 200 {
		t.Errorf("counters drifted: %+v", loaded)
	}
	if len(loaded.Plugins) != 1 || loaded.Plugins[0].Secret != "s1" {
		t.Errorf("secret drifted on round-trip: %+v", loaded.Plugins)
	}
	// Validate() normalizes to uppercase at both save and load.
	if loaded.Plugins[0].Symbols[0] != "AAPL" {
		t.Errorf("symbol not uppercased on round-trip: %v", loaded.Plugins[0].Symbols)
	}
}

func TestSaveExecutionConfig_NoTmpLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")
	cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "p1", URL: "https://x"}}}
	if err := SaveExecutionConfig(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") || strings.HasPrefix(e.Name(), ".execution-") {
			t.Errorf("atomic save left temp file behind: %s", e.Name())
		}
	}
}

func TestSaveExecutionConfig_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")
	cfg := ExecutionConfig{Plugins: []PluginConfig{{ID: "", URL: ""}}}
	if err := SaveExecutionConfig(path, cfg); err == nil {
		t.Error("invalid config should not persist")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("invalid config should not create file")
	}
}

func TestRedactSecrets(t *testing.T) {
	cfg := ExecutionConfig{Plugins: []PluginConfig{
		{ID: "a", Secret: "topsecret"},
		{ID: "b", Secret: "another"},
	}}
	red := cfg.RedactSecrets()
	for _, p := range red.Plugins {
		if p.Secret != SecretRedacted {
			t.Errorf("plugin %s: secret not redacted, got %q", p.ID, p.Secret)
		}
	}
	// Original must be untouched (RedactSecrets returns a copy).
	if cfg.Plugins[0].Secret != "topsecret" {
		t.Errorf("original mutated: %q", cfg.Plugins[0].Secret)
	}
}

func TestMergeIncomingSecrets(t *testing.T) {
	current := ExecutionConfig{Plugins: []PluginConfig{
		{ID: "a", Secret: "oldA"},
		{ID: "b", Secret: "oldB"},
	}}
	incoming := ExecutionConfig{Plugins: []PluginConfig{
		{ID: "a", Secret: ""},              // keep old
		{ID: "b", Secret: SecretRedacted},  // keep old
		{ID: "c", Secret: "newC"},          // new plugin, new secret
	}}
	merged := MergeIncomingSecrets(current, incoming)
	want := map[string]string{"a": "oldA", "b": "oldB", "c": "newC"}
	for _, p := range merged.Plugins {
		if p.Secret != want[p.ID] {
			t.Errorf("plugin %s: got secret %q, want %q", p.ID, p.Secret, want[p.ID])
		}
	}
}

func TestMergeIncomingSecrets_OverwriteWithNew(t *testing.T) {
	current := ExecutionConfig{Plugins: []PluginConfig{{ID: "a", Secret: "oldA"}}}
	incoming := ExecutionConfig{Plugins: []PluginConfig{{ID: "a", Secret: "rotated"}}}
	merged := MergeIncomingSecrets(current, incoming)
	if merged.Plugins[0].Secret != "rotated" {
		t.Errorf("new secret should win, got %q", merged.Plugins[0].Secret)
	}
}

func TestHolder_SetKillSwitch_PersistsBeforeMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")
	cfg := ExecutionConfig{
		Enabled: true,
		Plugins: []PluginConfig{{ID: "p1", URL: "https://x"}},
	}
	if err := SaveExecutionConfig(path, cfg); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	h := NewExecutionHolder(path, cfg)

	if err := h.SetKillSwitch(true); err != nil {
		t.Fatalf("SetKillSwitch: %v", err)
	}
	if !h.Get().KillSwitch {
		t.Error("in-memory kill switch not flipped")
	}
	// Verify disk committed first.
	onDisk, err := LoadExecutionConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !onDisk.KillSwitch {
		t.Error("kill switch not persisted to disk")
	}
}

func TestHolder_PluginByID(t *testing.T) {
	h := NewExecutionHolder("", ExecutionConfig{
		Plugins: []PluginConfig{{ID: "alpaca", URL: "https://x"}},
	})
	if p, ok := h.PluginByID("alpaca"); !ok || p.URL != "https://x" {
		t.Errorf("PluginByID hit failed: %+v ok=%v", p, ok)
	}
	if _, ok := h.PluginByID("missing"); ok {
		t.Error("PluginByID should miss on unknown id")
	}
}

func TestTimestampSkew_DedupWindow_Defaults(t *testing.T) {
	zero := ExecutionConfig{}
	if zero.TimestampSkew() != DefaultTimestampSkewSec {
		t.Errorf("default TimestampSkew: got %d", zero.TimestampSkew())
	}
	if zero.DedupWindow() != DefaultDedupWindowSec {
		t.Errorf("default DedupWindow: got %d", zero.DedupWindow())
	}
	set := ExecutionConfig{TimestampSkewSec: 60, DedupWindowSec: 30}
	if set.TimestampSkew() != 60 || set.DedupWindow() != 30 {
		t.Errorf("explicit values not returned: %+v", set)
	}
}

func writeTempYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "execution.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return path
}
