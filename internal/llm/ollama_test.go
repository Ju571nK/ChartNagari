package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaProvider_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("bad path: %s", r.URL.Path)
		}
		var body struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "gemma4:4b" || body.Stream != false {
			t.Fatalf("unexpected body: %+v", body)
		}
		if !strings.Contains(body.Prompt, "SYSTEM") || !strings.Contains(body.Prompt, "USER") {
			t.Fatalf("prompt not composed: %q", body.Prompt)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": "interpretation here"})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 5*time.Second)
	out, err := p.Complete(context.Background(), "SYSTEM", "USER")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "interpretation here" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestOllamaProvider_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"response": "too late"})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 10*time.Millisecond)
	_, err := p.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestOllamaProvider_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model not found"})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "missing:model", 1*time.Second)
	_, err := p.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "model not found") && !strings.Contains(err.Error(), "404") {
		t.Fatalf("error didn't propagate upstream message: %v", err)
	}
}

func TestOllamaProvider_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 1*time.Second)
	_, err := p.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
}

func TestOllamaProvider_HonorsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "gemma4:4b", 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Complete(ctx, "s", "u")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
