package execution

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// staticCfg is a trivial ConfigProvider for tests.
type staticCfg struct {
	cfg config.ExecutionConfig
	mu  sync.RWMutex
}

func (s *staticCfg) Get() config.ExecutionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *staticCfg) set(c config.ExecutionConfig) {
	s.mu.Lock()
	s.cfg = c
	s.mu.Unlock()
}

// newTestDispatcher builds a Dispatcher wired to an in-memory SQLite DedupStore
// so we can exercise the full code path without a real DB file.
func newTestDispatcher(t *testing.T, cfg config.ExecutionConfig) (*Dispatcher, *staticCfg) {
	t.Helper()
	db := newTestDB(t)
	dedup := NewDedupStore(db, cfg.DedupWindow())
	holder := &staticCfg{cfg: cfg}
	d := New(holder, dedup, Options{
		Timeout: 500 * time.Millisecond,
		Now:     time.Now,
		Logger:  zerolog.Nop(),
	})
	return d, holder
}

func sampleSignal() models.TradeSignal {
	return models.TradeSignal{
		ID:        "sig-1",
		Version:   models.TradeSignalVersion,
		Timestamp: time.Now().UTC(),
		Symbol:    "BTCUSDT",
		Direction: "LONG",
		Timeframe: "1H",
		Rule:      "wyckoff_spring",
		Score:     15,
	}
}

// T1: master kill switch short-circuits dispatch without any HTTP call.
func TestDispatch_KillSwitch_Skips(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled:    true,
		KillSwitch: true,
		Plugins: []config.PluginConfig{
			{ID: "p1", URL: ts.URL, Secret: "s", Enabled: true},
		},
	}
	d, _ := newTestDispatcher(t, cfg)
	d.Dispatch(context.Background(), sampleSignal())

	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("kill switch must block dispatch; got %d hits", hits)
	}
}

// T2: Enabled=false also short-circuits.
func TestDispatch_Disabled_Skips(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled: false,
		Plugins: []config.PluginConfig{{ID: "p1", URL: ts.URL, Secret: "s", Enabled: true}},
	}
	d, _ := newTestDispatcher(t, cfg)
	d.Dispatch(context.Background(), sampleSignal())
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("disabled dispatcher must not hit plugin; got %d", hits)
	}
}

// T3/T4: duplicate (symbol, rule, direction) inside the dedup window is only
// POSTed once.
func TestDispatch_Dedup_InsideWindow(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled:        true,
		DedupWindowSec: 300,
		Plugins:        []config.PluginConfig{{ID: "p1", URL: ts.URL, Secret: "s", Enabled: true}},
	}
	d, _ := newTestDispatcher(t, cfg)

	s := sampleSignal()
	d.Dispatch(context.Background(), s)
	d.Dispatch(context.Background(), s)

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("dedup should collapse 2 dispatches to 1; got %d", got)
	}
}

// T6: MaxDispatched cap refuses new dispatches when ActiveCount is at the cap.
// Strategy: let the first dispatch succeed (fast 2xx → ActiveCount=1), then
// confirm a second distinct signal is rejected before reaching HTTP.
func TestDispatch_MaxDispatchedCap(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled:       true,
		MaxDispatched: 1,
		Plugins:       []config.PluginConfig{{ID: "p1", URL: ts.URL, Secret: "s", Enabled: true}},
	}
	d, _ := newTestDispatcher(t, cfg)

	// First dispatch succeeds; ActiveCount becomes 1 (scheduleRelease waits 12s
	// so we are guaranteed to be at the cap for the duration of the test).
	d.Dispatch(context.Background(), sampleSignal())
	if d.ActiveCount() != 1 {
		t.Fatalf("ActiveCount after first dispatch = %d, want 1", d.ActiveCount())
	}

	// Second dispatch with distinct key to bypass dedup; cap must block before
	// any HTTP call reaches the server.
	hitsBefore := atomic.LoadInt32(&hits)
	s2 := sampleSignal()
	s2.Rule = "other_rule"
	d.Dispatch(context.Background(), s2)
	if atomic.LoadInt32(&hits) != hitsBefore {
		t.Fatalf("MaxDispatched cap should block second dispatch; hits %d→%d", hitsBefore, atomic.LoadInt32(&hits))
	}
}

// T7: per-plugin symbols whitelist.
func TestPluginAccepts_SymbolsFilter(t *testing.T) {
	p := config.PluginConfig{Symbols: []string{"BTCUSDT"}}
	s := models.TradeSignal{Symbol: "BTCUSDT", Direction: "LONG", Score: 10}
	if !pluginAccepts(p, s) {
		t.Error("BTCUSDT should match whitelist")
	}
	s.Symbol = "ETHUSDT"
	if pluginAccepts(p, s) {
		t.Error("ETHUSDT must not match BTCUSDT-only whitelist")
	}
}

// T8: min_score filter.
func TestPluginAccepts_MinScore(t *testing.T) {
	p := config.PluginConfig{MinScore: 12}
	if pluginAccepts(p, models.TradeSignal{Score: 10}) {
		t.Error("score 10 must be rejected when min_score=12")
	}
	if !pluginAccepts(p, models.TradeSignal{Score: 15}) {
		t.Error("score 15 should pass min_score=12")
	}
}

// T9: direction_filter.
func TestPluginAccepts_DirectionFilter(t *testing.T) {
	p := config.PluginConfig{DirectionFilter: "LONG"}
	if !pluginAccepts(p, models.TradeSignal{Direction: "LONG"}) {
		t.Error("LONG must pass direction_filter=LONG")
	}
	if pluginAccepts(p, models.TradeSignal{Direction: "SHORT"}) {
		t.Error("SHORT must be rejected by direction_filter=LONG")
	}
}

