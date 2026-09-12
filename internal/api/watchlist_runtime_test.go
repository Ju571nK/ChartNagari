package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

func TestWatchlistMutationsPublishOnlyPersistedSnapshots(t *testing.T) {
	s := setupTest(t)
	var updates []appconfig.WatchlistConfig
	s.WithWatchlistChanged(func(w appconfig.WatchlistConfig) { updates = append(updates, w) })
	add := map[string]string{"symbol": " spcx ", "type": "stock", "exchange": "nasdaq"}
	if rr := do(t, s, "POST", "/api/symbols", add); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	if len(updates) != 1 || updates[0].Symbols.Stocks[1].Symbol != "SPCX" {
		t.Fatal("addition not published")
	}
	if rr := do(t, s, "POST", "/api/symbols", add); rr.Code != 409 {
		t.Fatalf("duplicate allowed: %d", rr.Code)
	}
	if len(updates) != 1 {
		t.Fatal("duplicate published")
	}
	if rr := do(t, s, "PUT", "/api/symbols/SPCX", map[string]bool{"enabled": false}); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	if len(updates) != 2 || updates[1].Symbols.Stocks[1].Enabled {
		t.Fatal("disable not published")
	}
	if rr := do(t, s, "DELETE", "/api/symbols/SPCX", nil); rr.Code != 204 {
		t.Fatal(rr.Body.String())
	}
	if len(updates) != 3 || len(updates[2].Symbols.Stocks) != 1 {
		t.Fatal("delete not published")
	}
	stored, err := s.readWatchlist()
	if err != nil || len(stored.Symbols.Crypto) != 2 || stored.Symbols.Stocks[0].Symbol != "AAPL" {
		t.Fatal("unrelated watchlist changed")
	}
}

type lookupTransport func(*http.Request) (*http.Response, error)

func (f lookupTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestValidationDistinguishesProviderFailureAndNotFound(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	for _, test := range []struct {
		name             string
		status, expected int
	}{{"not found", 404, 200}, {"provider failure", 503, 502}} {
		t.Run(test.name, func(t *testing.T) {
			http.DefaultClient = &http.Client{Transport: lookupTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})}
			rr := do(t, setupTest(t), "GET", "/api/symbols/validate?symbol=SPCX", nil)
			if rr.Code != test.expected {
				t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestConcurrentSymbolRegistrationPreservesAllEdits(t *testing.T) {
	s := setupTest(t)
	var updates int
	s.WithWatchlistChanged(func(appconfig.WatchlistConfig) { updates++ })
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rr := do(t, s, "POST", "/api/symbols", map[string]string{"symbol": fmt.Sprintf("TEST%d", i), "type": "stock"})
			if rr.Code != 204 {
				t.Errorf("registration: %d", rr.Code)
			}
		}(i)
	}
	wg.Wait()
	w, err := s.readWatchlist()
	if err != nil || len(w.Symbols.Stocks) != 11 || updates != 10 {
		t.Fatalf("lost update: %v, %d stocks, %d updates", err, len(w.Symbols.Stocks), updates)
	}
	s.configDir = t.TempDir() + "/missing"
	if err := s.writeWatchlistLocked(w); err == nil || updates != 10 {
		t.Fatal("failed write published to runtime")
	}
}
