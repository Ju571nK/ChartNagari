package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
)

func TestPluginManifestProxy(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		ts, _ := strconv.ParseInt(r.Header.Get(plugin.TimestampHeader), 10, 64)
		if r.Method != "GET" || r.URL.RequestURI() != "/v1/manifest" || r.Header.Get(plugin.SignatureHeader) != plugin.Sign("private-shared-secret", "paper", ts, "GET", "/v1/manifest", nil) {
			t.Error("incorrect signed request")
		}
		_ = json.NewEncoder(w).Encode(plugin.Manifest{ProtocolVersion: plugin.ProtocolVersion, ID: "paper", Name: "Example", Version: "1.0.0", Modes: []string{"paper"}})
	}))
	defer upstream.Close()
	s, _, _, _ := newExecTestServer(t, appconfig.ExecutionConfig{Enabled: false, Plugins: []appconfig.PluginConfig{{ID: "paper", URL: upstream.URL + "/webhook", Secret: "private-shared-secret", Enabled: false}}})
	run := func(id, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/execution/plugins/"+id+"/manifest", nil)
		r.SetPathValue("id", id)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.getExecutionPluginManifest(w, r)
		return w
	}
	if w := run("paper", ""); w.Code != 403 || calls != 0 {
		t.Fatal("tokenless proxy permitted")
	}
	s.WithAPIToken("admin-token")
	if w := run("paper", ""); w.Code != 401 || calls != 0 {
		t.Fatal("unauthenticated proxy permitted")
	}
	if w := run("missing", "admin-token"); w.Code != 404 || calls != 0 {
		t.Fatal("unknown target fetched")
	}
	w := run("paper", "admin-token")
	if w.Code != 200 || calls != 1 || strings.Contains(w.Body.String(), "private-shared-secret") {
		t.Fatalf("proxy: %d %s", w.Code, w.Body.String())
	}
	if s.execHolder.Get().Enabled || s.execHolder.Get().Plugins[0].Enabled {
		t.Fatal("inspection enabled execution")
	}
}