// T11/T12: HMAC headers are present on every dispatched request.
func TestDispatch_SignsHeaders(t *testing.T) {
	var got struct {
		sig, ts, id string
	}
	done := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.sig = r.Header.Get(SignatureHeader)
		got.ts = r.Header.Get(TimestampHeader)
		got.id = r.Header.Get(PluginIDHeader)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled: true,
		Plugins: []config.PluginConfig{{ID: "p1", URL: ts.URL, Secret: "s3cr3t", Enabled: true}},
	}
	d, _ := newTestDispatcher(t, cfg)
	d.Dispatch(context.Background(), sampleSignal())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("plugin did not receive request")
	}
	if got.id != "p1" {
		t.Errorf("plugin id header = %q, want p1", got.id)
	}
	if got.sig == "" {
		t.Error("signature header missing")
	}
	if got.ts == "" {
		t.Error("timestamp header missing")
	}
	if _, err := strconv.ParseInt(got.ts, 10, 64); err != nil {
		t.Errorf("timestamp header not decimal unix: %v", err)
	}
}

// T13: non-2xx response triggers one retry, then gives up. ActiveCount stays 0.
func TestDispatch_NonSuccess_RetriesThenDrops(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled: true,
		Plugins: []config.PluginConfig{{ID: "p1", URL: ts.URL, Secret: "s", Enabled: true}},
	}
	d, _ := newTestDispatcher(t, cfg)
	d.Dispatch(context.Background(), sampleSignal())

	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("expected 2 hits (initial + 1 retry), got %d", got)
	}
	if d.ActiveCount() != 0 {
		t.Errorf("ActiveCount should stay 0 on failed dispatch, got %d", d.ActiveCount())
	}
}

// T14: 2xx increments ActiveCount.
func TestDispatch_Success_IncrementsActiveCount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled: true,
		Plugins: []config.PluginConfig{{ID: "p1", URL: ts.URL, Secret: "s", Enabled: true}},
	}
	d, _ := newTestDispatcher(t, cfg)
	d.Dispatch(context.Background(), sampleSignal())
	if d.ActiveCount() != 1 {
		t.Errorf("ActiveCount after success = %d, want 1", d.ActiveCount())
	}
}

// Release floors at 0 and never goes negative.
func TestRelease_FloorsAtZero(t *testing.T) {
	cfg := config.ExecutionConfig{Enabled: true}
	d, _ := newTestDispatcher(t, cfg)
	d.Release()
	d.Release()
	if d.ActiveCount() != 0 {
		t.Errorf("ActiveCount should never go below 0, got %d", d.ActiveCount())
	}
}

// Fan-out: a single signal reaches every eligible plugin in parallel (P1).
func TestDispatch_FanOut_AllPluginsHit(t *testing.T) {
	var a, b int32
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&a, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer tsA.Close()
	tsB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&b, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer tsB.Close()

	cfg := config.ExecutionConfig{
		Enabled: true,
		Plugins: []config.PluginConfig{
			{ID: "a", URL: tsA.URL, Secret: "s", Enabled: true},
			{ID: "b", URL: tsB.URL, Secret: "s", Enabled: true},
		},
	}
	d, _ := newTestDispatcher(t, cfg)
	d.Dispatch(context.Background(), sampleSignal())

	if atomic.LoadInt32(&a) != 1 || atomic.LoadInt32(&b) != 1 {
		t.Errorf("fan-out failed: a=%d b=%d, want 1/1", a, b)
	}
	if d.ActiveCount() != 2 {
		t.Errorf("ActiveCount after fan-out = %d, want 2", d.ActiveCount())
	}
}

// No eligible plugins (filters reject every plugin) → no HTTP, no ActiveCount.
func TestDispatch_NoEligiblePlugins(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.ExecutionConfig{
		Enabled: true,
		Plugins: []config.PluginConfig{
			// Enabled but min_score is above what sampleSignal provides.
			{ID: "p1", URL: ts.URL, Secret: "s", Enabled: true, MinScore: 9999},
			// Disabled entirely.
			{ID: "p2", URL: ts.URL, Secret: "s", Enabled: false},
		},
	}
	d, _ := newTestDispatcher(t, cfg)
	d.Dispatch(context.Background(), sampleSignal())
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("no eligible plugins should produce 0 hits, got %d", hits)
	}
	if d.ActiveCount() != 0 {
		t.Errorf("ActiveCount should remain 0, got %d", d.ActiveCount())
	}
}

// New() defaults: when Options are zero, Timeout falls back to 10s and the
// default http.Client is created.
func TestNew_DefaultOptions(t *testing.T) {
	db := newTestDB(t)
	dedup := NewDedupStore(db, 300)
	d := New(&staticCfg{}, dedup, Options{})
	if d == nil {
		t.Fatal("New returned nil")
	}
	if d.timeout != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", d.timeout)
	}
	if d.client == nil {
		t.Error("default client should be set")
	}
}

// extractPath unit coverage.
func TestExtractPath(t *testing.T) {
	cases := map[string]string{
		"https://host.example/api/hook":       "/api/hook",
		"http://localhost:8080/feedback?x=1":  "/feedback",
		"https://host":                       "/",
		"not-a-url":                          "/",
	}
	for in, want := range cases {
		if got := extractPath(in); got != want {
			t.Errorf("extractPath(%q)=%q, want %q", in, got, want)
		}
	}
}
