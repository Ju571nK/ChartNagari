// Package config — execution plugin configuration for the trade dispatcher.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// SecretRedacted is the placeholder returned to API clients instead of a raw
// plugin secret. When a PUT /api/execution/config request arrives with this
// value (or an empty string) the handler must preserve the existing secret.
const SecretRedacted = "***"

// DefaultTimestampSkewSec is the accepted clock skew window for HMAC-signed
// webhook and feedback requests when timestamp_skew_sec is unset or zero.
const DefaultTimestampSkewSec = 300

// DefaultDedupWindowSec is the TTL window (seconds) for (symbol, rule, direction)
// dedup rows inserted by the dispatcher when dedup_window_sec is unset or zero.
const DefaultDedupWindowSec = 300

// PluginConfig is a single downstream execution plugin registration.
type PluginConfig struct {
	ID              string   `yaml:"id" json:"id"`
	URL             string   `yaml:"url" json:"url"`
	Secret          string   `yaml:"secret" json:"secret"`
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	Symbols         []string `yaml:"symbols,omitempty" json:"symbols,omitempty"`
	MinScore        float64  `yaml:"min_score,omitempty" json:"min_score,omitempty"`
	DirectionFilter string   `yaml:"direction_filter,omitempty" json:"direction_filter,omitempty"`
}

// ExecutionConfig is the top-level structure for config/execution.yaml.
// It drives the dispatcher's fan-out behavior and the kill-switch.
type ExecutionConfig struct {
	Enabled          bool              `yaml:"enabled" json:"enabled"`
	KillSwitch       bool              `yaml:"kill_switch" json:"kill_switch"`
	MaxDispatched    int               `yaml:"max_dispatched" json:"max_dispatched"`
	DedupWindowSec   int               `yaml:"dedup_window_sec" json:"dedup_window_sec"`
	TimestampSkewSec int               `yaml:"timestamp_skew_sec" json:"timestamp_skew_sec"`
	AllowedOrigins   []string          `yaml:"allowed_origins,omitempty" json:"allowed_origins,omitempty"`
	SymbolMap        map[string]string `yaml:"symbol_map,omitempty" json:"symbol_map,omitempty"`
	Plugins          []PluginConfig    `yaml:"plugins" json:"plugins"`
}

// TimestampSkew returns the configured HMAC timestamp tolerance in seconds,
// falling back to DefaultTimestampSkewSec when unset or non-positive.
func (c ExecutionConfig) TimestampSkew() int {
	if c.TimestampSkewSec > 0 {
		return c.TimestampSkewSec
	}
	return DefaultTimestampSkewSec
}

// DedupWindow returns the dedup TTL in seconds, falling back to
// DefaultDedupWindowSec when unset or non-positive.
func (c ExecutionConfig) DedupWindow() int {
	if c.DedupWindowSec > 0 {
		return c.DedupWindowSec
	}
	return DefaultDedupWindowSec
}

// Validate checks the config for structural errors. Callers should invoke this
// on Load and before Save to avoid persisting malformed state.
func (c *ExecutionConfig) Validate() error {
	if c.MaxDispatched < 0 {
		return fmt.Errorf("max_dispatched must be >= 0, got %d", c.MaxDispatched)
	}
	if c.DedupWindowSec < 0 {
		return fmt.Errorf("dedup_window_sec must be >= 0, got %d", c.DedupWindowSec)
	}
	if c.TimestampSkewSec < 0 {
		return fmt.Errorf("timestamp_skew_sec must be >= 0, got %d", c.TimestampSkewSec)
	}

	seen := make(map[string]struct{}, len(c.Plugins))
	for i := range c.Plugins {
		p := &c.Plugins[i]
		if strings.TrimSpace(p.ID) == "" {
			return fmt.Errorf("plugins[%d]: id is required", i)
		}
		if _, dup := seen[p.ID]; dup {
			return fmt.Errorf("plugins[%d]: duplicate id %q", i, p.ID)
		}
		seen[p.ID] = struct{}{}

		if strings.TrimSpace(p.URL) == "" {
			return fmt.Errorf("plugins[%d] (%s): url is required", i, p.ID)
		}
		if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
			return fmt.Errorf("plugins[%d] (%s): url must be http:// or https://", i, p.ID)
		}
		if p.MinScore < 0 {
			return fmt.Errorf("plugins[%d] (%s): min_score must be >= 0, got %v", i, p.ID, p.MinScore)
		}
		switch p.DirectionFilter {
		case "", "LONG", "SHORT":
			// ok
		default:
			return fmt.Errorf("plugins[%d] (%s): direction_filter must be \"\", \"LONG\", or \"SHORT\", got %q",
				i, p.ID, p.DirectionFilter)
		}
		// Normalize plugin symbols to uppercase in-place.
		for j, sym := range p.Symbols {
			p.Symbols[j] = strings.ToUpper(strings.TrimSpace(sym))
		}
	}

	// Normalize symbol_map keys and values to uppercase in-place.
	if len(c.SymbolMap) > 0 {
		normalized := make(map[string]string, len(c.SymbolMap))
		for k, v := range c.SymbolMap {
			kk := strings.ToUpper(strings.TrimSpace(k))
			if kk == "" {
				return errors.New("symbol_map: empty key")
			}
			normalized[kk] = strings.ToUpper(strings.TrimSpace(v))
		}
		c.SymbolMap = normalized
	}

	return nil
}

