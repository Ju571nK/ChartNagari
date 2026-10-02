package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/llm"
)

func setupRequest(t *testing.T, client *http.Client, base, method, path string, body any) (int, []byte) {
	t.Helper()
	var input io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, input)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, b
}

func TestAISetupLifecycleAndRuntimeSwitch(t *testing.T) {
	var authSeen, pulled bool
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer private-key" {
			authSeen = true
		}
		switch r.URL.Path {
		case "/api/generate":
			json.NewEncoder(w).Encode(map[string]string{"response": "ready"})
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": "qwen3:4b"}}})
		case "/api/pull":
			var input map[string]any
			json.NewDecoder(r.Body).Decode(&input)
			pulled = input["model"] == "qwen3:8b" && input["name"] == nil
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer host.Close()
	path := filepath.Join(t.TempDir(), "ai_profiles.json")
	store, err := appconfig.NewAIProfileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	switcher := llm.NewSwitch(nil)
	s := New(t.TempDir(), "")
	s.WithAISetup(store, switcher)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	client := srv.Client()

	status, body := setupRequest(t, client, "", http.MethodPost, srv.URL+"/api/ai/profiles", map[string]any{
		"name": "Local", "kind": "ollama", "base_url": host.URL, "model": "qwen3:4b", "api_key": "private-key",
	})
	if status != 200 {
		t.Fatalf("save: %d %s", status, body)
	}
	if bytes.Contains(body, []byte("private-key")) {
		t.Fatal("secret in profile response")
	}
	var profile appconfig.PublicAIProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		t.Fatal(err)
	}
	if !profile.HasAPIKey || profile.Tested {
		t.Fatalf("wrong state: %+v", profile)
	}
	status, _ = setupRequest(t, client, "", http.MethodPost, srv.URL+"/api/ai/profiles/"+profile.ID+"/activate", nil)
	if status != http.StatusConflict {
		t.Fatalf("untested activation: %d", status)
	}
	if switcher.Available() {
		t.Fatal("untested profile became active")
	}
	status, body = setupRequest(t, client, "", http.MethodPost, srv.URL+"/api/ai/profiles/"+profile.ID+"/test", nil)
	if status != 200 || !bytes.Contains(body, []byte(`"ok":true`)) {
		t.Fatalf("test: %d %s", status, body)
	}
	status, body = setupRequest(t, client, "", http.MethodGet, srv.URL+"/api/ai/profiles/"+profile.ID+"/models", nil)
	if status != 200 || !bytes.Contains(body, []byte("qwen3:4b")) {
		t.Fatalf("models: %d %s", status, body)
	}
	status, body = setupRequest(t, client, "", http.MethodPost, srv.URL+"/api/ai/profiles/"+profile.ID+"/pull", map[string]string{"model": "qwen3:8b"})
	if status != 200 || !pulled {
		t.Fatalf("pull: %d %s", status, body)
	}
	if switcher.Available() {
		t.Fatal("test/list/pull changed runtime")
	}
	status, body = setupRequest(t, client, "", http.MethodPost, srv.URL+"/api/ai/profiles/"+profile.ID+"/activate", nil)
	if status != 200 {
		t.Fatalf("activate: %d %s", status, body)
	}
	output, err := switcher.Complete(t.Context(), "", "hi")
	if err != nil || output != "ready" || !authSeen {
		t.Fatalf("runtime: %q %v auth=%v", output, err, authSeen)
	}
	mode, err := os.Stat(path)
	if err != nil || mode.Mode().Perm() != 0600 {
		t.Fatalf("profile file mode: %v %v", mode, err)
	}
	content, _ := os.ReadFile(path)
	if !bytes.Contains(content, []byte("private-key")) {
		t.Fatal("secret not persisted")
	}
	status, body = setupRequest(t, client, "", http.MethodGet, srv.URL+"/api/ai/setup", nil)
	if status != 200 || bytes.Contains(body, []byte("private-key")) {
		t.Fatalf("setup redaction: %d %s", status, body)
	}
	if !bytes.Contains(body, []byte(`"supported":false`)) {
		t.Fatalf("Laya support flag absent: %s", body)
	}

	// Editing the active profile leaves the activated snapshot and runtime intact.
	status, body = setupRequest(t, client, "", http.MethodPost, srv.URL+"/api/ai/profiles", map[string]any{
		"id": profile.ID, "name": "Local draft", "kind": "ollama", "base_url": host.URL, "model": "qwen3:8b",
	})
	if status != 200 {
		t.Fatalf("edit: %d %s", status, body)
	}
	status, _ = setupRequest(t, client, "", http.MethodPost, srv.URL+"/api/ai/profiles/"+profile.ID+"/activate", nil)
	if status != http.StatusConflict {
		t.Fatalf("edited profile activated without test: %d", status)
	}
	restarted, err := appconfig.NewAIProfileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	active, ok := restarted.Active()
	if !ok || active.Model != "qwen3:4b" {
		t.Fatalf("restart lost activated snapshot: %+v %v", active, ok)
	}
}

