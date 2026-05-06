package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/mcp"
)

func newMCPTestServer(t *testing.T, reg *mcp.Registry, token string) *httptest.Server {
	t.Helper()
	s := &Server{}
	s.WithMCPRegistry(reg)
	if token != "" {
		s.WithAPIToken(token)
	}
	return httptest.NewServer(s.Handler())
}

func postMCP(t *testing.T, url, body, sessionID, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url+"/api/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func TestMCP_InitializeReturnsCapabilitiesAndSession(t *testing.T) {
	reg := mcp.NewRegistry()
	srv := newMCPTestServer(t, reg, "")
	defer srv.Close()

	resp := postMCP(t, srv.URL,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		"", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	sid := resp.Header.Get("Mcp-Session-Id")
	if sid == "" {
		t.Error("missing Mcp-Session-Id header")
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	result, _ := out["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result: %+v", out)
	}
	caps, _ := result["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("capabilities.tools missing: %+v", caps)
	}
}

func TestMCP_ToolsList(t *testing.T) {
	reg := mcp.NewRegistry()
	reg.Register(mcp.NewListWatchlist(&stubWatchlist{}))
	srv := newMCPTestServer(t, reg, "")
	defer srv.Close()

	initResp := postMCP(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, "", "")
	sid := initResp.Header.Get("Mcp-Session-Id")
	initResp.Body.Close()

	resp := postMCP(t, srv.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, sid, "")
	defer resp.Body.Close()

	var out struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Result.Tools) != 1 || out.Result.Tools[0].Name != "list_watchlist" {
		t.Errorf("tools/list returned: %+v", out.Result.Tools)
	}
}

func TestMCP_Unauthorized(t *testing.T) {
	reg := mcp.NewRegistry()
	srv := newMCPTestServer(t, reg, "secret")
	defer srv.Close()

	resp := postMCP(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestMCP_AuthorizedWithBearer(t *testing.T) {
	reg := mcp.NewRegistry()
	srv := newMCPTestServer(t, reg, "secret")
	defer srv.Close()

	resp := postMCP(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, "", "secret")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestMCP_NoRegistry503(t *testing.T) {
	s := &Server{}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	resp, _ := http.Post(ts.URL+"/api/mcp", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestMCP_MalformedJSON(t *testing.T) {
	reg := mcp.NewRegistry()
	srv := newMCPTestServer(t, reg, "")
	defer srv.Close()
	resp := postMCP(t, srv.URL, `{"not json`, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	var env struct {
		Error struct{ Code int } `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if env.Error.Code != -32700 {
		t.Errorf("want parse error code, got %d", env.Error.Code)
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	reg := mcp.NewRegistry()
	srv := newMCPTestServer(t, reg, "")
	defer srv.Close()
	initResp := postMCP(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, "", "")
	sid := initResp.Header.Get("Mcp-Session-Id")
	initResp.Body.Close()

	resp := postMCP(t, srv.URL, `{"jsonrpc":"2.0","id":2,"method":"x/y"}`, sid, "")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte(`-32601`)) {
		t.Errorf("expected MethodNotFound code: %s", body)
	}
}

// stubWatchlist satisfies mcp.WatchlistSource for tools/list test.
type stubWatchlist struct{}

func (*stubWatchlist) Watchlist() appconfig.WatchlistConfig {
	return appconfig.WatchlistConfig{}
}