// LoadExecutionConfig reads and parses execution.yaml from the given path.
// A missing file returns a zero-value config (disabled, no plugins) with no error.
func LoadExecutionConfig(path string) (ExecutionConfig, error) {
	var cfg ExecutionConfig
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SaveExecutionConfig writes the config atomically: temp file → fsync → rename →
// fsync(parent dir). This guarantees that a kill-switch toggle or secret rotation
// is durable before the in-memory holder flips. See design review Codex #10.
func SaveExecutionConfig(path string, cfg ExecutionConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".execution-*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if anything below fails before rename.
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}

	// fsync the parent directory so the rename itself is durable.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// RedactSecrets returns a copy of the config with every plugin.Secret replaced
// by SecretRedacted. Use this before sending the config to API clients.
func (c ExecutionConfig) RedactSecrets() ExecutionConfig {
	out := c
	if len(c.Plugins) > 0 {
		out.Plugins = make([]PluginConfig, len(c.Plugins))
		for i, p := range c.Plugins {
			p.Secret = SecretRedacted
			out.Plugins[i] = p
		}
	}
	return out
}

// MergeIncomingSecrets merges secrets from an incoming (API-provided) config into
// the current on-disk config. If the incoming plugin.Secret is empty or equal to
// SecretRedacted, the existing secret is preserved; otherwise the new secret wins.
// Matching is by plugin ID.
func MergeIncomingSecrets(current ExecutionConfig, incoming ExecutionConfig) ExecutionConfig {
	existing := make(map[string]string, len(current.Plugins))
	for _, p := range current.Plugins {
		existing[p.ID] = p.Secret
	}
	for i := range incoming.Plugins {
		p := &incoming.Plugins[i]
		if p.Secret == "" || p.Secret == SecretRedacted {
			if old, ok := existing[p.ID]; ok {
				p.Secret = old
			}
		}
	}
	return incoming
}

// ExecutionHolder provides thread-safe access to execution configuration.
// KillSwitch toggles must always SetKillSwitch (which persists to disk first).
type ExecutionHolder struct {
	mu   sync.RWMutex
	cfg  ExecutionConfig
	path string
}

// NewExecutionHolder creates a holder seeded with the given config. The path is
// recorded so SetKillSwitch can persist before flipping the in-memory flag.
func NewExecutionHolder(path string, cfg ExecutionConfig) *ExecutionHolder {
	return &ExecutionHolder{cfg: cfg, path: path}
}

// Get returns a snapshot of the current configuration.
func (h *ExecutionHolder) Get() ExecutionConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// Set replaces the configuration. Callers are responsible for persisting first
// when durability matters (use SaveExecutionConfig before Set on API writes).
func (h *ExecutionHolder) Set(cfg ExecutionConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = cfg
}

// SetKillSwitch flips the kill switch. The change is persisted to disk BEFORE
// the in-memory flag updates so a crash between the two never leaves a plugin
// dispatching against a "killed" config. Codex #10.
func (h *ExecutionHolder) SetKillSwitch(on bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := h.cfg
	next.KillSwitch = on
	if h.path != "" {
		if err := SaveExecutionConfig(h.path, next); err != nil {
			return err
		}
	}
	h.cfg = next
	return nil
}

// PluginByID returns the plugin with the given ID and whether it was found.
func (h *ExecutionHolder) PluginByID(id string) (PluginConfig, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, p := range h.cfg.Plugins {
		if p.ID == id {
			return p, true
		}
	}
	return PluginConfig{}, false
}
