package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeUpstream returns a canned JSON-RPC response and echoes the Authorization header.
func fakeUpstream(t *testing.T, echo *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if echo != nil {
			*echo = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "abc123")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
}

func TestBridge_ForwardsRequestWithAuthHeader(t *testing.T) {
	var echoed string
	srv := fakeUpstream(t, &echoed)
	defer srv.Close()

	cfg := bridgeConfig{
		url:     srv.URL + "/api/mcp",
		token:   "mytoken",
		timeout: 2 * time.Second,
	}

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var out bytes.Buffer
	var stderr bytes.Buffer

	if err := runBridge(cfg, in, &out, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf("output missing result: %s", out.String())
	}
	if echoed != "Bearer mytoken" {
		t.Errorf("auth header not forwarded: %q", echoed)
	}
}

func TestBridge_UpstreamNetworkErrorTranslatedToMcpError(t *testing.T) {
	cfg := bridgeConfig{
		url:     "http://127.0.0.1:1/api/mcp",
		token:   "",
		timeout: 200 * time.Millisecond,
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var out bytes.Buffer
	var stderr bytes.Buffer
	_ = runBridge(cfg, in, &out, &stderr)

	var resp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(&out).Decode(&resp); err != nil {
		t.Fatalf("decode: %v (raw: %s)", err, out.String())
	}
	if resp.Error.Code != -32603 {
		t.Errorf("want -32603, got %d", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "not reachable") &&
		!strings.Contains(resp.Error.Message, "refused") &&
		!strings.Contains(resp.Error.Message, "timeout") {
		t.Errorf("error msg should hint at connectivity: %s", resp.Error.Message)
	}
}

func TestBridge_Upstream401TranslatesToBridgeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()
	cfg := bridgeConfig{url: srv.URL + "/api/mcp", timeout: 2 * time.Second}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var out bytes.Buffer
	var stderr bytes.Buffer
	_ = runBridge(cfg, in, &out, &stderr)

	var resp struct {
		Error struct {
			Code    int
			Message string
		} `json:"error"`
	}
	_ = json.NewDecoder(&out).Decode(&resp)
	if resp.Error.Code != -32603 {
		t.Errorf("want -32603, got %d", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "CHARTNAGARI_TOKEN") {
		t.Errorf("error msg should mention token: %q", resp.Error.Message)
	}
}

func TestBridge_PropagatesSessionHeader(t *testing.T) {
	var sessionOut string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionOut = r.Header.Get("Mcp-Session-Id")
		w.Header().Set("Mcp-Session-Id", "serverissued")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	cfg := bridgeConfig{url: srv.URL + "/api/mcp", timeout: 2 * time.Second}
	// Two requests — first creates session, second must reuse the returned ID.
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")
	var out bytes.Buffer
	var stderr bytes.Buffer
	_ = runBridge(cfg, in, &out, &stderr)

	if sessionOut != "serverissued" {
		t.Errorf("second request did not propagate session ID: %q", sessionOut)
	}
}

func TestBridge_MissingURLErrors(t *testing.T) {
	cfg := bridgeConfig{url: "", timeout: 1 * time.Second}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var out bytes.Buffer
	var stderr bytes.Buffer
	err := runBridge(cfg, in, &out, &stderr)
	if err == nil {
		t.Fatal("want startup error on missing URL")
	}
}
