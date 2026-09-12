package collector

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"sync"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

// Runtime serializes watchlist generations. A replacement never overlaps the
// old collectors/pipeline, and rapid edits are coalesced to the latest snapshot.
type Runtime struct {
	mu      sync.Mutex
	desired appconfig.WatchlistConfig
	wake    chan struct{}
}

func NewRuntime(initial appconfig.WatchlistConfig) *Runtime {
	r := &Runtime{wake: make(chan struct{}, 1)}
	r.Update(initial)
	return r
}

func cloneWatchlist(w appconfig.WatchlistConfig) appconfig.WatchlistConfig {
	w.Symbols.Crypto = append([]appconfig.SymbolEntry(nil), w.Symbols.Crypto...)
	w.Symbols.Stocks = append([]appconfig.SymbolEntry(nil), w.Symbols.Stocks...)
	w.Symbols.Indices = append([]appconfig.SymbolEntry(nil), w.Symbols.Indices...)
	w.Timeframes = append([]string(nil), w.Timeframes...)
	return w
}

func (r *Runtime) Update(w appconfig.WatchlistConfig) {
	r.mu.Lock()
	r.desired = cloneWatchlist(w)
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runtime) Watchlist() appconfig.WatchlistConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneWatchlist(r.desired)
}

// run must return only after all workers of its generation have stopped.
func (r *Runtime) Run(ctx context.Context, run func(context.Context, appconfig.WatchlistConfig)) {
	var current appconfig.WatchlistConfig
	var cancel context.CancelFunc
	var done chan struct{}
	stop := func() {
		if cancel != nil {
			cancel()
			<-done
			cancel = nil
		}
	}
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
			next := r.Watchlist()
			if cancel != nil && reflect.DeepEqual(current, next) {
				continue
			}
			stop()
			if ctx.Err() != nil {
				return
			}
			current = r.Watchlist()
			workerCtx, workerCancel := context.WithCancel(ctx)
			cancel = workerCancel
			done = make(chan struct{})
			go func(w appconfig.WatchlistConfig, finished chan struct{}) { defer close(finished); run(workerCtx, w) }(current, done)
		}
	}
}

// Bind existing HTTP collectors to their generation without changing fetch APIs.
type generationTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t generationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	if t.ctx.Err() != nil {
		cancel()
	}
	response, err := t.base.RoundTrip(req.Clone(ctx))
	if err != nil {
		stop()
		cancel()
		return nil, err
	}
	response.Body = &generationBody{ReadCloser: response.Body, cleanup: func() { stop(); cancel() }}
	return response, nil
}

type generationBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *generationBody) Close() error { defer b.cleanup(); return b.ReadCloser.Close() }
func generationClient(client *http.Client, ctx context.Context) *http.Client {
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = generationTransport{ctx, base}
	return &copy
}
