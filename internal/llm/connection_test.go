package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAICompatibleConnectionAndSwitch(t *testing.T) {
	var calls int
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sample-key" {
			t.Errorf("missing auth")
		}
		switch r.URL.Path {
		case "/v1/chat/completions":
			calls++
			var input struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Model != "local-model" {
				t.Errorf("wrong inference request: %+v %v", input, err)
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "ready"}}}})
		case "/v1/models":
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "local-model"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer host.Close()
	connection, err := NewConnection("openai-compatible", host.URL+"/v1", "local-model", "sample-key")
	if err != nil {
		t.Fatal(err)
	}
	models, err := connection.Models(context.Background())
	if err != nil || len(models) != 1 || models[0] != "local-model" {
		t.Fatalf("models: %v %v", models, err)
	}
	switcher := NewSwitch(nil)
	if switcher.Available() {
		t.Fatal("cold switch available")
	}
	switcher.Set(connection)
	output, err := switcher.Complete(context.Background(), "system", "user")
	if err != nil || output != "ready" || calls != 1 {
		t.Fatalf("completion: %q %v calls=%d", output, err, calls)
	}
}

func TestOllamaReasoningOnlyResponseIsNotUsableText(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"response": "", "thinking": "internal reasoning"})
	}))
	defer host.Close()
	c, err := NewConnection("ollama", host.URL, "qwen3:4b", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Complete(context.Background(), "", "say ready"); err == nil {
		t.Fatal("empty user-facing response passed")
	}
}

func TestOllamaPullHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer host.Close()
	c, err := NewConnection("ollama", host.URL, "qwen3:4b", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- c.Pull(ctx, "qwen3:8b") }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("pull did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled pull succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled pull did not stop")
	}
}

func TestOllamaPullUsesModelField(t *testing.T) {
	var got map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pull" {
			t.Errorf("path = %q, want /api/pull", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode pull request: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer host.Close()
	c, err := NewConnection("ollama", host.URL, "qwen3:4b", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Pull(context.Background(), "qwen3:8b"); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "qwen3:8b" || got["name"] != nil || got["stream"] != false {
		t.Fatalf("unexpected Ollama pull payload: %#v", got)
	}
}
