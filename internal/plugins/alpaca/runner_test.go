package alpaca

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestNewRunner_InvalidConfig(t *testing.T) {
	t.Parallel()
	_, err := NewRunner(Config{}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNewRunner_Success(t *testing.T) {
	t.Parallel()
	cfg := Config{
		AlpacaAPIURL:     "http://127.0.0.1:1",
		AlpacaAPIKey:     "k",
		AlpacaAPISecret:  "s",
		FeedbackURL:      "http://127.0.0.1:8080/api/execution/feedback",
		PluginSecret:     "shh",
		PluginID:         "alpaca-paper",
		ListenAddr:       ":0",
		NotionalPerTrade: 1000,
		DBPath:           filepath.Join(t.TempDir(), "r.db"),
		TimestampSkewSec: 300,
	}
	r, err := NewRunner(cfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if err := r.store.Close(); err != nil {
		t.Errorf("store close: %v", err)
	}
}

func TestRunner_StartShutdown(t *testing.T) {
	t.Parallel()
	// Pick a free port up-front so Start can bind deterministically.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	cfg := Config{
		AlpacaAPIURL:     "http://127.0.0.1:1",
		AlpacaAPIKey:     "k",
		AlpacaAPISecret:  "s",
		FeedbackURL:      "http://127.0.0.1:8080/api/execution/feedback",
		PluginSecret:     "shh",
		PluginID:         "alpaca-paper",
		ListenAddr:       addr,
		NotionalPerTrade: 1000,
		DBPath:           filepath.Join(t.TempDir(), "r.db"),
		TimestampSkewSec: 300,
	}
	r, err := NewRunner(cfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx) }()

	// Wait for listener to be ready.
	var healthOK bool
	for i := 0; i < 50; i++ {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				healthOK = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !healthOK {
		t.Fatal("server never became ready")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

func TestRunner_StartListenError(t *testing.T) {
	t.Parallel()
	// Bind a listener and hold it so the runner's Start fails to bind.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	addr := l.Addr().String()

	cfg := Config{
		AlpacaAPIURL:     "http://127.0.0.1:1",
		AlpacaAPIKey:     "k",
		AlpacaAPISecret:  "s",
		FeedbackURL:      "http://127.0.0.1:8080/api/execution/feedback",
		PluginSecret:     "shh",
		PluginID:         "alpaca-paper",
		ListenAddr:       addr,
		NotionalPerTrade: 1000,
		DBPath:           filepath.Join(t.TempDir(), "r.db"),
		TimestampSkewSec: 300,
	}
	r, err := NewRunner(cfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("expected listen error")
	}
}
