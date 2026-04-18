package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

// setPluginSecret mutates the in-memory ExecutionHolder so the named plugin
// carries the given secret. If the plugin ID is not present it is appended.
// This bypasses the PUT endpoint to avoid extra auth/version-check setup.
func (h *executionHandlerTestServer) setPluginSecret(t *testing.T, pluginID, secret string) {
	t.Helper()
	cfg := h.srv.execHolder.Get()
	found := false
	for i := range cfg.Plugins {
		if cfg.Plugins[i].ID == pluginID {
			cfg.Plugins[i].Secret = secret
			found = true
			break
		}
	}
	if !found {
		cfg.Plugins = append(cfg.Plugins, appconfig.PluginConfig{
			ID:      pluginID,
			URL:     "https://example.com/hook",
			Secret:  secret,
			Enabled: true,
		})
	}
	h.srv.execHolder.Set(cfg)
}

// authReq sends an authenticated request with the given method, path, and
// optional body to the test server, returning the raw *http.Response.
func (h *executionHandlerTestServer) authReq(t *testing.T, method, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, h.ts.URL+path, body)
	if err != nil {
		t.Fatalf("authReq NewRequest %s %s: %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authReq %s %s: %v", method, path, err)
	}
	return resp
}

// TestNoSecretLeakInExecutionEndpoints configures a plugin with a known secret,
// hits every GET /api/execution/* endpoint, and asserts the raw secret string
// never appears in any response body or header.
func TestNoSecretLeakInExecutionEndpoints(t *testing.T) {
	const knownSecret = "SUPERSECRET_DO_NOT_LEAK_0123456789ABCDEF"

	srv, cleanup := newExecutionHandlerTestServer(t)
	defer cleanup()

	// Seed the existing alpaca-paper plugin with the sentinel secret.
	srv.setPluginSecret(t, "alpaca-paper", knownSecret)

	paths := []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/api/execution/config", nil},
		{"GET", "/api/execution/feedback", nil},
		{"GET", "/api/execution/plugins/stats?window=24h", nil},
	}

	for _, p := range paths {
		var resp *http.Response
		if p.body != nil {
			resp = srv.authReq(t, p.method, p.path, bytes.NewReader(p.body))
		} else {
			resp = srv.authReq(t, p.method, p.path, nil)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		// Response body must not contain the raw secret.
		if bytes.Contains(body, []byte(knownSecret)) {
			t.Errorf("[LEAK] %s %s response contains raw secret:\n%s", p.method, p.path, body)
		}

		// Response headers must not contain the raw secret either.
		for k, vals := range resp.Header {
			for _, v := range vals {
				if strings.Contains(v, knownSecret) {
					t.Errorf("[LEAK] %s %s header %s=%s contains raw secret", p.method, p.path, k, v)
				}
			}
		}
	}

	// Also assert that the helper's getConfig view doesn't carry the raw secret
	// through marshaling — protects against the helper struct becoming a leak vector.
	cfg := srv.getConfig(t)
	cfgJSON, _ := json.Marshal(cfg)
	if bytes.Contains(cfgJSON, []byte(knownSecret)) {
		t.Errorf("[LEAK] GET config carries raw secret through test helper")
	}
}
