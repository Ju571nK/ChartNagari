package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

func watch(symbol string, enabled bool) appconfig.WatchlistConfig {
	var w appconfig.WatchlistConfig
	w.Timeframes = []string{"1H"}
	if symbol != "" {
		w.Symbols.Stocks = []appconfig.SymbolEntry{{Symbol: symbol, Enabled: enabled}}
	}
	return w
}

func TestRuntimeReplacesWithoutOverlap(t *testing.T) {
	r := NewRuntime(watch("SPCX", true))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan appconfig.WatchlistConfig, 10)
	finished := make(chan struct{})
	var active atomic.Int32
	go func() {
		defer close(finished)
		r.Run(ctx, func(ctx context.Context, w appconfig.WatchlistConfig) {
			if active.Add(1) != 1 {
				t.Error("overlapping generations")
			}
			started <- w
			<-ctx.Done()
			active.Add(-1)
		})
	}()
	await := func() appconfig.WatchlistConfig {
		t.Helper()
		select {
		case w := <-started:
			return w
		case <-time.After(time.Second):
			t.Fatal("generation did not start")
			return appconfig.WatchlistConfig{}
		}
	}
	await()
	for _, change := range []appconfig.WatchlistConfig{watch("TSLA", true), watch("TSLA", false), watch("", false)} {
		r.Update(change)
		got := await()
		if len(got.Symbols.Stocks) != len(change.Symbols.Stocks) {
			t.Fatal("deletion not applied")
		}
		if len(got.Symbols.Stocks) > 0 && got.Symbols.Stocks[0] != change.Symbols.Stocks[0] {
			t.Fatal("wrong generation")
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked")
	}
	if active.Load() != 0 {
		t.Fatal("workers remain")
	}
}

func TestRuntimeSnapshotIsolationAndConcurrentUpdates(t *testing.T) {
	w := watch("SPCX", true)
	r := NewRuntime(w)
	w.Symbols.Stocks[0].Symbol = "MUTATED"
	if r.Watchlist().Symbols.Stocks[0].Symbol != "SPCX" {
		t.Fatal("input alias")
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Update(watch("TSLA", true))
			snapshot := r.Watchlist()
			snapshot.Symbols.Stocks[0].Symbol = "LOCAL"
		}()
	}
	wg.Wait()
	if r.Watchlist().Symbols.Stocks[0].Symbol != "TSLA" {
		t.Fatal("snapshot alias")
	}
}

func TestGenerationHTTPPreservesTimeoutAndCancellation(t *testing.T) {
	for _, byTimeout := range []bool{false, true} {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
		ctx, cancel := context.WithCancel(context.Background())
		client := generationClient(&http.Client{Timeout: 100 * time.Millisecond}, ctx)
		done := make(chan error, 1)
		go func() { _, err := client.Get(server.URL); done <- err }()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("HTTP request did not start")
		}
		if !byTimeout {
			cancel()
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("request not canceled")
			}
		case <-time.After(time.Second):
			t.Fatal("cancellation/timeout was lost")
		}
		cancel()
		server.Close()
	}
}
