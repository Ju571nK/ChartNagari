package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// HTTPDoer is the minimal interface the dispatcher uses to POST webhooks.
// *http.Client satisfies it; tests can inject a mock.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ConfigProvider returns the current ExecutionConfig. In production this is
// *config.ExecutionHolder; tests can pass a trivial closure.
type ConfigProvider interface {
	Get() config.ExecutionConfig
}

// Dispatcher fans out a TradeSignal to every eligible plugin in parallel via
// HMAC-signed HTTP POSTs. It enforces kill-switch, dedup, per-plugin filters,
// and a max-concurrent cap (ActiveCount). Decisions A3/A4/A5/A6/P1 and Codex
// #1/#2/#4/#6 are encoded here.
type Dispatcher struct {
	holder  ConfigProvider
	dedup   *DedupStore
	client  HTTPDoer
	log     zerolog.Logger
	nowFn   func() time.Time
	timeout time.Duration

	activeCount int64 // atomic; incremented on 2xx dispatch, decremented on auto-release or feedback
}

// Options customizes Dispatcher behavior (mainly for tests).
type Options struct {
	HTTPClient    HTTPDoer
	Timeout       time.Duration // per-plugin HTTP timeout, default 10s
	Now           func() time.Time
	Logger        zerolog.Logger
	ReleaseAfter  time.Duration // ActiveCount auto-release window, default 12s (10s + 2s retry)
	releaseAfter  time.Duration // resolved
}

// New constructs a Dispatcher wired to the given holder + dedup store.
func New(holder ConfigProvider, dedup *DedupStore, opts Options) *Dispatcher {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: timeout + 2*time.Second}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Dispatcher{
		holder:  holder,
		dedup:   dedup,
		client:  opts.HTTPClient,
		log:     opts.Logger,
		nowFn:   opts.Now,
		timeout: timeout,
	}
}

// ActiveCount returns the current in-flight dispatch count. Incremented on
// successful POST (2xx), decremented via the release window or feedback.
func (d *Dispatcher) ActiveCount() int64 {
	return atomic.LoadInt64(&d.activeCount)
}

// Release decrements ActiveCount (never below zero). Callers: the feedback
// handler on terminal statuses (FILLED/REJECTED/CANCELLED/ERROR).
func (d *Dispatcher) Release() {
	for {
		cur := atomic.LoadInt64(&d.activeCount)
		if cur <= 0 {
			return
		}
		if atomic.CompareAndSwapInt64(&d.activeCount, cur, cur-1) {
			return
		}
	}
}

// Dispatch fans out a single TradeSignal to every eligible plugin in parallel.
// Errors from individual plugins are logged and swallowed — one bad plugin
// cannot block the others (P1). The caller (pipeline) should fire-and-forget:
// Dispatch already handles its own lifecycle.
func (d *Dispatcher) Dispatch(ctx context.Context, signal models.TradeSignal) {
	cfg := d.holder.Get()

	// Master kill switch + enabled flag (T1, T2).
	if !cfg.Enabled {
		d.log.Debug().Str("signal_id", signal.ID).Msg("exec: dispatcher disabled, skip")
		return
	}
	if cfg.KillSwitch {
		d.log.Warn().Str("signal_id", signal.ID).Str("symbol", signal.Symbol).
			Msg("exec: kill switch ON, skip dispatch")
		return
	}
	// ActiveCount cap (T6). MaxDispatched == 0 means "no cap".
	if cfg.MaxDispatched > 0 && atomic.LoadInt64(&d.activeCount) >= int64(cfg.MaxDispatched) {
		d.log.Warn().Str("signal_id", signal.ID).Int64("active", atomic.LoadInt64(&d.activeCount)).
			Int("max", cfg.MaxDispatched).Msg("exec: max concurrent reached, skip")
		return
	}

	// Dedup (T3, T4) — single-statement INSERT OR IGNORE (Codex #2).
	// Fail-closed on ErrBusy (Codex #6).
	now := d.nowFn()
	if d.dedup != nil {
		fresh, err := d.dedup.ReserveDispatch(ctx, signal.Symbol, signal.Rule, signal.Direction, now)
		if err != nil {
			if errors.Is(err, ErrBusy) {
				d.log.Error().Err(err).Str("signal_id", signal.ID).
					Msg("exec: dedup busy — fail-closed, skip dispatch")
				return
			}
			d.log.Error().Err(err).Str("signal_id", signal.ID).
				Msg("exec: dedup error — fail-closed, skip dispatch")
			return
		}
		if !fresh {
			d.log.Debug().Str("signal_id", signal.ID).Str("symbol", signal.Symbol).
				Str("rule", signal.Rule).Str("direction", signal.Direction).
				Msg("exec: dedup hit, skip")
			return
		}
	}

	// Select eligible plugins.
	eligible := make([]config.PluginConfig, 0, len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		if !p.Enabled {
			continue
		}
		if !pluginAccepts(p, signal) {
			continue
		}
		eligible = append(eligible, p)
	}
	if len(eligible) == 0 {
		d.log.Debug().Str("signal_id", signal.ID).Msg("exec: no eligible plugins")
		return
	}

	// Serialize the payload once — every plugin gets the same body bytes so
	// the HMAC signature matches for each receiver (Codex #1).
	body, err := json.Marshal(signal)
	if err != nil {
		d.log.Error().Err(err).Str("signal_id", signal.ID).Msg("exec: marshal failed")
		return
	}

	// Fan-out (P1).
	var wg sync.WaitGroup
	for _, p := range eligible {
		wg.Add(1)
		go func(plugin config.PluginConfig) {
			defer wg.Done()
			d.dispatchOne(ctx, plugin, signal, body)
		}(p)
	}
	wg.Wait()
}