func TestAISetupAndModelsRequireBearer(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": "qwen3:4b"}}})
	}))
	defer host.Close()
	store, err := appconfig.NewAIProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.Save(appconfig.AIProfile{Name: "Local", Kind: "ollama", BaseURL: host.URL, Model: "qwen3:4b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := New(t.TempDir(), "")
	s.WithAPIToken("setup-secret")
	s.WithAISetup(store, llm.NewSwitch(nil))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range []string{"/api/ai/setup", "/api/ai/profiles/" + p.ID + "/models"} {
		for _, tc := range []struct {
			name  string
			token string
			want  int
		}{{"absent", "", http.StatusUnauthorized}, {"wrong", "wrong", http.StatusUnauthorized}, {"correct", "setup-secret", http.StatusOK}} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				resp, err := srv.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != tc.want {
					t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
				}
			})
		}
	}
}

func TestTestAIProfileRejectsSuccessfulInferenceForStaleSnapshot(t *testing.T) {
	started := make(chan string, 1)
	finish := make(chan struct{})
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Errorf("decode inference request: %v", err)
		}
		started <- input.Model
		<-finish
		json.NewEncoder(w).Encode(map[string]string{"response": "ready"})
	}))
	defer host.Close()
	store, err := appconfig.NewAIProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := store.Save(appconfig.AIProfile{Name: "Local", Kind: "ollama", BaseURL: host.URL, Model: "qwen3:4b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := New(t.TempDir(), "")
	s.WithAISetup(store, llm.NewSwitch(nil))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	type testResponse struct {
		status int
		body   []byte
		err    error
	}
	response := make(chan testResponse, 1)
	go func() {
		resp, err := srv.Client().Post(srv.URL+"/api/ai/profiles/"+profile.ID+"/test", "application/json", nil)
		if err != nil {
			response <- testResponse{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		response <- testResponse{status: resp.StatusCode, body: body, err: err}
	}()
	select {
	case model := <-started:
		if model != "qwen3:4b" {
			t.Fatalf("test used model %q from a different profile snapshot", model)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("inference did not start")
	}
	if _, err := store.Save(appconfig.AIProfile{ID: profile.ID, Name: "Local", Kind: "ollama", BaseURL: host.URL, Model: "qwen3:8b"}, false); err != nil {
		t.Fatal(err)
	}
	close(finish)
	select {
	case result := <-response:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.status != http.StatusOK || !bytes.Contains(result.body, []byte(`"ok":false`)) {
			t.Fatalf("stale successful inference was accepted: status=%d %s", result.status, result.body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("test request did not finish")
	}
	profiles, _ := store.Snapshot()
	if profiles[0].Tested {
		t.Fatal("stale profile snapshot was marked tested")
	}
}

func TestAIProfileCredentialHostChangeAndRedirect(t *testing.T) {
	redirectTargetHit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectTargetHit = true }))
	defer target.Close()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer host.Close()
	store, _ := appconfig.NewAIProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	p, err := store.Save(appconfig.AIProfile{Name: "Remote", Kind: "openai-compatible", BaseURL: host.URL + "/v1", Model: "qwen", APIKey: "do-not-leak"}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := New(t.TempDir(), "")
	s.WithAISetup(store, llm.NewSwitch(nil))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	status, body := setupRequest(t, srv.Client(), "", http.MethodPost, srv.URL+"/api/ai/profiles/"+p.ID+"/test", nil)
	if status != 200 || !bytes.Contains(body, []byte(`"ok":false`)) || bytes.Contains(body, []byte("do-not-leak")) || redirectTargetHit {
		t.Fatalf("redirect escaped: %d %s target=%v", status, body, redirectTargetHit)
	}
	if _, err := store.Save(appconfig.AIProfile{ID: p.ID, Name: "Remote", Kind: "openai-compatible", BaseURL: target.URL + "/v1", Model: "qwen"}, false); err == nil {
		t.Fatal("host change reused old credential")
	}
	if _, err := store.Save(appconfig.AIProfile{ID: p.ID, Name: "Remote", Kind: "openai-compatible", BaseURL: target.URL + "/v1", Model: "qwen"}, true); err != nil {
		t.Fatal(err)
	}
	saved, _ := store.Get(p.ID)
	if saved.APIKey != "" {
		t.Fatal("clear_api_key retained secret")
	}
	for _, value := range []string{"http://user:pass@localhost:11434", "http://localhost:11434/?q=secret", "http://localhost:11434/#x", "ftp://localhost"} {
		_, err := appconfig.ValidateAIProfile(appconfig.AIProfile{Name: "x", Kind: "ollama", Model: "q", BaseURL: value})
		if err == nil {
			t.Fatalf("accepted unsafe URL: %s", value)
		}
	}
	if !strings.Contains(string(body), "connection or inference failed") {
		t.Fatalf("unclear test failure: %s", body)
	}
}