// pluginAccepts applies per-plugin symbols/min_score/direction_filter
// (T7, T8, T9).
func pluginAccepts(p config.PluginConfig, s models.TradeSignal) bool {
	if len(p.Symbols) > 0 {
		sym := strings.ToUpper(strings.TrimSpace(s.Symbol))
		match := false
		for _, want := range p.Symbols {
			if want == sym {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	if p.MinScore > 0 && s.Score < p.MinScore {
		return false
	}
	if p.DirectionFilter != "" && p.DirectionFilter != s.Direction {
		return false
	}
	return true
}

// dispatchOne handles signing + POST + 1 retry for a single plugin. On 2xx
// we increment ActiveCount and schedule auto-release (T14). On repeated
// failure we log and drop.
func (d *Dispatcher) dispatchOne(ctx context.Context, plugin config.PluginConfig, signal models.TradeSignal, body []byte) {
	method := http.MethodPost
	// Derive the path component from the URL so canonical signing matches
	// what a Go net/http plugin server will see.
	path := extractPath(plugin.URL)

	// Retry once on failure (T11, T12, T13).
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		ts := d.nowFn().Unix()
		signature := Sign(plugin.Secret, plugin.ID, ts, method, path, body)

		reqCtx, cancel := context.WithTimeout(ctx, d.timeout)
		req, err := http.NewRequestWithContext(reqCtx, method, plugin.URL, bytes.NewReader(body))
		if err != nil {
			cancel()
			lastErr = err
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(SignatureHeader, signature)
		req.Header.Set(TimestampHeader, strconv.FormatInt(ts, 10))
		req.Header.Set(PluginIDHeader, plugin.ID)

		start := d.nowFn()
		resp, err := d.client.Do(req)
		cancel()
		if err != nil {
			lastErr = err
			d.log.Warn().Err(err).Str("plugin_id", plugin.ID).Int("attempt", attempt+1).
				Str("signal_id", signal.ID).Msg("exec: POST failed")
			if attempt == 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			break
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			atomic.AddInt64(&d.activeCount, 1)
			d.log.Info().Str("plugin_id", plugin.ID).Str("signal_id", signal.ID).
				Int("status", resp.StatusCode).Dur("elapsed", d.nowFn().Sub(start)).
				Msg("exec: dispatched")
			// Auto-release window (T14): if no feedback arrives within the
			// timeout + retry budget we free the slot so ActiveCount doesn't
			// leak on silent plugin failures.
			go d.scheduleRelease(plugin.ID, signal.ID)
			return
		}
		lastErr = errors.New("non-2xx status: " + strconv.Itoa(resp.StatusCode))
		d.log.Warn().Int("status", resp.StatusCode).Str("plugin_id", plugin.ID).
			Int("attempt", attempt+1).Str("signal_id", signal.ID).Msg("exec: plugin non-2xx")
		if attempt == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
	}
	d.log.Error().Err(lastErr).Str("plugin_id", plugin.ID).Str("signal_id", signal.ID).
		Msg("exec: dispatch failed, drop")
}

// scheduleRelease auto-decrements ActiveCount after the retry window so the
// counter does not drift when a plugin never sends feedback. 12s = 10s HTTP
// timeout + 2s retry budget; feedback arriving sooner can call Release() and
// the double-dec is prevented by the atomic floor in Release.
func (d *Dispatcher) scheduleRelease(pluginID, signalID string) {
	time.Sleep(12 * time.Second)
	d.Release()
	d.log.Debug().Str("plugin_id", pluginID).Str("signal_id", signalID).
		Msg("exec: auto-released ActiveCount slot")
}

// extractPath returns the URL path component (without scheme/host). Falls back
// to "/" when parsing fails.
func extractPath(raw string) string {
	if idx := strings.Index(raw, "://"); idx >= 0 {
		rest := raw[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			p := rest[slash:]
			if q := strings.Index(p, "?"); q >= 0 {
				p = p[:q]
			}
			if p == "" {
				return "/"
			}
			return p
		}
	}
	return "/"
}
